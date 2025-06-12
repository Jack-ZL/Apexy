package metrics

// Metrics 包提供了一个简单的 Prometheus 监控服务，支持注册和暴露自定义指标
import (
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
}

// NewMetricsServer 创建 MetricsServer 实例
func NewMetricsServer(port int) *MetricsServer {
	registry := prometheus.NewRegistry() // 创建 Prometheus Registry
	return &MetricsServer{
		port:     port,
		registry: registry,
	}
}

// Start 启动 Metrics HTTP 服务
func (m *MetricsServer) Start() {
	go func() {
		http.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})) // 注册 /metrics Handler
		addr := fmt.Sprintf(":%d", m.port)
		log.Infof("Prometheus metrics endpoint listening on %s/metrics", addr)
		if err := http.ListenAndServe(addr, nil); err != nil && err != http.ErrServerClosed { // 监听指定端口
			log.Errorf("Metrics server ListenAndServe failed: %v", err)
		}
	}()
}

// ServeHTTP 实现 http.Handler 接口，用于直接处理 metrics 请求 (例如在 main.go 中注册 /metrics handler)
func (m *MetricsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}).ServeHTTP(w, r) //  使用 Prometheus 官方 Handler 处理
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
// Histogram 用于统计一组数值的分布情况，常用于记录请求延迟、响应时间等连续型数据的分布。通过 prometheus.NewHistogram 创建指标后，注册到自定义的 registry，便于后续采集和暴露。
func (m *MetricsServer) RegisterHistogram(opts prometheus.HistogramOpts) prometheus.Histogram {
	histogram := prometheus.NewHistogram(opts)
	m.registry.MustRegister(histogram)
	return histogram
}

// 示例指标 (可以根据实际需要扩展)
var (
	requestTotalCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_gateway_request_total",
			Help: "Total number of requests handled by the API gateway.",
		},
		[]string{"method", "path", "status"},
	)

	requestDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "api_gateway_request_duration_seconds",
			Help:    "Histogram of request duration in seconds.",
			Buckets: prometheus.DefBuckets, // 默认 buckets
		},
		[]string{"method", "path"},
	)
)

// RegisterDefaultMetrics 注册默认指标
func RegisterDefaultMetrics(registry *prometheus.Registry) {
	registry.MustRegister(requestTotalCounter)
	registry.MustRegister(requestDurationHistogram)
}

// IncRequestTotalCounter 增加请求总数 Counter 指标
func IncRequestTotalCounter(method, path, status string) {
	requestTotalCounter.With(prometheus.Labels{"method": method, "path": path, "status": status}).Inc()
}

// ObserveRequestDurationHistogram 观察请求延迟 Histogram 指标
func ObserveRequestDurationHistogram(method, path string, duration float64) {
	requestDurationHistogram.With(prometheus.Labels{"method": method, "path": path}).Observe(duration)
}

// ConvertStatusCodeToString 将 HTTP 状态码转换为字符串
func ConvertStatusCodeToString(statusCode int) string {
	return strconv.Itoa(statusCode)
}

// 注册默认指标到默认 Registry (全局 Registry)
func init() {
	registry := prometheus.NewRegistry()
	RegisterDefaultMetrics(registry)
}
