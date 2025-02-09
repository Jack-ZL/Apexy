package plugin

import "net/http"

// Plugin 插件接口
type Plugin interface {
	Name() string                          // 插件名称
	Handle(next http.Handler) http.Handler // 插件处理逻辑
}
