package plugin

import (
	"fmt"
	"net/http"
	"sync"
)

// Plugin 插件接口。
// 插件本质上是一个中间件: 接收下游 Handler, 返回包装后的 Handler,
// 可以在请求前后插入自定义逻辑 (鉴权、改写、统计等)。
type Plugin interface {
	Name() string                          // 插件名称
	Handle(next http.Handler) http.Handler // 插件处理逻辑
}

// registry 全局插件注册表。
// 插件通过 Register 注册 (通常在 init() 或 main 中显式调用),
// 网关启动时通过 Plugins 取出并按注册顺序接入中间件链。
var (
	registryMu sync.Mutex
	registry   []Plugin
)

// Register 注册一个插件。重复注册同名插件会返回错误, 避免插件被意外执行两次。
func Register(p Plugin) error {
	registryMu.Lock()
	defer registryMu.Unlock()

	for _, existing := range registry {
		if existing.Name() == p.Name() {
			return fmt.Errorf("plugin %q is already registered", p.Name())
		}
	}
	registry = append(registry, p)
	return nil
}

// MustRegister 注册插件, 失败时 panic (用于程序启动阶段的显式注册)
func MustRegister(p Plugin) {
	if err := Register(p); err != nil {
		panic(err)
	}
}

// Plugins 返回已注册插件列表的副本 (按注册顺序)
func Plugins() []Plugin {
	registryMu.Lock()
	defer registryMu.Unlock()

	plugins := make([]Plugin, len(registry))
	copy(plugins, registry)
	return plugins
}
