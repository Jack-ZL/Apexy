package errors

import "errors"

// 定义一些通用的错误
var (
	ErrConfigLoadFailed  = errors.New("config load failed")
	ErrRouteNotFound     = errors.New("route not found")
	ErrBackendError      = errors.New("backend service error")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
	// ... 可以根据需要添加更多自定义错误
)
