package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempConfig 将 yaml 内容写入临时文件并返回路径
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const validConfig = `
server:
  port: 8080
  tls:
    enabled: false
metrics:
  port: 9090
log:
  level: INFO
auth:
  api_key_auth:
    enabled: true
    header_name: X-API-Key
    keys:
      - "k1"
rate_limit:
  enabled: true
  trust_forwarded_headers: false
  general:
    requests_per_minute: 1000
  routes:
    /api/v1/users:
      requests_per_minute: 500
routes:
  - path: /api/v1/users
    method: GET
    backend: http://localhost:8081
    timeout_seconds: 10
    auth:
      api_key_auth_enabled: true
    rate_limit:
      enabled: false
      policy:
        requests_per_minute: 200
  - path: /public/data
    backend: http://localhost:8083
    auth:
      api_key_auth_enabled: false
`

func TestLoadConfigValid(t *testing.T) {
	cfg, err := LoadConfig(writeTempConfig(t, validConfig))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// 日志级别默认转小写
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want info", cfg.Log.Level)
	}
	// 路由级 *bool 正确解析: true / false / 未配置 (nil)
	if cfg.Routes[0].Auth.APIKeyAuthEnabled == nil || *cfg.Routes[0].Auth.APIKeyAuthEnabled != true {
		t.Error("routes[0].auth.api_key_auth_enabled should be true")
	}
	if cfg.Routes[1].Auth.APIKeyAuthEnabled == nil || *cfg.Routes[1].Auth.APIKeyAuthEnabled != false {
		t.Error("routes[1].auth.api_key_auth_enabled should be false")
	}
	if cfg.Routes[0].RateLimit.Enabled == nil || *cfg.Routes[0].RateLimit.Enabled != false {
		t.Error("routes[0].rate_limit.enabled should be false")
	}
	if cfg.Routes[0].TimeoutSeconds != 10 {
		t.Errorf("TimeoutSeconds = %d, want 10", cfg.Routes[0].TimeoutSeconds)
	}
	// 路由级覆盖辅助方法
	if !cfg.Routes[0].Auth.APIKeyAuthEnabledOr(false) {
		t.Error("APIKeyAuthEnabledOr should return route value true")
	}
	if cfg.Routes[1].Auth.APIKeyAuthEnabledOr(true) {
		t.Error("APIKeyAuthEnabledOr should return route value false")
	}
	// 全局按路径限流策略
	if p, ok := cfg.RateLimit.Routes["/api/v1/users"]; !ok || p.RequestsPerMinute != 500 {
		t.Errorf("rate_limit.routes[/api/v1/users] = %+v, want 500", p)
	}
}

func TestLoadConfigUnknownFieldRejected(t *testing.T) {
	// 严格模式: 未知字段 (拼写错误) 应报错而非静默忽略
	bad := validConfig + "\nserverr:\n  port: 1\n"
	if _, err := LoadConfig(writeTempConfig(t, bad)); err == nil {
		t.Error("expected error for unknown field, got nil")
	}
}

func TestValidateErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *Config)
		wantErr string
	}{
		{"非法服务端口", func(c *Config) { c.Server.Port = 0 }, "server.port"},
		{"端口越界", func(c *Config) { c.Server.Port = 70000 }, "server.port"},
		{"metrics与server同端口", func(c *Config) { c.Metrics.Port = 8080 }, "metrics.port"},
		{"TLS启用但缺证书", func(c *Config) { c.Server.TLS.Enabled = true }, "tls"},
		{"非法日志级别", func(c *Config) { c.Log.Level = "verbose" }, "log.level"},
		{"认证启用但无Key", func(c *Config) { c.Auth.APIKeyAuth.Keys = nil }, "keys"},
		{"认证启用但无Header名", func(c *Config) { c.Auth.APIKeyAuth.HeaderName = "" }, "header_name"},
		{"路由路径不以斜杠开头", func(c *Config) { c.Routes[0].Path = "api" }, "must start with '/'"},
		{"非法HTTP方法", func(c *Config) { c.Routes[0].Method = "FETCH" }, "method"},
		{"非法后端URL", func(c *Config) { c.Routes[0].Backend = "ftp://x" }, "backend"},
		{"后端URL为空", func(c *Config) { c.Routes[0].Backend = "" }, "backend"},
		{"超时为负", func(c *Config) { c.Routes[0].TimeoutSeconds = -1 }, "timeout_seconds"},
		{"重复路由", func(c *Config) {
			c.Routes = append(c.Routes, c.Routes[0])
		}, "duplicate route"},
	}

	base, err := LoadConfig(writeTempConfig(t, validConfig))
	if err != nil {
		t.Fatalf("base config load failed: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 深拷贝基础配置: 重新解析一遍, 避免用例间相互污染
			cfg, err := LoadConfig(writeTempConfig(t, validConfig))
			if err != nil {
				t.Fatal(err)
			}
			tt.mutate(cfg)
			err = cfg.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
	_ = base
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "config load failed") {
		t.Errorf("error = %q, want wrapped ErrConfigLoadFailed", err.Error())
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	_, err := LoadConfig(writeTempConfig(t, "server: [unterminated\n"))
	if err == nil {
		t.Fatal("expected error for invalid yaml")
	}
}
