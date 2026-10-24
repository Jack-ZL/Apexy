package logging

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"

	"apexy/log"
	"apexy/middleware"
	"apexy/route"
)

// RequestRecorder 抽象请求指标记录能力, 由 *metrics.MetricsServer 实现。
// 使用接口而非直接依赖 metrics 包, 保持中间件与具体指标实现的解耦, 也便于测试时注入 mock。
type RequestRecorder interface {
	RecordRequest(method, path string, statusCode int, durationSeconds float64)
}

// LoggingMiddleware 日志记录 Middleware, 同时负责上报请求指标 (recorder 可为 nil, 表示不上报)
func LoggingMiddleware(logger log.Logger, recorder RequestRecorder) middleware.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startTime := time.Now()
			wrappedWriter := &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK} // 包装 ResponseWriter 方便记录状态码

			next.ServeHTTP(wrappedWriter, r) // 调用下一个 Handler

			duration := time.Since(startTime)
			logger.Infof("Request: Method=%s, Path=%s, Status=%d, Size=%d, Duration=%v",
				r.Method, r.URL.Path, wrappedWriter.statusCode, wrappedWriter.size, duration.String())

			// 上报 Prometheus 指标 (请求总数 + 延迟分布)
			if recorder != nil {
				recorder.RecordRequest(r.Method, metricsPathLabel(r), wrappedWriter.statusCode, duration.Seconds())
			}
		})
	}
}

// metricsPathLabel 返回用于指标 label 的路径。
// 优先使用匹配到的路由规则路径 (如 /api/v1/users) 而不是原始请求路径 (如 /api/v1/users/123),
// 避免带路径参数的请求导致 Prometheus label 基数无限膨胀 (high cardinality 问题)。
func metricsPathLabel(r *http.Request) string {
	if routeEntry := route.FromContext(r.Context()); routeEntry != nil {
		return routeEntry.Path
	}
	return r.URL.Path
}

// responseWriterWrapper 包装 http.ResponseWriter 以记录状态码和响应大小。
// 注意: 必须同时透传 Flush/Hijack/Push 等可选接口, 否则包装后下游
// (如反向代理的流式响应、WebSocket 升级、HTTP/2 Push) 会因类型断言失败而失效。
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
	size       int // 已写入的响应体字节数
}

func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterWrapper) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.size += n // 累计响应大小
	return n, err
}

// Flush 透传 http.Flusher: 支持 SSE/分块传输等流式响应及时刷出
func (w *responseWriterWrapper) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack 透传 http.Hijacker: 支持 WebSocket 等协议升级
func (w *responseWriterWrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
}

// Push 透传 http.Pusher: 支持 HTTP/2 Server Push
func (w *responseWriterWrapper) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}
