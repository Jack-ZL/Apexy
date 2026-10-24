package requestid

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDGenerated(t *testing.T) {
	handler := RequestIDMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 下游 (代理请求构造处) 能从请求 Header 读到生成的 ID
		if got := r.Header.Get(HeaderXRequestID); got == "" {
			t.Error("request header X-Request-ID not set for downstream")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/x", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// 响应 Header 也应带上 ID
	rid := w.Header().Get(HeaderXRequestID)
	if len(rid) != 32 {
		t.Errorf("response X-Request-ID = %q, want 32 hex chars", rid)
	}
}

func TestRequestIDPassthrough(t *testing.T) {
	handler := RequestIDMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set(HeaderXRequestID, "upstream-id-123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get(HeaderXRequestID); got != "upstream-id-123" {
		t.Errorf("X-Request-ID = %q, want passthrough upstream-id-123", got)
	}
}

func TestRequestIDUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := newRequestID()
		if seen[id] {
			t.Fatalf("duplicate request ID generated: %s", id)
		}
		seen[id] = true
	}
}
