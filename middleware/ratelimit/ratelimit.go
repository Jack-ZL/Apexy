package ratelimit

import (
	"apexy/middleware"
	"net/http"
	"strings"
	"sync"
	"time"

	"apexy/config"
	"apexy/log"
)

// RateLimitMiddleware 速率限制 Middleware
func RateLimitMiddleware(cfg config.RateLimitConfig) middleware.MiddlewareFunc {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler {
			return next // 如果全局速率限制未启用，则直接跳过
		}
	}

	// 使用 map 存储每个 IP 的令牌桶
	buckets := make(map[string]*tokenBucket)
	bucketsMutex := &sync.Mutex{} // 保护 buckets map 的并发访问

	// 令牌桶填充定时器，每分钟填充令牌
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			bucketsMutex.Lock()
			for _, bucket := range buckets {
				bucket.refill() // 补充令牌
			}
			bucketsMutex.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.General.RequestsPerMinute <= 0 { // 速率限制策略无效
				next.ServeHTTP(w, r)
				return
			}

			ip := getClientIP(r) // 获取客户端 IP 地址
			if ip == "" {
				log.Warnf("Failed to get client IP address")
				http.Error(w, "Rate limit error", http.StatusTooManyRequests)
				return
			}

			bucketsMutex.Lock()
			bucket, ok := buckets[ip]
			if !ok { // 如果 IP 对应的令牌桶不存在，则创建新的令牌桶
				bucket = newTokenBucket(cfg.General.RequestsPerMinute)
				buckets[ip] = bucket
			}
			bucketsMutex.Unlock()

			if bucket.allow() { // 尝试获取令牌
				next.ServeHTTP(w, r) // 允许请求通过
			} else {
				log.Warnf("Rate limit exceeded for IP: %s", ip)
				http.Error(w, "Too many requests", http.StatusTooManyRequests) // 拒绝请求
				return
			}
		})
	}
}

// tokenBucket 令牌桶结构体
type tokenBucket struct {
	tokens     int        // 当前令牌数量
	capacity   int        // 令牌桶容量
	refillRate int        // 令牌补充速率 (每分钟补充令牌数)
	lastRefill time.Time  // 上次补充令牌时间
	mutex      sync.Mutex // 互斥锁，保护令牌桶的并发访问
}

// newTokenBucket 创建令牌桶实例
func newTokenBucket(capacity int) *tokenBucket {
	return &tokenBucket{
		capacity:   capacity,
		tokens:     capacity, // 初始令牌数等于容量
		refillRate: capacity, // 令牌补充速率等于容量 (每分钟补充 capacity 个令牌)
		lastRefill: time.Now(),
	}
}

// allow 尝试从令牌桶中获取令牌，如果成功返回 true，否则返回 false
func (b *tokenBucket) allow() bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.refill() // 尝试补充令牌

	if b.tokens > 0 {
		b.tokens--  // 令牌数减 1
		return true // 获取令牌成功
	}
	return false // 令牌不足，获取令牌失败
}

// refill 补充令牌
func (b *tokenBucket) refill() {
	now := time.Now()
	elapsedTime := now.Sub(b.lastRefill)                     // 计算距离上次补充令牌的时间间隔
	tokensToAdd := int(elapsedTime.Minutes()) * b.refillRate // 计算应该补充的令牌数
	if tokensToAdd > 0 {
		b.tokens += tokensToAdd
		if b.tokens > b.capacity {
			b.tokens = b.capacity // 令牌数不能超过容量
		}
		b.lastRefill = now // 更新上次补充令牌时间
	}
}

// getClientIP 获取客户端 IP 地址
func getClientIP(r *http.Request) string {
	ip := r.Header.Get("X-Real-IP") // 优先从 X-Real-IP Header 中获取
	if ip == "" {
		ip = r.Header.Get("X-Forwarded-For") // 从 X-Forwarded-For Header 中获取 (可能包含多个 IP，取第一个)
		if ip != "" {
			ips := strings.SplitN(ip, ",", 1)
			ip = ips[0]
		} else {
			ip = r.RemoteAddr // 从 RemoteAddr 中获取
			ipPort := strings.SplitN(ip, ":", 2)
			ip = ipPort[0] // 移除端口号
		}
	}
	return ip
}
