package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"apexy/config"
)

func newHandler(cfg config.CORSConfig) http.Handler {
	return CORSMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func TestCORSWildcardWithCredentialsIsRejected(t *testing.T) {
	// 安全约束: allow_credentials=true 时通配 Origin 不生效, 必须显式列出
	cfg := config.CORSConfig{
		Enabled:          true,
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET"},
		AllowCredentials: true,
	}
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	newHandler(cfg).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (wildcard must not apply with credentials)", w.Code)
	}
}

func TestCORSWildcardWithoutCredentials(t *testing.T) {
	cfg := config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET"},
	}
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "https://any.com")
	w := httptest.NewRecorder()
	newHandler(cfg).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	// 回显具体 Origin 而非 "*"
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://any.com" {
		t.Errorf("Allow-Origin = %q, want echoed origin", got)
	}
	// 必须声明 Vary: Origin 防止缓存投毒
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}
}

func TestCORSExplicitOriginWithCredentials(t *testing.T) {
	cfg := config.CORSConfig{
		Enabled:          true,
		AllowedOrigins:   []string{"https://trusted.com"},
		AllowedMethods:   []string{"GET"},
		AllowCredentials: true,
	}

	// 白名单内 Origin 放行且带凭证头
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "https://trusted.com")
	w := httptest.NewRecorder()
	newHandler(cfg).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("trusted origin: status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, want true", got)
	}

	// 白名单外 Origin 拒绝
	req2 := httptest.NewRequest("GET", "/x", nil)
	req2.Header.Set("Origin", "https://evil.com")
	w2 := httptest.NewRecorder()
	newHandler(cfg).ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Errorf("evil origin: status = %d, want 403", w2.Code)
	}
}

func TestCORSPreflight(t *testing.T) {
	cfg := config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"https://trusted.com"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Content-Type", "X-API-Key"},
		MaxAgeSeconds:  300,
	}
	handler := newHandler(cfg)

	// 合法的预检请求: 204 + Max-Age
	req := httptest.NewRequest("OPTIONS", "/x", nil)
	req.Header.Set("Origin", "https://trusted.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, X-API-Key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("valid preflight: status = %d, want 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Max-Age"); got != "300" {
		t.Errorf("Max-Age = %q, want 300", got)
	}

	// 预检声明了不允许的方法: 403
	req2 := httptest.NewRequest("OPTIONS", "/x", nil)
	req2.Header.Set("Origin", "https://trusted.com")
	req2.Header.Set("Access-Control-Request-Method", "DELETE")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Errorf("disallowed method preflight: status = %d, want 403", w2.Code)
	}

	// 预检声明了不允许的 Header: 403
	req3 := httptest.NewRequest("OPTIONS", "/x", nil)
	req3.Header.Set("Origin", "https://trusted.com")
	req3.Header.Set("Access-Control-Request-Method", "GET")
	req3.Header.Set("Access-Control-Request-Headers", "X-Evil")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	if w3.Code != http.StatusForbidden {
		t.Errorf("disallowed header preflight: status = %d, want 403", w3.Code)
	}
}

func TestCORSDefaultMaxAge(t *testing.T) {
	cfg := config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET"},
	}
	req := httptest.NewRequest("OPTIONS", "/x", nil)
	req.Header.Set("Origin", "https://a.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	newHandler(cfg).ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Max-Age"); got != "600" {
		t.Errorf("default Max-Age = %q, want 600", got)
	}
}

func TestCORSNoOriginPassthrough(t *testing.T) {
	cfg := config.CORSConfig{Enabled: true, AllowedOrigins: []string{"https://a.com"}}
	req := httptest.NewRequest("GET", "/x", nil) // 无 Origin 头 (非跨域)
	w := httptest.NewRecorder()
	newHandler(cfg).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("non-CORS request: status = %d, want 200", w.Code)
	}
}

func TestCORSDisabled(t *testing.T) {
	cfg := config.CORSConfig{Enabled: false}
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	newHandler(cfg).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("disabled CORS: status = %d, want 200 (passthrough)", w.Code)
	}
}
