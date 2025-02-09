package logging

import (
	"net/http"
	"time"

	"apexy/log"
	"apexy/middleware"
)

// LoggingMiddleware 日志记录 Middleware
func LoggingMiddleware(logger log.Logger) middleware.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startTime := time.Now()
			wrappedWriter := &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK} // 包装 ResponseWriter 方便记录状态码

			next.ServeHTTP(wrappedWriter, r) // 调用下一个 Handler

			duration := time.Since(startTime)
			logger.Infof("Request: Method=%s, Path=%s, Status=%d, Duration=%v", r.Method, r.URL.Path, wrappedWriter.statusCode, duration.String())
		})
	}
}

// responseWriterWrapper 包装 http.ResponseWriter 以记录状态码
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}
