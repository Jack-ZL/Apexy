package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apexy/config"
	"apexy/route"
)

func boolPtr(b bool) *bool { return &b }

// newTestHandler 构造一个通过认证后返回 200 的下游 Handler
func newTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// doRequest 执行请求, withRoute 非 nil 时将路由注入上下文 (模拟 HTTPHandler 行为)
func doRequest(t *testing.T, h http.Handler, headerKey, headerVal string, withRoute *config.Route) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/test", nil)
	if headerVal != "" {
		req.Header.Set(headerKey, headerVal)
	}
	if withRoute != nil {
		req = req.WithContext(route.WithRoute(req.Context(), withRoute))
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestAPIKeyAuth(t *testing.T) {
	cfg := config.APIKeyAuthConfig{
		Enabled:    true,
		HeaderName: "X-API-Key",
		Keys:       []string{"valid-key-1", "valid-key-2"},
	}
	handler := APIKeyAuthMiddleware(cfg)(newTestHandler())

	tests := []struct {
		name     string
		key      string
		routeCfg *config.Route
		wantCode int
	}{
		{"有效Key放行", "valid-key-1", nil, http.StatusOK},
		{"第二个有效Key放行", "valid-key-2", nil, http.StatusOK},
		{"无Key拒绝", "", nil, http.StatusUnauthorized},
		{"无效Key拒绝", "bad-key", nil, http.StatusUnauthorized},
		{"路由级禁用认证-无Key也放行", "", &config.Route{Path: "/api/test", Auth: config.RouteAuth{APIKeyAuthEnabled: boolPtr(false)}}, http.StatusOK},
		{"路由级禁用认证-无效Key也放行", "bad-key", &config.Route{Path: "/api/test", Auth: config.RouteAuth{APIKeyAuthEnabled: boolPtr(false)}}, http.StatusOK},
		{"路由级启用认证-有效Key放行", "valid-key-1", &config.Route{Path: "/api/test", Auth: config.RouteAuth{APIKeyAuthEnabled: boolPtr(true)}}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(t, handler, "X-API-Key", tt.key, tt.routeCfg)
			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}

func TestAPIKeyAuthGlobalDisabledRouteEnabled(t *testing.T) {
	// 全局禁用, 但路由显式启用 -> 该路由仍需认证
	cfg := config.APIKeyAuthConfig{Enabled: false, HeaderName: "X-API-Key", Keys: []string{"k"}}
	handler := APIKeyAuthMiddleware(cfg)(newTestHandler())

	routeEntry := &config.Route{Path: "/api/test", Auth: config.RouteAuth{APIKeyAuthEnabled: boolPtr(true)}}

	if w := doRequest(t, handler, "X-API-Key", "", routeEntry); w.Code != http.StatusUnauthorized {
		t.Errorf("no key: status = %d, want 401", w.Code)
	}
	if w := doRequest(t, handler, "X-API-Key", "k", routeEntry); w.Code != http.StatusOK {
		t.Errorf("valid key: status = %d, want 200", w.Code)
	}
	// 无路由上下文 + 全局禁用 -> 放行
	if w := doRequest(t, handler, "X-API-Key", "", nil); w.Code != http.StatusOK {
		t.Errorf("global disabled: status = %d, want 200", w.Code)
	}
}

func TestAPIKeyFromQueryAndTrim(t *testing.T) {
	cfg := config.APIKeyAuthConfig{Enabled: true, HeaderName: "api_key", Keys: []string{"q-key"}}
	handler := APIKeyAuthMiddleware(cfg)(newTestHandler())

	// 从 Query 参数获取 Key
	req := httptest.NewRequest("GET", "/api/test?api_key=q-key", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("query key: status = %d, want 200", w.Code)
	}

	// Key 前后带空格应被 trim 后通过
	req2 := httptest.NewRequest("GET", "/api/test", nil)
	req2.Header.Set("api_key", "  q-key  ")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("trimmed key: status = %d, want 200", w2.Code)
	}
}

func TestMaskAPIKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"abcdefgh", "abcd****(len=8)"},
		{"abc", "a****(len=3)"},
		{"abcd", "a****(len=4)"},
	}
	for _, tt := range tests {
		if got := maskAPIKey(tt.in); got != tt.want {
			t.Errorf("maskAPIKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	// 脱敏后不应包含完整 Key
	if masked := maskAPIKey("super-secret-key"); strings.Contains(masked, "super-secret-key") {
		t.Error("masked key leaked full value")
	}
}
