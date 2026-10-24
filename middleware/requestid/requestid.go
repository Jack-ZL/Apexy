package requestid

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"apexy/middleware"
)

// HeaderXRequestID 请求 ID 的 Header 名称
const HeaderXRequestID = "X-Request-ID"

// RequestIDMiddleware 请求 ID Middleware:
//   - 若客户端/上游代理已携带 X-Request-ID, 则原样透传 (便于全链路追踪)
//   - 否则生成一个 16 字节的随机 ID
//
// ID 会同时写入请求 Header (随代理转发给后端, 后端可记录同一 ID 实现链路关联)
// 和响应 Header (返回给调用方, 便于报障时定位日志)。
func RequestIDMiddleware() middleware.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get(HeaderXRequestID)
			if requestID == "" {
				requestID = newRequestID()
				r.Header.Set(HeaderXRequestID, requestID) // 写入请求 Header, 随代理转发给后端
			}
			w.Header().Set(HeaderXRequestID, requestID) // 写入响应 Header, 返回给调用方
			next.ServeHTTP(w, r)
		})
	}
}

// newRequestID 使用加密安全随机数生成 32 位十六进制请求 ID。
// 生成失败时 (极罕见, 系统熵源不可用) 退化为固定值, 保证 Header 始终非空。
func newRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "unknown-request-id"
	}
	return hex.EncodeToString(buf)
}
