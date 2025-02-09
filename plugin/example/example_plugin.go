package example

import (
	"fmt"
	"net/http"
)

// ExamplePlugin 示例插件
type ExamplePlugin struct {
	name   string
	config map[string]interface{} // 插件配置
}

// NewExamplePlugin 创建示例插件实例
func NewExamplePlugin(config map[string]interface{}) *ExamplePlugin {
	return &ExamplePlugin{
		name:   "ExamplePlugin",
		config: config,
	}
}

// Name 插件名称
func (p *ExamplePlugin) Name() string {
	return p.name
}

// Handle 插件处理逻辑
func (p *ExamplePlugin) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Example Plugin Before Handler, Config:", p.config) // 插件处理逻辑 (Before Handler)
		next.ServeHTTP(w, r)                                            // 调用下一个 Handler
		fmt.Println("Example Plugin After Handler")                     // 插件处理逻辑 (After Handler)
	})
}
