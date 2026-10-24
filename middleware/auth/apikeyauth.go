package auth

import (
	"apexy/middleware"
	"apexy/pkg/errors"
	"apexy/route"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"apexy/config"
	"apexy/log"
)

// APIKeyAuthMiddleware API Key 认证 Middleware
// 支持路由级配置覆盖全局配置:
//   - 全局 enabled=false 时, 若某路由显式配置 api_key_auth_enabled: true, 该路由仍会启用认证
//   - 全局 enabled=true 时, 若某路由显式配置 api_key_auth_enabled: false, 该路由跳过认证
func APIKeyAuthMiddleware(cfg config.APIKeyAuthConfig) middleware.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 逐请求判断认证开关: 路由级配置 (来自请求上下文) 优先于全局配置。
			// 注意: 不能在这里根据全局 cfg.Enabled 提前短路, 否则路由级的覆盖配置无法生效。
			enabled := cfg.Enabled
			if routeEntry := route.FromContext(r.Context()); routeEntry != nil {
				enabled = routeEntry.Auth.APIKeyAuthEnabledOr(cfg.Enabled)
			}
			if !enabled {
				next.ServeHTTP(w, r) // 该请求无需 API Key 认证, 直接放行
				return
			}

			apiKey := r.Header.Get(cfg.HeaderName) // 从 Header 中获取 API Key
			if apiKey == "" {
				apiKey = r.URL.Query().Get(cfg.HeaderName) // 尝试从 Query 参数中获取
			}
			apiKey = trimApiKey(apiKey) // 去除前后空格, 避免复制粘贴引入的空白字符导致认证失败

			if apiKey == "" {
				log.Warnf("API Key not found in request")
				http.Error(w, fmt.Sprintf("%v: API Key is required", errors.ErrUnauthorized), http.StatusUnauthorized)
				return
			}

			validKey := false
			for _, validAPIKey := range cfg.Keys {
				// 使用常数时间比较, 防止通过响应时间差异逐字节猜解 API Key (时序攻击)。
				// 先比较长度是安全的: 长度本身不是秘密, 且 subtle.ConstantTimeCompare
				// 对长度不同的输入会直接返回 0。
				if subtle.ConstantTimeCompare([]byte(apiKey), []byte(validAPIKey)) == 1 {
					validKey = true
					break
				}
			}

			if !validKey {
				// 日志脱敏: 只记录 Key 的前 4 位和长度, 避免完整 Key (可能是用户误粘贴的有效凭证) 泄漏到日志
				log.Warnf("Invalid API Key: %s", maskAPIKey(apiKey))
				http.Error(w, "Invalid API Key", http.StatusUnauthorized)
				return
			}

			// API Key 验证通过，继续处理下一个 Handler
			next.ServeHTTP(w, r)
		})
	}
}

// trimApiKey 去除 API Key 的前后空格
func trimApiKey(apiKey string) string {
	return strings.TrimSpace(apiKey)
}

// maskAPIKey 对 API Key 脱敏: 保留前 4 位, 其余用 "*" 代替, 并附上原始长度。
// 例如 "abcdefgh" -> "abcd****(len=8)", 便于排查问题又不泄漏完整凭证。
func maskAPIKey(apiKey string) string {
	const prefixLen = 4
	if len(apiKey) <= prefixLen {
		return fmt.Sprintf("%s****(len=%d)", apiKey[:1], len(apiKey))
	}
	return fmt.Sprintf("%s****(len=%d)", apiKey[:prefixLen], len(apiKey))
}
