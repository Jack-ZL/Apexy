package route

import (
	"net/http"
	"sync"

	"apexy/config"
	"apexy/log"
)

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
func matchRoute(req *http.Request, route *config.Route) bool {
	if route.Method != "" && req.Method != route.Method { // 检查 HTTP 方法是否匹配 (如果路由规则配置了 Method)
		return false
	}

	if route.Path != "" && req.URL.Path != route.Path { // 检查 URL 路径是否匹配 (精确匹配)
		//  TODO:  可以根据需要支持更复杂的路径匹配 (例如 AntPath, 正则表达式)
		return false
	}

	//  TODO:  可以根据需要添加更多匹配条件 (例如 Header, Query Parameters, 自定义匹配规则)

	return true // 路由匹配成功
}
