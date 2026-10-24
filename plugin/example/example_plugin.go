package example

import (
	"net/http"

	"apexy/log"
)

// ExamplePlugin 示例插件: 在请求处理前后打印日志, 演示插件的接入方式
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
		// 使用网关统一的日志组件 (而非 fmt.Println), 保证日志格式/级别/输出目标一致
		log.Debugf("Example Plugin Before Handler: %s %s, Config: %v", r.Method, r.URL.Path, p.config)
		next.ServeHTTP(w, r) // 调用下一个 Handler
		log.Debugf("Example Plugin After Handler: %s %s", r.Method, r.URL.Path)
	})
}
