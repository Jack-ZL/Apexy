package cors

// Package cors 实现 CORS (跨域资源共享) Middleware，解决跨域问题
import (
	"net/http"
	"strconv"
	"strings"

	"apexy/config"
	"apexy/log"
	"apexy/middleware"
)

// defaultMaxAgeSeconds 预检结果默认缓存时间 (秒)
const defaultMaxAgeSeconds = 600

// CORSMiddleware CORS (跨域资源共享) Middleware
func CORSMiddleware(cfg config.CORSConfig) middleware.MiddlewareFunc {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler {
			return next // 如果 CORS 未启用，则直接跳过
		}
	}

	// 预计算 Max-Age 字符串, 避免每个请求重复转换
	maxAge := cfg.MaxAgeSeconds
	if maxAge <= 0 {
		maxAge = defaultMaxAgeSeconds
	}
	maxAgeStr := strconv.Itoa(maxAge)

	// 启动时检查危险配置组合并告警:
	// AllowCredentials + 通配 Origin 意味着任意网站都能携带用户凭证发起跨域请求。
	// 下方 origin 匹配逻辑在 AllowCredentials=true 时会拒绝 "*" 通配, 只允许显式列出的 Origin。
	if cfg.AllowCredentials && contains(cfg.AllowedOrigins, "*") {
		log.Warnf("CORS: allow_credentials=true with wildcard origin '*' is insecure; " +
			"wildcard will be ignored, only explicitly listed origins are allowed with credentials")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin") // 获取请求中的 Origin Header
			if origin == "" {
				next.ServeHTTP(w, r) // 非跨域请求，直接跳过 CORS 处理
				return
			}

			// 无论允许与否都声明 Vary: Origin, 防止缓存层把某个 Origin 的响应
			// 错误地返回给其他 Origin (缓存投毒)
			w.Header().Add("Vary", "Origin")

			if !isOriginAllowed(origin, cfg) {
				log.Warnf("CORS blocked: Origin %s is not allowed", origin)
				http.Error(w, "CORS origin not allowed", http.StatusForbidden)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)                                   // 允许的 Origin (回显具体 Origin 而非 "*", 以兼容凭证模式)
			w.Header().Set("Access-Control-Expose-Headers", strings.Join(cfg.ExposedHeaders, ", ")) // 暴露的 Headers
			if cfg.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true") // 允许携带 Credentials (例如 Cookie)
			}

			if r.Method == http.MethodOptions {
				// 预检请求 (Preflight Request): 需要额外校验浏览器声明的
				// Access-Control-Request-Method / Access-Control-Request-Headers 是否在允许范围内
				reqMethod := r.Header.Get("Access-Control-Request-Method")
				if reqMethod != "" && !isAllowed(reqMethod, cfg.AllowedMethods) {
					log.Warnf("CORS preflight blocked: method %s is not allowed", reqMethod)
					http.Error(w, "CORS method not allowed", http.StatusForbidden)
					return
				}
				reqHeaders := r.Header.Get("Access-Control-Request-Headers")
				if reqHeaders != "" && !areHeadersAllowed(reqHeaders, cfg.AllowedHeaders) {
					log.Warnf("CORS preflight blocked: headers %s are not allowed", reqHeaders)
					http.Error(w, "CORS headers not allowed", http.StatusForbidden)
					return
				}

				w.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowedMethods, ", ")) // 允许的方法
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", ")) // 允许的 Headers
				w.Header().Set("Access-Control-Max-Age", maxAgeStr)                                    // 预检结果缓存时间, 减少重复预检请求
				w.WriteHeader(http.StatusNoContent)                                                    // 预检成功返回 204 No Content (无响应体)
				return
			}

			// 简单请求/实际请求也带上 Allow-Methods/Allow-Headers, 保持响应头一致
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowedMethods, ", "))
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))

			// 继续处理下一个 Handler
			next.ServeHTTP(w, r)
		})
	}
}

// isOriginAllowed 判断请求 Origin 是否在白名单中。
// 安全约束: 当 AllowCredentials=true 时, 通配符 "*" 不生效,
// 必须显式列出允许的 Origin, 防止任意网站携带用户凭证跨域访问。
func isOriginAllowed(origin string, cfg config.CORSConfig) bool {
	for _, allowed := range cfg.AllowedOrigins {
		if allowed == origin { // 精确匹配
			return true
		}
		if allowed == "*" && !cfg.AllowCredentials { // 通配仅在非凭证模式下允许
			return true
		}
	}
	return false
}

// isAllowed 判断 value 是否在允许列表中 (列表包含 "*" 时允许任意值, 忽略大小写)
func isAllowed(value string, allowedList []string) bool {
	for _, allowed := range allowedList {
		if allowed == "*" || strings.EqualFold(allowed, value) {
			return true
		}
	}
	return false
}

// areHeadersAllowed 校验预检请求声明的 Access-Control-Request-Headers (逗号分隔)
// 是否全部在允许列表中
func areHeadersAllowed(requestHeaders string, allowedHeaders []string) bool {
	for _, h := range strings.Split(requestHeaders, ",") {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if !isAllowed(h, allowedHeaders) {
			return false
		}
	}
	return true
}

// contains 判断字符串切片中是否包含指定值
func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
