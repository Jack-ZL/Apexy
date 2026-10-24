package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"apexy/config"
	"apexy/route"
)

func boolPtr(b bool) *bool { return &b }

func TestTokenBucketAllowAndRefill(t *testing.T) {
	b := newTokenBucket(3)

	// 容量为 3: 前 3 次获取成功, 第 4 次失败
	for i := 0; i < 3; i++ {
		if !b.allow() {
			t.Fatalf("allow() #%d = false, want true", i+1)
		}
	}
	if b.allow() {
		t.Fatal("allow() should fail when bucket is empty")
	}

	// 模拟时间流逝 30 秒 (速率 3/分钟 -> 应补充 1.5 个令牌, 取整后可获取 1 个)
	b.mutex.Lock()
	b.lastRefill = time.Now().Add(-30 * time.Second)
	b.mutex.Unlock()

	if !b.allow() {
		t.Fatal("allow() should succeed after proportional refill (30s of 3/min = 1.5 tokens)")
	}
	if b.allow() {
		t.Fatal("only 1 full token should be available after 30s (0.5 remains)")
	}

	// 再流逝 60 秒: 补充 3 个令牌, 加上剩余 0.5, 最多 3 个 (不超过容量)
	b.mutex.Lock()
	b.lastRefill = time.Now().Add(-60 * time.Second)
	b.mutex.Unlock()
	for i := 0; i < 3; i++ {
		if !b.allow() {
			t.Fatalf("allow() #%d = false after full refill, want true", i+1)
		}
	}
}

func TestTokenBucketRefillCapsAtCapacity(t *testing.T) {
	b := newTokenBucket(5)
	b.allow() // 消耗 1 个
	// 流逝 1 小时, 令牌不应超过容量
	b.mutex.Lock()
	b.lastRefill = time.Now().Add(-time.Hour)
	b.mutex.Unlock()
	b.allow()
	b.mutex.Lock()
	got := b.tokens
	b.mutex.Unlock()
	if got > float64(b.capacity) {
		t.Errorf("tokens = %v, exceeds capacity %d", got, b.capacity)
	}
}

func TestResolvePolicyPriority(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled: true,
		General: config.RateLimitPolicy{RequestsPerMinute: 1000},
		Routes: map[string]config.RateLimitPolicy{
			"/api/v1/users": {RequestsPerMinute: 500},
		},
	}

	tests := []struct {
		name        string
		routeEntry  *config.Route
		wantEnabled bool
		wantRPM     int
		wantScope   string
	}{
		{
			name:        "无路由上下文-使用全局策略",
			routeEntry:  nil,
			wantEnabled: true, wantRPM: 1000, wantScope: "general",
		},
		{
			name:        "路由无覆盖-命中全局按路径策略",
			routeEntry:  &config.Route{Path: "/api/v1/users"},
			wantEnabled: true, wantRPM: 500, wantScope: "/api/v1/users",
		},
		{
			name: "路由内联策略优先级最高",
			routeEntry: &config.Route{Path: "/api/v1/users", RateLimit: config.RouteRateLimit{
				Enabled: boolPtr(true),
				Policy:  config.RateLimitPolicy{RequestsPerMinute: 200},
			}},
			wantEnabled: true, wantRPM: 200, wantScope: "/api/v1/users",
		},
		{
			name: "路由显式禁用限流",
			routeEntry: &config.Route{Path: "/api/v1/users", RateLimit: config.RouteRateLimit{
				Enabled: boolPtr(false),
			}},
			wantEnabled: false, wantRPM: 500, wantScope: "/api/v1/users",
		},
		{
			name: "全局禁用但路由显式启用",
			routeEntry: &config.Route{Path: "/other", RateLimit: config.RouteRateLimit{
				Enabled: boolPtr(true),
				Policy:  config.RateLimitPolicy{RequestsPerMinute: 10},
			}},
			wantEnabled: true, wantRPM: 10, wantScope: "/other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localCfg := cfg
			if tt.name == "全局禁用但路由显式启用" {
				localCfg.Enabled = false
			}
			req := httptest.NewRequest("GET", "/x", nil)
			if tt.routeEntry != nil {
				req = req.WithContext(route.WithRoute(req.Context(), tt.routeEntry))
			}
			enabled, rpm, scope := resolvePolicy(localCfg, req)
			if enabled != tt.wantEnabled || rpm != tt.wantRPM || scope != tt.wantScope {
				t.Errorf("resolvePolicy = (%v, %d, %q), want (%v, %d, %q)",
					enabled, rpm, scope, tt.wantEnabled, tt.wantRPM, tt.wantScope)
			}
		})
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name           string
		remoteAddr     string
		xRealIP        string
		xff            string
		trustForwarded bool
		want           string
	}{
		{"默认只信RemoteAddr-忽略伪造头", "1.2.3.4:5678", "9.9.9.9", "8.8.8.8", false, "1.2.3.4"},
		{"信任头时优先X-Real-IP", "1.2.3.4:5678", "9.9.9.9", "8.8.8.8", true, "9.9.9.9"},
		{"信任头时其次XFF首个IP", "1.2.3.4:5678", "", "8.8.8.8, 7.7.7.7", true, "8.8.8.8"},
		{"IPv6 RemoteAddr正确解析", "[::1]:5678", "", "", false, "::1"},
		{"RemoteAddr无端口", "1.2.3.4", "", "", false, "1.2.3.4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/x", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if got := getClientIP(req, tt.trustForwarded); got != tt.want {
				t.Errorf("getClientIP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRateLimitMiddlewareEnforcement(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled: true,
		General: config.RateLimitPolicy{RequestsPerMinute: 3},
	}
	handler := RateLimitMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 限流按 IP 计数, 每个请求使用不同的 RemoteAddr 端口但相同 IP
	do := func() int {
		req := httptest.NewRequest("GET", "/x", nil)
		req.RemoteAddr = "5.6.7.8:1000"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w.Code
	}

	for i := 0; i < 3; i++ {
		if code := do(); code != http.StatusOK {
			t.Fatalf("request #%d: status = %d, want 200", i+1, code)
		}
	}
	if code := do(); code != http.StatusTooManyRequests {
		t.Errorf("request #4: status = %d, want 429", code)
	}
}

func TestRateLimitMiddlewareRouteDisabled(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled: true,
		General: config.RateLimitPolicy{RequestsPerMinute: 1},
	}
	handler := RateLimitMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 路由显式禁用限流: 连续请求都应放行
	routeEntry := &config.Route{Path: "/open", RateLimit: config.RouteRateLimit{Enabled: boolPtr(false)}}
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/open", nil)
		req.RemoteAddr = "6.6.6.6:1000"
		req = req.WithContext(route.WithRoute(req.Context(), routeEntry))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request #%d on disabled route: status = %d, want 200", i+1, w.Code)
		}
	}
}
