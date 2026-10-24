package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"apexy/config"
)

func TestJoinURLPath(t *testing.T) {
	tests := []struct {
		base, reqPath, want string
	}{
		{"", "/api/v1/users/1", "/api/v1/users/1"},
		{"/", "/api/v1/users/1", "/api/v1/users/1"},
		{"/v2", "/api/v1/users/1", "/v2/api/v1/users/1"},
		{"/v2/", "/api/v1/users/1", "/v2/api/v1/users/1"},
	}
	for _, tt := range tests {
		if got := joinURLPath(tt.base, tt.reqPath); got != tt.want {
			t.Errorf("joinURLPath(%q, %q) = %q, want %q", tt.base, tt.reqPath, got, tt.want)
		}
	}
}

func TestPrepareProxyRequest(t *testing.T) {
	p := NewReverseProxy()
	routeEntry := &config.Route{
		Path:    "/api/v1/users",
		Backend: "http://backend.local:9000/base",
		RequestTransform: config.RequestTransform{
			HeadersToAdd:        map[string]string{"X-Request-ID": "rid-1"},
			HeadersToRemove:     []string{"X-Powered-By"},
			QueryParamsToAdd:    map[string]string{"app_version": "1.0"},
			QueryParamsToRemove: []string{"debug"},
		},
	}

	req := httptest.NewRequest("GET", "/api/v1/users/123?debug=1&keep=1", nil)
	req.Header.Set("X-Powered-By", "old")

	proxyReq, err := p.PrepareProxyRequest(req, routeEntry.Backend, routeEntry)
	if err != nil {
		t.Fatalf("PrepareProxyRequest failed: %v", err)
	}

	// 目标 Scheme/Host 改写为后端地址
	if proxyReq.URL.Scheme != "http" || proxyReq.URL.Host != "backend.local:9000" {
		t.Errorf("unexpected target: %s://%s", proxyReq.URL.Scheme, proxyReq.URL.Host)
	}
	// 原始路径保留, 并拼接在后端基础路径之后
	if proxyReq.URL.Path != "/base/api/v1/users/123" {
		t.Errorf("unexpected path: %s", proxyReq.URL.Path)
	}
	// Host Header 改写
	if proxyReq.Host != "backend.local:9000" {
		t.Errorf("unexpected host header: %s", proxyReq.Host)
	}
	// 请求转换: Header 增删
	if got := proxyReq.Header.Get("X-Request-ID"); got != "rid-1" {
		t.Errorf("X-Request-ID = %q, want rid-1", got)
	}
	if got := proxyReq.Header.Get("X-Powered-By"); got != "" {
		t.Errorf("X-Powered-By should be removed, got %q", got)
	}
	// 请求转换: Query 增删
	q := proxyReq.URL.Query()
	if q.Get("app_version") != "1.0" {
		t.Errorf("app_version = %q, want 1.0", q.Get("app_version"))
	}
	if q.Get("debug") != "" {
		t.Errorf("debug should be removed, got %q", q.Get("debug"))
	}
	if q.Get("keep") != "1" {
		t.Errorf("keep = %q, want 1", q.Get("keep"))
	}
	// 原始请求不应被修改
	if req.Header.Get("X-Powered-By") != "old" {
		t.Error("original request was mutated")
	}
}

// echoBackend 返回一个回显请求信息的测试后端
func echoBackend(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "test-backend")
		w.Header().Set("X-Backend-Echo", "yes")
		fmt.Fprintf(w, "path=%s xrid=%s", r.URL.Path, r.Header.Get("X-Request-ID"))
	}))
}

func TestServeHTTPResponseTransform(t *testing.T) {
	backend := echoBackend(t)
	defer backend.Close()

	p := NewReverseProxy()
	routeEntry := &config.Route{
		Path:    "/api",
		Backend: backend.URL,
		ResponseTransform: config.ResponseTransform{
			HeadersToAdd:    map[string]string{"X-Gateway-Version": "1.0"},
			HeadersToRemove: []string{"Server"},
		},
	}

	req := httptest.NewRequest("GET", "/api/v1/users/9", nil)
	proxyReq, err := p.PrepareProxyRequest(req, backend.URL, routeEntry)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	p.ServeHTTP(w, proxyReq, routeEntry)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	// 响应转换: 添加自定义 Header, 删除 Server Header
	if got := resp.Header.Get("X-Gateway-Version"); got != "1.0" {
		t.Errorf("X-Gateway-Version = %q, want 1.0", got)
	}
	if got := resp.Header.Get("Server"); got != "" {
		t.Errorf("Server header should be removed, got %q", got)
	}
	// 子路径应原样到达后端
	body := w.Body.String()
	if !strings.Contains(body, "path=/api/v1/users/9") {
		t.Errorf("backend did not receive original path, body: %s", body)
	}
}

func TestServeHTTPBackendUnreachable502(t *testing.T) {
	p := NewReverseProxy()
	routeEntry := &config.Route{Path: "/api", Backend: "http://127.0.0.1:1"} // 不可达端口

	req := httptest.NewRequest("GET", "/api/x", nil)
	proxyReq, err := p.PrepareProxyRequest(req, routeEntry.Backend, routeEntry)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	p.ServeHTTP(w, proxyReq, routeEntry)
	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}

func TestServeHTTPTimeoutExceeded(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1500 * time.Millisecond) // 超过路由 1 秒超时
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := NewReverseProxy()
	routeEntry := &config.Route{Path: "/slow", Backend: backend.URL, TimeoutSeconds: 1}

	req := httptest.NewRequest("GET", "/slow", nil)
	proxyReq, err := p.PrepareProxyRequest(req, backend.URL, routeEntry)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	p.ServeHTTP(w, proxyReq, routeEntry)
	if w.Code != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want 504", w.Code)
	}
}
