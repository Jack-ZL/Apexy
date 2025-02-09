package handler

import (
	"net/http"

	"apexy/log"
	"apexy/middleware"
	"apexy/proxy"
	"apexy/route"
)

// HTTPHandler 结构体，处理 HTTP 请求
type HTTPHandler struct {
	router          *route.Router
	reverseProxy    *proxy.ReverseProxy
	middlewareChain *middleware.ChainBuilder
}

// NewHTTPHandler 创建 HTTPHandler 实例
func NewHTTPHandler(router *route.Router, reverseProxy *proxy.ReverseProxy, middlewareChain *middleware.ChainBuilder) *HTTPHandler {
	return &HTTPHandler{
		router:          router,
		reverseProxy:    reverseProxy,
		middlewareChain: middlewareChain,
	}
}

// ServeHTTP 实现 http.Handler 接口，处理所有进入 API 网关的 HTTP 请求
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. 路由匹配
	routeEntry := h.router.MatchRoute(r)
	if routeEntry == nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("404 Not Found"))
		log.Warnf("Route not found for path: %s, method: %s", r.URL.Path, r.Method)
		return
	}

	backendURL := routeEntry.Backend
	log.Debugf("Matched route: %s %s -> %s", r.Method, r.URL.Path, backendURL)

	// 2. 创建 ReverseProxy 请求 (基于原始请求和后端 URL)
	proxyRequest, err := h.reverseProxy.PrepareProxyRequest(r, backendURL, routeEntry)
	if err != nil {
		http.Error(w, "Failed to prepare proxy request", http.StatusInternalServerError)
		log.Errorf("PrepareProxyRequest failed: %v", err)
		return
	}

	// 3. 应用 Middleware 链 (构建请求处理链)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.reverseProxy.ServeHTTP(w, proxyRequest, routeEntry) // 执行反向代理
	})
	//handler = h.middlewareChain.Build(handler) // 构建并应用 Middleware 链
	handler = h.middlewareChain.Build(handler).ServeHTTP // 构建并应用 Middleware 链

	// 4. 执行 Handler (开始处理请求，应用中间件和反向代理)
	handler.ServeHTTP(w, r)
}
