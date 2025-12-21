package errors

import "errors"

// 定义一些通用的错误
var (
	ErrConfigLoadFailed   = errors.New("config load failed")
	ErrRouteNotFound      = errors.New("route not found")
	ErrBackendError       = errors.New("backend service error")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrRateLimitExceeded  = errors.New("rate limit exceeded")
	ErrInvalidRequest     = errors.New("invalid request")
	ErrTimeout            = errors.New("request timeout")
	ErrInternalServer     = errors.New("internal server error")
	ErrBadGateway         = errors.New("bad gateway")
	ErrServiceUnavailable = errors.New("service unavailable")
	ErrGatewayTimeout     = errors.New("gateway timeout")
	ErrTooManyRequests    = errors.New("too many requests")
	ErrForbidden          = errors.New("forbidden")
	ErrNotImplemented     = errors.New("not implemented")
	ErrConflict           = errors.New("conflict")
	// ... 可以根据需要添加更多自定义错误
)
