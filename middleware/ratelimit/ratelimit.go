package ratelimit

import (
	"apexy/middleware"
	"apexy/pkg/errors"
	"apexy/route"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"apexy/config"
	"apexy/log"
)

// 令牌桶清理策略: 每分钟扫描一次, 清理闲置超过 10 分钟的桶。
// 闲置阈值远大于限流窗口 (1 分钟), 保证被清理的桶即使重建也不会放大配额。
const (
	bucketCleanupInterval = time.Minute
	bucketIdleTimeout     = 10 * time.Minute
)

// RateLimitMiddleware 速率限制 Middleware (基于令牌桶算法, 按客户端 IP 限流)
// 支持路由级配置覆盖全局配置, 策略优先级从高到低:
//  1. 路由内联配置 route.rate_limit (enabled / policy.requests_per_minute)
//  2. 全局配置中按路径匹配的策略 rate_limit.routes[path]
//  3. 全局通用策略 rate_limit.general
func RateLimitMiddleware(cfg config.RateLimitConfig) middleware.MiddlewareFunc {
	// 使用 map 存储每个 "IP + 策略作用域" 的令牌桶。
	// Key 中加入作用域是因为不同路由可能有不同的限流阈值, 必须各自独立计数。
	buckets := make(map[string]*tokenBucket)
	bucketsMutex := &sync.Mutex{} // 保护 buckets map 的并发访问

	// 注意: 这里不再使用全局定时器统一补充令牌。
	// 令牌桶在每次 allow() 时按流逝时间惰性补充 (lazy refill), 定时器不仅多余,
	// 而且之前在不持有 bucket.mutex 的情况下调用 refill(), 存在数据竞争。

	// 过期令牌桶清理: 每个 IP 的桶会永久驻留 map 中, 长期运行会导致内存无限增长。
	// 这里启动一个后台协程定期清理长时间无请求的桶 (超过 bucketIdleTimeout 未活跃)。
	// 清理是安全的: 桶闲置超过一个完整限流窗口后令牌必然已充满, 删除后重建等价于原状态。
	go func() {
		ticker := time.NewTicker(bucketCleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			bucketsMutex.Lock()
			for key, bucket := range buckets {
				if bucket.idleSince(now) > bucketIdleTimeout {
					delete(buckets, key)
				}
			}
			bucketsMutex.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 逐请求解析生效的限流开关、阈值和作用域 (路由级配置优先于全局配置)。
			// 不能根据全局 cfg.Enabled 提前短路, 否则路由级的覆盖配置无法生效。
			enabled, requestsPerMinute, scope := resolvePolicy(cfg, r)

			if !enabled || requestsPerMinute <= 0 { // 未启用限流或策略无效, 直接放行
				next.ServeHTTP(w, r)
				return
			}

			ip := getClientIP(r, cfg.TrustForwardedHeaders) // 获取客户端 IP 地址
			if ip == "" {
				log.Warnf("Failed to get client IP address")
				http.Error(w, fmt.Sprintf("%v: cannot resolve client IP", errors.ErrRateLimitExceeded), http.StatusTooManyRequests)
				return
			}

			bucketKey := ip + "|" + scope // 同一 IP 在不同限流作用域下使用独立的令牌桶
			bucketsMutex.Lock()
			bucket, ok := buckets[bucketKey]
			if !ok { // 如果令牌桶不存在，则按当前生效阈值创建新的令牌桶
				bucket = newTokenBucket(requestsPerMinute)
				buckets[bucketKey] = bucket
			}
			bucketsMutex.Unlock()

			if bucket.allow() { // 尝试获取令牌
				next.ServeHTTP(w, r) // 允许请求通过
			} else {
				log.Warnf("Rate limit exceeded for IP: %s, scope: %s", ip, scope)
				http.Error(w, fmt.Sprintf("%v: too many requests", errors.ErrRateLimitExceeded), http.StatusTooManyRequests) // 拒绝请求
				return
			}
		})
	}
}

// resolvePolicy 解析当前请求最终生效的限流配置, 返回 (是否启用, 每分钟请求数, 作用域)。
// 作用域用于隔离不同策略的令牌桶: 全局通用策略为 "general", 路由特定策略为路由路径。
func resolvePolicy(cfg config.RateLimitConfig, r *http.Request) (enabled bool, requestsPerMinute int, scope string) {
	// 默认使用全局配置
	enabled = cfg.Enabled
	requestsPerMinute = cfg.General.RequestsPerMinute
	scope = "general"

	routeEntry := route.FromContext(r.Context())
	if routeEntry == nil {
		return // 请求上下文中没有路由信息, 只能使用全局配置
	}

	// 优先级 2: 全局配置中按路径匹配的策略 rate_limit.routes[path]
	if policy, ok := cfg.Routes[routeEntry.Path]; ok && policy.RequestsPerMinute > 0 {
		requestsPerMinute = policy.RequestsPerMinute
		scope = routeEntry.Path
	}

	// 优先级 1 (最高): 路由内联配置 route.rate_limit
	enabled = routeEntry.RateLimit.EnabledOr(enabled) // 路由级开关覆盖全局开关
	if routeEntry.RateLimit.Policy.RequestsPerMinute > 0 {
		requestsPerMinute = routeEntry.RateLimit.Policy.RequestsPerMinute // 路由级阈值覆盖全局阈值
		scope = routeEntry.Path
	}

	return
}

// tokenBucket 令牌桶结构体
type tokenBucket struct {
	tokens     float64    // 当前令牌数量 (使用浮点数按时间比例精确补充, 避免整数截断丢失令牌)
	capacity   int        // 令牌桶容量
	refillRate int        // 令牌补充速率 (每分钟补充令牌数)
	lastRefill time.Time  // 上次补充令牌时间
	mutex      sync.Mutex // 互斥锁，保护令牌桶的并发访问
}

// newTokenBucket 创建令牌桶实例
func newTokenBucket(capacity int) *tokenBucket {
	return &tokenBucket{
		capacity:   capacity,
		tokens:     float64(capacity), // 初始令牌数等于容量
		refillRate: capacity,          // 令牌补充速率等于容量 (每分钟补充 capacity 个令牌)
		lastRefill: time.Now(),
	}
}

// allow 尝试从令牌桶中获取令牌，如果成功返回 true，否则返回 false
func (b *tokenBucket) allow() bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.refill() // 尝试补充令牌

	if b.tokens >= 1 {
		b.tokens--  // 令牌数减 1
		return true // 获取令牌成功
	}
	return false // 令牌不足，获取令牌失败
}

// refill 按距离上次补充的流逝时间, 成比例地补充令牌 (惰性补充)。
// 修复: 之前使用 int(elapsed.Minutes()) 整数截断, 不足一分钟的流逝时间不会补充任何令牌,
// 且 lastRefill 被整体推进导致这部分时间被永久丢弃, 实际限流比配置更严格。
func (b *tokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(b.lastRefill)
	if elapsed <= 0 {
		return
	}
	// 按秒级精度成比例补充: 每分钟补充 refillRate 个令牌
	b.tokens += elapsed.Seconds() / 60 * float64(b.refillRate)
	if b.tokens > float64(b.capacity) {
		b.tokens = float64(b.capacity) // 令牌数不能超过容量
	}
	b.lastRefill = now // 更新上次补充令牌时间
}

// idleSince 返回令牌桶自上次活动 (补充令牌) 以来闲置的时长, 供清理协程判断是否过期
func (b *tokenBucket) idleSince(now time.Time) time.Duration {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return now.Sub(b.lastRefill)
}

// getClientIP 获取客户端 IP 地址。
// trustForwarded 控制是否信任 X-Real-IP / X-Forwarded-For 请求头:
//   - false (默认, 安全): 只使用 TCP 连接的 RemoteAddr。这两个 Header 可被客户端任意伪造,
//     直接信任会导致攻击者每次请求换一个伪造 IP 即可完全绕过限流。
//   - true: 仅当网关部署在可信反向代理 (Nginx/云 LB, 由代理覆写这些 Header) 之后时开启。
func getClientIP(r *http.Request, trustForwarded bool) string {
	if trustForwarded {
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			return strings.TrimSpace(ip)
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// XFF 可能包含多级代理追加的 IP 列表, 第一个是最初的客户端 IP
			ips := strings.SplitN(xff, ",", 2)
			if ip := strings.TrimSpace(ips[0]); ip != "" {
				return ip
			}
		}
	}
	// 从 RemoteAddr 中获取。使用 net.SplitHostPort 兼容 IPv6 地址
	// (修复: 之前用 ":" 切分, IPv6 地址如 [::1]:8080 会被解析错误)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr // RemoteAddr 不含端口时直接使用
	}
	return host
}
