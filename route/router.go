package route

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"apexy/config"
	"apexy/log"
)

// contextKey 路由上下文的私有 Key 类型, 避免与其他包的 context Key 冲突
type contextKey struct{}

// WithRoute 将匹配到的路由规则写入请求上下文。
// HTTPHandler 在路由匹配成功后调用, 下游中间件 (认证、限流、日志、指标)
// 可通过 FromContext 读取路由级配置, 实现"路由级配置覆盖全局配置"。
func WithRoute(ctx context.Context, r *config.Route) context.Context {
	return context.WithValue(ctx, contextKey{}, r)
}

// FromContext 从请求上下文中读取匹配到的路由规则, 未匹配到时返回 nil
func FromContext(ctx context.Context) *config.Route {
	r, _ := ctx.Value(contextKey{}).(*config.Route)
	return r
}

// Router 结构体，负责路由管理
type Router struct {
	routes      []config.Route // 路由规则列表
	routesMutex sync.RWMutex   // 读写锁，保护路由列表的并发访问
}

// NewRouter 创建 Router 实例
func NewRouter() *Router {
	return &Router{
		routes:      []config.Route{},
		routesMutex: sync.RWMutex{},
	}
}

// LoadRoutesFromConfig 从配置加载路由规则
func (r *Router) LoadRoutesFromConfig(routesConfig []config.Route) error {
	r.routesMutex.Lock()
	defer r.routesMutex.Unlock()

	r.routes = make([]config.Route, len(routesConfig)) // 创建新的路由列表
	copy(r.routes, routesConfig)                       // 复制路由配置

	log.Infof("Loaded %d routes from config", len(r.routes))
	return nil
}

// MatchRoute 匹配路由，根据请求信息查找匹配的路由规则
func (r *Router) MatchRoute(req *http.Request) *config.Route {
	r.routesMutex.RLock() // 读锁，允许多个 Reader 同时读取路由列表
	defer r.routesMutex.RUnlock()

	for _, route := range r.routes {
		if matchRoute(req, &route) { // 调用 matchRoute 函数进行路由匹配
			return &route // 找到匹配的路由，返回路由规则的指针
		}
	}
	return nil // 没有找到匹配的路由
}

// 获取所有路由规则 (只读访问)
func (r *Router) GetRoutes() []config.Route {
	r.routesMutex.RLock()
	defer r.routesMutex.RUnlock()
	routes := make([]config.Route, len(r.routes))
	copy(routes, r.routes)
	return routes
}

// matchRoute 路由匹配函数，判断请求是否匹配路由规则
// 匹配规则:
//  1. Method 为空则匹配所有 HTTP 方法, 否则必须完全一致 (忽略大小写)
//  2. Path 支持两种匹配方式:
//     - 精确匹配: 请求路径与路由路径完全相等
//     - 前缀匹配: 请求路径以路由路径为前缀, 且必须在路径段边界处截断
//     例如路由 /api/v1/users 可以匹配 /api/v1/users 和 /api/v1/users/123,
//     但不会错误匹配 /api/v1/users-extra (非路径段边界)
func matchRoute(req *http.Request, route *config.Route) bool {
	if route.Path == "" { // 路由未配置 Path, 视为无效路由
		return false
	}

	if route.Method != "" && !strings.EqualFold(req.Method, route.Method) { // 检查 HTTP 方法是否匹配 (如果路由规则配置了 Method)
		return false
	}

	// 精确匹配
	if req.URL.Path == route.Path {
		return true
	}

	// 前缀匹配: 请求路径必须以路由路径为前缀
	if !strings.HasPrefix(req.URL.Path, route.Path) {
		return false
	}

	// 路由路径以 "/" 结尾 (如 /api/v1/), 任意更长的路径都属于该前缀下, 直接匹配成功
	if strings.HasSuffix(route.Path, "/") {
		return true
	}

	// 路由路径不以 "/" 结尾时, 要求前缀之后的第一个字符必须是 "/",
	// 保证在路径段边界处匹配 (避免 /api/v1/users 错误匹配 /api/v1/users-extra)
	return req.URL.Path[len(route.Path)] == '/'

	// TODO:  可以根据需要添加更多匹配条件 (例如 Header, Query Parameters, 正则表达式、AntPath)
}
