package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"apexy/config"
	"apexy/log"
)

// ReverseProxy 结构体，封装反向代理核心逻辑
type ReverseProxy struct {
	proxy *httputil.ReverseProxy
}

// NewReverseProxy 创建 ReverseProxy 实例
func NewReverseProxy() *ReverseProxy {
	return &ReverseProxy{
		proxy: &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				// Director 函数，在每次代理请求时调用，用于修改请求
				// 默认 ReverseProxy 会根据 TargetHost 和 Request.URL 重写请求
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				log.Errorf("ReverseProxy Error: %v, URL: %s, Error: %v", r.URL.String(), err)
				http.Error(w, "Backend service error", http.StatusInternalServerError)
			},
			// Transport: &http.Transport{ // 可自定义 Transport (例如配置 TLS, 连接池参数)
			// 	MaxIdleConns:        100,
			// 	IdleConnTimeout:     90 * time.Second,
			// 	DisableKeepAlives:   false,
			// 	MaxIdleConnsPerHost: 100,
			// 	TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, // #nosec
			// },
		},
	}
}

// PrepareProxyRequest 准备反向代理请求，修改请求的目标 URL 和 Host
func (p *ReverseProxy) PrepareProxyRequest(req *http.Request, backendURL string, routeEntry *config.Route) (*http.Request, error) {
	targetURL, err := url.Parse(backendURL)
	if err != nil {
		return nil, err
	}

	proxyRequest := req.Clone(req.Context())   // 克隆原始请求，重要！避免修改原始请求
	proxyRequest.URL.Scheme = targetURL.Scheme // 设置代理请求的 Scheme (http/https)
	proxyRequest.URL.Host = targetURL.Host     // 设置代理请求的 Host (域名/IP:端口)
	proxyRequest.URL.Path = targetURL.Path     // 设置代理请求的 Path (路径)
	proxyRequest.Host = targetURL.Host         // 设置 Host Header,  某些后端服务会检查 Host Header

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
	proxyRequest.URL.RawQuery = query.Encode()

	// 请求转换 - 删除 Query Parameters  (暂未实现，可根据需要添加)

	return proxyRequest, nil
}

// ServeHTTP 实现 http.Handler 接口，处理反向代理请求
func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, req *http.Request, routeEntry *config.Route) {
	// 响应转换 - 添加 Header (在 ServeHTTP 之前无法修改 Response Header,  只能在 Director 或 Transport 中修改)
	p.proxy.Director = func(proxyReq *http.Request) {
		//  Director 默认逻辑 (重写 URL)
		targetURL, _ := url.Parse(routeEntry.Backend) // 忽略错误，PrepareProxyRequest 中已解析过
		proxyReq.URL.Scheme = targetURL.Scheme
		proxyReq.URL.Host = targetURL.Host
		proxyReq.URL.Path = routeEntry.Path
		proxyReq.Host = targetURL.Host

		//  Director 中添加 响应 Header  (注意：需要在 Director 中修改 proxyReq.Header)
		for key, value := range routeEntry.ResponseTransform.HeadersToAdd {
			proxyReq.Header.Set(key, value) //  注意：修改的是 proxyReq.Header
		}

		//  Director 中删除 响应 Header (注意：需要在 Director 中修改 proxyReq.Header)
		for _, headerKey := range routeEntry.ResponseTransform.HeadersToRemove {
			proxyReq.Header.Del(headerKey) // 注意：修改的是 proxyReq.Header
		}
	}

	p.proxy.ServeHTTP(w, req) // 执行反向代理
}
