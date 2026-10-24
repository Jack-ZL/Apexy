package metrics

// Metrics 包提供了一个简单的 Prometheus 监控服务，支持注册和暴露自定义指标
import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"apexy/log"
)

// MetricsServer 结构体，封装 Metrics 服务
type MetricsServer struct {
	port     int
	registry *prometheus.Registry
	server   *http.Server // 独立的 Metrics HTTP 服务, 保存实例以支持优雅关闭

	// 网关默认指标。
	// 修复: 之前这些指标注册在 init() 创建的一次性 registry 上, 与 MetricsServer
	// 自己的 registry 不是同一个, 导致 /metrics 端点上永远看不到这些指标;
	// 且记录函数从未被调用。现在统一注册到 MetricsServer.registry,
	// 并由日志中间件在每次请求结束后调用 RecordRequest 上报。
	requestTotal    *prometheus.CounterVec   // 请求总数 (按 method/path/status 维度)
	requestDuration *prometheus.HistogramVec // 请求延迟分布 (按 method/path 维度)
}

// NewMetricsServer 创建 MetricsServer 实例, 并注册网关默认指标
func NewMetricsServer(port int) *MetricsServer {
	registry := prometheus.NewRegistry() // 创建 Prometheus Registry

	m := &MetricsServer{
		port:     port,
		registry: registry,
		requestTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_gateway_request_total",
				Help: "Total number of requests handled by the API gateway.",
			},
			[]string{"method", "path", "status"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "api_gateway_request_duration_seconds",
				Help:    "Histogram of request duration in seconds.",
				Buckets: prometheus.DefBuckets, // 默认 buckets
			},
			[]string{"method", "path"},
		),
	}

	// 将默认指标注册到本实例的 registry, 确保能通过 /metrics 端点暴露
	registry.MustRegister(m.requestTotal)
	registry.MustRegister(m.requestDuration)
	// 同时注册 Go 运行时指标 (goroutine 数、内存等), 便于监控网关自身状态
	registry.MustRegister(prometheus.NewGoCollector())

	return m
}

// Start 启动 Metrics HTTP 服务 (独立端口, 与网关业务端口隔离)
func (m *MetricsServer) Start() {
	// 使用独立的 ServeMux。
	// 修复: 之前调用 http.Handle 注册到全局 DefaultServeMux, 隐式依赖全局状态,
	// 且与 main.go 中主服务 mux 上的 /metrics 重复暴露。
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))

	m.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", m.port),
		Handler: mux,
	}

	go func() {
		log.Infof("Prometheus metrics endpoint listening on %s/metrics", m.server.Addr)
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("Metrics server ListenAndServe failed: %v", err)
		}
	}()
}

// Shutdown 优雅关闭 Metrics HTTP 服务
func (m *MetricsServer) Shutdown(ctx context.Context) error {
	if m.server == nil {
		return nil
	}
	return m.server.Shutdown(ctx)
}

// RecordRequest 记录一次请求的指标 (请求总数 + 延迟分布)。
// 由日志中间件在每次请求处理完成后调用。
func (m *MetricsServer) RecordRequest(method, path string, statusCode int, durationSeconds float64) {
	status := strconv.Itoa(statusCode)
	m.requestTotal.With(prometheus.Labels{"method": method, "path": path, "status": status}).Inc()
	m.requestDuration.With(prometheus.Labels{"method": method, "path": path}).Observe(durationSeconds)
}

// RegisterGauge 指标 Gauge 注册
// Gauge 用于表示一个可以任意增减的数值，比如当前内存使用量、并发数等。
func (m *MetricsServer) RegisterGauge(opts prometheus.GaugeOpts) prometheus.Gauge {
	gauge := prometheus.NewGauge(opts)
	m.registry.MustRegister(gauge)
	return gauge
}

// RegisterCounter 指标 Counter 注册
// Counter 用于统计只增不减的数值，比如请求总数、错误总数等。每次调用 Inc() 方法时，计数器会加一，适合用于累计事件发生的次数。
func (m *MetricsServer) RegisterCounter(opts prometheus.CounterOpts) prometheus.Counter {
	counter := prometheus.NewCounter(opts)
	m.registry.MustRegister(counter)
	return counter
}

// RegisterHistogram 指标 Histogram（直方图）注册
// Histogram 用于统计一组数值的分布情况，常用于记录请求延迟、响应时间等连续型数据的分布。
func (m *MetricsServer) RegisterHistogram(opts prometheus.HistogramOpts) prometheus.Histogram {
	histogram := prometheus.NewHistogram(opts)
	m.registry.MustRegister(histogram)
	return histogram
}
