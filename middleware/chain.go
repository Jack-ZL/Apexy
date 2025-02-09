package middleware

import "net/http"

// MiddlewareFunc 定义 Middleware 函数类型
type MiddlewareFunc func(http.Handler) http.Handler

// ChainBuilder 用于构建 Middleware 链
type ChainBuilder struct {
	middlewares []MiddlewareFunc
}

// NewChainBuilder 创建 ChainBuilder 实例
func NewChainBuilder() *ChainBuilder {
	return &ChainBuilder{
		middlewares: []MiddlewareFunc{},
	}
}

// AddMiddleware 添加 Middleware 到链
func (cb *ChainBuilder) AddMiddleware(middleware MiddlewareFunc) {
	cb.middlewares = append(cb.middlewares, middleware)
}

// Build 构建 Middleware 链，将 MiddlewareFunc 数组链接成一个 http.Handler
func (cb *ChainBuilder) Build(finalHandler http.Handler) http.Handler {
	// 从后往前 Wrap Handler, 形成链式调用，类似洋葱模型
	handler := finalHandler
	for i := len(cb.middlewares) - 1; i >= 0; i-- {
		handler = cb.middlewares[i](handler)
	}
	return handler
}
