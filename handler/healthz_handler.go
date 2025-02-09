package handler

import (
	"net/http"
)

// HealthzHandler 健康检查 Handler 结构体
type HealthzHandler struct {
}

// NewHealthzHandler 创建 HealthzHandler 实例
func NewHealthzHandler() *HealthzHandler {
	return &HealthzHandler{}
}

// ServeHTTP 实现 http.Handler 接口，处理健康检查请求
func (h *HealthzHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
