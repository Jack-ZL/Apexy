package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"apexy/config"
	"apexy/log"
	"apexy/pkg/errors"
)

// ReverseProxy 结构体，封装反向代理核心逻辑
type ReverseProxy struct {
	// transport 所有代理请求共享的传输层 (连接池)。
	// 注意: 这里共享的是 Transport 而不是 httputil.ReverseProxy 实例,
	// 因为 ReverseProxy 的 Director/ModifyResponse 字段是路由相关的,
	// 若多个并发请求共享同一个 ReverseProxy 实例并在请求期间改写这些字段, 会产生数据竞争。
	transport *http.Transport
}

// NewReverseProxy 创建 ReverseProxy 实例
func NewReverseProxy() *ReverseProxy {
	return &ReverseProxy{
		// 共享连接池, 复用后端 TCP 连接, 提升代理性能
		transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

// PrepareProxyRequest 准备反向代理请求，修改请求的目标 URL 和 Host, 并应用请求转换规则
func (p *ReverseProxy) PrepareProxyRequest(req *http.Request, backendURL string, routeEntry *config.Route) (*http.Request, error) {
	targetURL, err := url.Parse(backendURL)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid backend URL %q: %v", errors.ErrInvalidRequest, backendURL, err)
	}

	proxyRequest := req.Clone(req.Context())   // 克隆原始请求，重要！避免修改原始请求
	proxyRequest.URL.Scheme = targetURL.Scheme // 设置代理请求的 Scheme (http/https)
	proxyRequest.URL.Host = targetURL.Host     // 设置代理请求的 Host (域名/IP:端口)
	// 保留原始请求路径, 并将其拼接在后端 URL 的基础路径之后。
	// 修复: 之前直接用 targetURL.Path 覆盖原始路径, 导致子路径信息丢失
	// (例如 /api/v1/users/123 被改写为后端的根路径)。
	proxyRequest.URL.Path = joinURLPath(targetURL.Path, req.URL.Path)
	proxyRequest.URL.RawPath = ""      // 清空 RawPath, 让 net/url 根据 Path 重新编码, 避免新旧路径不一致
	proxyRequest.Host = targetURL.Host // 设置 Host Header, 某些后端服务会检查 Host Header

	// 请求转换 - 添加 Header
	for key, value := range routeEntry.RequestTransform.HeadersToAdd {
		proxyRequest.Header.Set(key, value)
	}

	// 请求转换 - 删除 Header
	for _, headerKey := range routeEntry.RequestTransform.HeadersToRemove {
		proxyRequest.Header.Del(headerKey)
	}

	// 请求转换 - 添加 Query Parameters
	query := proxyRequest.URL.Query()
	for key, value := range routeEntry.RequestTransform.QueryParamsToAdd {
		query.Add(key, value)
	}

	// 请求转换 - 删除 Query Parameters
	for _, key := range routeEntry.RequestTransform.QueryParamsToRemove {
		query.Del(key)
	}
	proxyRequest.URL.RawQuery = query.Encode()

	return proxyRequest, nil
}

// ServeHTTP 实现反向代理转发。req 必须是 PrepareProxyRequest 返回的、已完成改写的代理请求。
// 每次请求创建一个轻量的 httputil.ReverseProxy 实例 (共享底层 Transport 连接池),
// 避免并发修改共享实例的 Director/ModifyResponse 字段导致数据竞争。
func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, req *http.Request, routeEntry *config.Route) {
	// 路由级后端超时: 通过请求上下文控制整个代理交换 (连接+发送+等待响应) 的最长时间,
	// 超时后上下文取消, ReverseProxy 中断请求并走 ErrorHandler 返回 504。
	// TimeoutSeconds 为 0 表示不限制 (依赖 Transport/Server 层的超时)。
	if routeEntry.TimeoutSeconds > 0 {
		ctx, cancel := context.WithTimeout(req.Context(), time.Duration(routeEntry.TimeoutSeconds)*time.Second)
		defer cancel()
		req = req.WithContext(ctx)
	}

	proxy := &httputil.ReverseProxy{
		Transport: p.transport, // 共享连接池
		// req 已在 PrepareProxyRequest 中完成 URL 改写和请求转换, Director 无需再做任何事。
		// 保留空 Director (而非使用 Rewrite), ReverseProxy 仍会自动追加 X-Forwarded-For。
		Director: func(proxyReq *http.Request) {},
		// 响应转换 - 在收到后端响应后修改响应 Header。
		// 修复: 之前在 Director 中修改的是"请求" Header, 响应转换规则从未真正生效,
		// 正确做法是通过 ModifyResponse 修改后端返回的响应。
		ModifyResponse: func(resp *http.Response) error {
			for key, value := range routeEntry.ResponseTransform.HeadersToAdd {
				resp.Header.Set(key, value)
			}
			for _, headerKey := range routeEntry.ResponseTransform.HeadersToRemove {
				resp.Header.Del(headerKey)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Errorf("ReverseProxy Error: %v, URL: %s, Error: %v", r.URL.String(), err)
			// 区分超时与其他后端错误: 上下文超时返回 504 Gateway Timeout,
			// 后端不可达/连接失败返回 502 Bad Gateway (而非之前统一的 500)
			if r.Context().Err() == context.DeadlineExceeded {
				http.Error(w, fmt.Sprintf("%v: backend timeout after %ds", errors.ErrGatewayTimeout, routeEntry.TimeoutSeconds), http.StatusGatewayTimeout)
				return
			}
			http.Error(w, fmt.Sprintf("%v: %v", errors.ErrBackendError, err), http.StatusBadGateway)
		},
	}

	proxy.ServeHTTP(w, req) // 执行反向代理
}

// joinURLPath 将后端 URL 的基础路径与原始请求路径拼接:
//   - 后端无基础路径 (如 http://localhost:8081):     直接使用原始请求路径
//   - 后端有基础路径 (如 http://localhost:8081/v2): 拼接为 /v2 + 原始请求路径
func joinURLPath(basePath, requestPath string) string {
	if basePath == "" || basePath == "/" {
		return requestPath
	}
	return strings.TrimSuffix(basePath, "/") + "/" + strings.TrimPrefix(requestPath, "/")
}
