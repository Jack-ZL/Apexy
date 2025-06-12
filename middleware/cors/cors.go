package cors

// Package cors 实现 CORS (跨域资源共享) Middleware，解决跨域问题
import (
	"apexy/middleware"
	"net/http"
	"strings"

	"apexy/config"
	"apexy/log"
)

// CORSMiddleware CORS (跨域资源共享) Middleware
func CORSMiddleware(cfg config.CORSConfig) middleware.MiddlewareFunc {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler {
			return next // 如果 CORS 未启用，则直接跳过
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin") // 获取请求中的 Origin Header
			if origin == "" {
				next.ServeHTTP(w, r) // 非跨域请求，直接跳过 CORS 处理
				return
			}

			allowedOrigin := false
			for _, allowed := range cfg.AllowedOrigins {
				if allowed == "*" || allowed == origin {
					allowedOrigin = true
					break
				}
			}

			if !allowedOrigin {
				log.Warnf("CORS blocked: Origin %s is not allowed", origin)
				http.Error(w, "CORS origin not allowed", http.StatusForbidden)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)                                   // 允许的 Origin
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowedMethods, ", "))  // 允许的方法
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))  // 允许的 Headers
			w.Header().Set("Access-Control-Expose-Headers", strings.Join(cfg.ExposedHeaders, ", ")) // 暴露的 Headers
			if cfg.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true") // 允许携带 Credentials (例如 Cookie)
			}

			if r.Method == http.MethodOptions {
				// 预检请求 (Preflight Request)
				w.WriteHeader(http.StatusOK)
				return
			}

			// 继续处理下一个 Handler
			next.ServeHTTP(w, r)
		})
	}
}
