package auth

import (
	"apexy/middleware"
	"net/http"
	"strings"

	"apexy/config"
	"apexy/log"
)

// APIKeyAuthMiddleware API Key 认证 Middleware
func APIKeyAuthMiddleware(cfg config.APIKeyAuthConfig) middleware.MiddlewareFunc {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler {
			return next // 如果 API Key 认证未启用，则直接跳过
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get(cfg.HeaderName) // 从 Header 中获取 API Key
			if apiKey == "" {
				apiKey = r.URL.Query().Get(cfg.HeaderName) // 尝试从 Query 参数中获取
			}

			if apiKey == "" {
				log.Warnf("API Key not found in request")
				http.Error(w, "API Key is required", http.StatusUnauthorized)
				return
			}

			validKey := false
			for _, validAPIKey := range cfg.Keys {
				if apiKey == validAPIKey {
					validKey = true
					break
				}
			}

			if !validKey {
				log.Warnf("Invalid API Key: %s", apiKey)
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
