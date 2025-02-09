package config

import (
	"os"
	"strings"

	"gopkg.in/yaml.v2"
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
	Port int `yaml:"port"`
}

// LogConfig 定义日志配置
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"` // "json" or "text"
	Output string `yaml:"output"` // "file" or "stdout"
	File   string `yaml:"file"`   // 日志文件路径 (如果 output 是 "file")
}

// MetricsConfig 定义指标监控配置
type MetricsConfig struct {
	Port int `yaml:"port"`
}

// Route 定义路由规则
type Route struct {
	Path              string            `yaml:"path"`
	Method            string            `yaml:"method"`
	Backend           string            `yaml:"backend"` // 后端服务地址
	RequestTransform  RequestTransform  `yaml:"request_transform"`
	ResponseTransform ResponseTransform `yaml:"response_transform"`
	Auth              RouteAuth         `yaml:"auth"`
	RateLimit         RouteRateLimit    `yaml:"rate_limit"`
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
	APIKeyAuthEnabled bool `yaml:"api_key_auth_enabled"`
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
}

// RateLimitConfig 定义全局速率限制配置
type RateLimitConfig struct {
	Enabled bool                       `yaml:"enabled"`
	General RateLimitPolicy            `yaml:"general"` // 一般限流策略
	Routes  map[string]RateLimitPolicy `yaml:"routes"`  // 路由特定限流策略
}

// RouteRateLimit 定义路由级别的速率限制配置
type RouteRateLimit struct {
	Enabled bool            `yaml:"enabled"`
	Policy  RateLimitPolicy `yaml:"policy"`
}

// RateLimitPolicy 定义速率限制策略
type RateLimitPolicy struct {
	RequestsPerMinute int `yaml:"requests_per_minute"`
}

// LoadConfig 加载配置文件
func LoadConfig(path string) (*Config, error) {
	f, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = yaml.Unmarshal(f, &cfg)
	if err != nil {
		return nil, err
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

	return &cfg, nil
}
