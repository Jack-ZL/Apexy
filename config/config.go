package config

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"apexy/pkg/errors"
)

// Config 定义 API 网关的配置结构
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Log       LogConfig       `yaml:"log"`
	Metrics   MetricsConfig   `yaml:"metrics"`
	Routes    []Route         `yaml:"routes"`
	CORS      CORSConfig      `yaml:"cors"`
	Auth      AuthConfig      `yaml:"auth"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
}

// ServerConfig 定义服务器配置
type ServerConfig struct {
	Port int       `yaml:"port"`
	TLS  TLSConfig `yaml:"tls"` // HTTPS/TLS 配置 (可选)

	// HTTP Server 超时 (秒), 0 表示使用默认值。
	// 注意: WriteTimeout 必须大于所有路由的 timeout_seconds, 否则后端超时时
	// 网关来不及写回 504 响应, 连接会被 Server 直接关闭 (客户端收到空响应)。
	// 默认 WriteTimeout=0 (不限制), 由路由级 timeout_seconds 控制后端等待时间,
	// 这也是 API 网关的常见做法 (需支持 SSE/长轮询等长响应)。
	ReadTimeoutSeconds  int `yaml:"read_timeout_seconds"`  // 读取请求超时, 默认 15s
	WriteTimeoutSeconds int `yaml:"write_timeout_seconds"` // 写响应超时, 默认 0 (不限制)
	IdleTimeoutSeconds  int `yaml:"idle_timeout_seconds"`  // keep-alive 空闲超时, 默认 120s
}

// ReadTimeout 返回读取请求超时时间 (未配置时使用默认值 15s)
func (s *ServerConfig) ReadTimeout() time.Duration {
	if s.ReadTimeoutSeconds > 0 {
		return time.Duration(s.ReadTimeoutSeconds) * time.Second
	}
	return 15 * time.Second
}

// WriteTimeout 返回写响应超时时间 (未配置时为 0, 即不限制)
func (s *ServerConfig) WriteTimeout() time.Duration {
	return time.Duration(s.WriteTimeoutSeconds) * time.Second
}

// IdleTimeout 返回 keep-alive 空闲超时时间 (未配置时使用默认值 120s)
func (s *ServerConfig) IdleTimeout() time.Duration {
	if s.IdleTimeoutSeconds > 0 {
		return time.Duration(s.IdleTimeoutSeconds) * time.Second
	}
	return 120 * time.Second
}

// TLSConfig 定义主服务的 TLS 配置。
// Enabled 为 true 时, 主 HTTP 服务将以 HTTPS 方式监听, 需提供证书和私钥文件路径。
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"` // TLS 证书文件路径 (PEM)
	KeyFile  string `yaml:"key_file"`  // TLS 私钥文件路径 (PEM)
}

// LogConfig 定义日志配置
type LogConfig struct {
	Level  string `yaml:"level"`  // 日志级别 "debug", "info", "warn", "error"
	Format string `yaml:"format"` // 日志格式化方式  "json" or "text"
	Output string `yaml:"output"` // "file" or "stdout"
	File   string `yaml:"file"`   // 日志文件路径 (如果 output 是 "file")
}

// MetricsConfig 定义指标监控配置
type MetricsConfig struct {
	Port int `yaml:"port"`
}

// Route 定义路由规则
type Route struct {
	Path              string            `yaml:"path"`               // 路由路径
	Method            string            `yaml:"method"`             // 请求方式
	Backend           string            `yaml:"backend"`            // 后端服务地址
	TimeoutSeconds    int               `yaml:"timeout_seconds"`    // 后端请求超时时间 (秒), 0 表示不限制
	RequestTransform  RequestTransform  `yaml:"request_transform"`  // 请求转换规则
	ResponseTransform ResponseTransform `yaml:"response_transform"` // 响应转换规则
	Auth              RouteAuth         `yaml:"auth"`               // 路由认证配置
	RateLimit         RouteRateLimit    `yaml:"rate_limit"`         // 路由速率限制配置
}

// RequestTransform 定义请求转换规则
type RequestTransform struct {
	HeadersToAdd        map[string]string `yaml:"headers_to_add"`
	HeadersToRemove     []string          `yaml:"headers_to_remove"`
	QueryParamsToAdd    map[string]string `yaml:"query_params_to_add"`
	QueryParamsToRemove []string          `yaml:"query_params_to_remove"`
}

// ResponseTransform 定义响应转换规则
type ResponseTransform struct {
	HeadersToAdd    map[string]string `yaml:"headers_to_add"`
	HeadersToRemove []string          `yaml:"headers_to_remove"`
}

// RouteAuth 定义路由级别的认证配置
type RouteAuth struct {
	// 是否启用 API Key 认证。使用指针类型以区分"未配置"(nil) 和"显式配置为 false"两种情况:
	//   - nil:   路由未配置该项, 沿用全局 auth.api_key_auth.enabled
	//   - true:  该路由强制启用认证 (覆盖全局配置)
	//   - false: 该路由强制禁用认证 (覆盖全局配置)
	APIKeyAuthEnabled *bool `yaml:"api_key_auth_enabled"`
}

// APIKeyAuthEnabled 返回该路由最终生效的 API Key 认证开关。
// globalEnabled 为全局配置值, 路由级配置 (非 nil 时) 优先于全局配置。
func (r *RouteAuth) APIKeyAuthEnabledOr(globalEnabled bool) bool {
	if r.APIKeyAuthEnabled != nil {
		return *r.APIKeyAuthEnabled // 路由级显式配置, 覆盖全局
	}
	return globalEnabled // 路由未配置, 沿用全局
}

// AuthConfig 定义全局认证配置
type AuthConfig struct {
	APIKeyAuth APIKeyAuthConfig `yaml:"api_key_auth"`
}

// APIKeyAuthConfig 定义 API Key 认证配置
type APIKeyAuthConfig struct {
	Enabled    bool     `yaml:"enabled"`
	HeaderName string   `yaml:"header_name"`
	Keys       []string `yaml:"keys"` // 允许的 API Keys 列表
}

// CORSConfig 定义 CORS 配置
type CORSConfig struct {
	Enabled          bool     `yaml:"enabled"`
	AllowedOrigins   []string `yaml:"allowed_origins"`
	AllowedMethods   []string `yaml:"allowed_methods"`
	AllowedHeaders   []string `yaml:"allowed_headers"`
	ExposedHeaders   []string `yaml:"exposed_headers"`
	AllowCredentials bool     `yaml:"allow_credentials"`
	MaxAgeSeconds    int      `yaml:"max_age_seconds"` // 预检请求 (OPTIONS) 结果的缓存时间 (秒), 0 表示使用默认值 600
}

// RateLimitConfig 定义全局速率限制配置
type RateLimitConfig struct {
	Enabled bool `yaml:"enabled"`
	// 是否信任 X-Real-IP / X-Forwarded-For 请求头来获取客户端 IP。
	// 安全警告: 仅当网关部署在可信反向代理 (如 Nginx/云 LB) 之后时才应开启,
	// 否则攻击者可以伪造这些 Header 绕过基于 IP 的限流。默认 false, 只使用 TCP 连接的 RemoteAddr。
	TrustForwardedHeaders bool                       `yaml:"trust_forwarded_headers"`
	General               RateLimitPolicy            `yaml:"general"` // 一般限流策略
	Routes                map[string]RateLimitPolicy `yaml:"routes"`  // 路由特定限流策略
}

// RouteRateLimit 定义路由级别的速率限制配置
type RouteRateLimit struct {
	// 是否启用速率限制。使用指针类型以区分"未配置"(nil) 和"显式配置为 false":
	//   - nil:   路由未配置该项, 沿用全局 rate_limit.enabled
	//   - true:  该路由强制启用限流 (覆盖全局配置)
	//   - false: 该路由强制禁用限流 (覆盖全局配置)
	Enabled *bool           `yaml:"enabled"`
	Policy  RateLimitPolicy `yaml:"policy"` // 路由特定限流策略, RequestsPerMinute > 0 时覆盖全局策略
}

// EnabledOr 返回该路由最终生效的限流开关。
// globalEnabled 为全局配置值, 路由级配置 (非 nil 时) 优先于全局配置。
func (r *RouteRateLimit) EnabledOr(globalEnabled bool) bool {
	if r.Enabled != nil {
		return *r.Enabled // 路由级显式配置, 覆盖全局
	}
	return globalEnabled // 路由未配置, 沿用全局
}

// RateLimitPolicy 定义速率限制策略
type RateLimitPolicy struct {
	RequestsPerMinute int `yaml:"requests_per_minute"`
}

// LoadConfig 加载配置文件
func LoadConfig(path string) (*Config, error) {
	f, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", errors.ErrConfigLoadFailed, path, err)
	}

	var cfg Config
	// KnownFields(true): 严格模式, 配置文件中出现未知字段时报错,
	// 避免因字段名拼写错误导致配置被静默忽略
	dec := yaml.NewDecoder(bytes.NewReader(f))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("%w: invalid yaml: %v", errors.ErrConfigLoadFailed, err)
	}

	// 默认日志级别为 info
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	// 默认日志格式为 text
	if cfg.Log.Format == "" {
		cfg.Log.Format = "text"
	}
	// 默认日志输出到 stdout
	if cfg.Log.Output == "" {
		cfg.Log.Output = "stdout"
	}

	// 转换日志级别为小写
	cfg.Log.Level = strings.ToLower(cfg.Log.Level)

	// 校验配置合法性, 尽早暴露配置错误 (fail-fast)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate 校验配置的合法性, 返回第一个发现的错误。
// 在启动阶段调用, 避免带着非法配置运行。
func (c *Config) Validate() error {
	// 服务端口校验
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("%w: server.port must be between 1 and 65535, got %d", errors.ErrInvalidRequest, c.Server.Port)
	}
	if c.Metrics.Port <= 0 || c.Metrics.Port > 65535 {
		return fmt.Errorf("%w: metrics.port must be between 1 and 65535, got %d", errors.ErrInvalidRequest, c.Metrics.Port)
	}
	// Server 超时必须为非负数 (0 表示使用默认值/不限制)
	if c.Server.ReadTimeoutSeconds < 0 || c.Server.WriteTimeoutSeconds < 0 || c.Server.IdleTimeoutSeconds < 0 {
		return fmt.Errorf("%w: server timeouts must be non-negative", errors.ErrInvalidRequest)
	}
	if c.Metrics.Port == c.Server.Port {
		return fmt.Errorf("%w: metrics.port must differ from server.port", errors.ErrInvalidRequest)
	}

	// TLS 配置校验
	if c.Server.TLS.Enabled && (c.Server.TLS.CertFile == "" || c.Server.TLS.KeyFile == "") {
		return fmt.Errorf("%w: server.tls.cert_file and server.tls.key_file are required when tls is enabled", errors.ErrInvalidRequest)
	}

	// 日志配置校验
	switch c.Log.Level {
	case "debug", "info", "warn", "error", "fatal", "panic":
	default:
		return fmt.Errorf("%w: invalid log.level %q", errors.ErrInvalidRequest, c.Log.Level)
	}

	// 认证配置校验: 启用认证时必须配置 Header 名称和至少一个 API Key
	if c.Auth.APIKeyAuth.Enabled {
		if c.Auth.APIKeyAuth.HeaderName == "" {
			return fmt.Errorf("%w: auth.api_key_auth.header_name is required when api key auth is enabled", errors.ErrInvalidRequest)
		}
		if len(c.Auth.APIKeyAuth.Keys) == 0 {
			return fmt.Errorf("%w: auth.api_key_auth.keys must not be empty when api key auth is enabled", errors.ErrInvalidRequest)
		}
	}

	// 路由校验
	seen := make(map[string]bool, len(c.Routes))
	for i, r := range c.Routes {
		if r.Path == "" || !strings.HasPrefix(r.Path, "/") {
			return fmt.Errorf("%w: routes[%d].path must start with '/', got %q", errors.ErrInvalidRequest, i, r.Path)
		}
		if r.Method != "" {
			switch strings.ToUpper(r.Method) {
			case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete,
				http.MethodPatch, http.MethodHead, http.MethodOptions:
			default:
				return fmt.Errorf("%w: routes[%d].method %q is not a valid HTTP method", errors.ErrInvalidRequest, i, r.Method)
			}
		}
		backendURL, err := url.Parse(r.Backend)
		if err != nil || backendURL.Host == "" || (backendURL.Scheme != "http" && backendURL.Scheme != "https") {
			return fmt.Errorf("%w: routes[%d].backend %q must be a valid http(s) URL", errors.ErrInvalidRequest, i, r.Backend)
		}
		if r.TimeoutSeconds < 0 {
			return fmt.Errorf("%w: routes[%d].timeout_seconds must be >= 0", errors.ErrInvalidRequest, i)
		}
		key := strings.ToUpper(r.Method) + " " + r.Path
		if seen[key] {
			return fmt.Errorf("%w: duplicate route %q", errors.ErrInvalidRequest, key)
		}
		seen[key] = true
	}

	return nil
}
