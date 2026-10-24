package handler

import (
	"apexy/log"
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
	writeRes, err := w.Write([]byte("OK"))
	if err != nil {
		// 修复: 之前写失败会 panic 打挂整个网关进程。健康检查写失败
		// (通常是客户端提前断开) 属于可恢复错误, 记录日志即可, 不应中断服务。
		log.Errorf("Healthz write failed: %v", err)
		return
	}
	// 健康检查通常由 LB/K8s 每秒级轮询, 使用 Debug 级别避免刷屏
	log.Debugf("Healthz check responded with %d bytes", writeRes)
}
