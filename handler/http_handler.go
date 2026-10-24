package handler

import (
	"fmt"
	"net/http"

	"apexy/log"
	"apexy/middleware"
	"apexy/pkg/errors"
	"apexy/proxy"
	"apexy/route"
)

// HTTPHandler 结构体，处理 HTTP 请求
type HTTPHandler struct {
	router          *route.Router            // 路由服务
	reverseProxy    *proxy.ReverseProxy      // 反向代理服务
	middlewareChain *middleware.ChainBuilder // 中间件链构建器
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
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(fmt.Sprintf("%v: %s %s", errors.ErrRouteNotFound, r.Method, r.URL.Path)))
		log.Warnf("Route not found for path: %s, method: %s", r.URL.Path, r.Method)
		return
	}

	// 将匹配到的路由注入请求上下文, 下游中间件 (认证/限流/指标)
	// 通过 route.FromContext 读取路由级配置, 实现路由级配置覆盖全局配置
	r = r.WithContext(route.WithRoute(r.Context(), routeEntry))

	backendURL := routeEntry.Backend
	log.Debugf("Matched route: %s %s -> %s", r.Method, r.URL.Path, backendURL)

	// 2. 构建中间件链, 链的末端执行 "构造代理请求 + 反向代理"。
	// 注意: 代理请求必须在链末端 (而非链之前) 克隆构造, 这样中间件对请求的修改
	// (如 Request-ID 中间件写入的 X-Request-ID Header) 才能被克隆进代理请求并转发给后端。
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyRequest, err := h.reverseProxy.PrepareProxyRequest(r, backendURL, routeEntry)
		if err != nil {
			log.Errorf("PrepareProxyRequest failed: %v", err)
			http.Error(w, fmt.Sprintf("%v: failed to prepare proxy request", errors.ErrInternalServer), http.StatusInternalServerError)
			return
		}
		h.reverseProxy.ServeHTTP(w, proxyRequest, routeEntry) // 执行反向代理
	})

	// 3. 应用 Middleware 链并执行 (开始处理请求，应用中间件和反向代理)
	h.middlewareChain.Build(finalHandler).ServeHTTP(w, r)
}
