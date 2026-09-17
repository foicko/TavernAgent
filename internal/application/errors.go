// Package application 编排会话、分支、回合调度、恢复与预算。
// 应用层使用领域类型与端口，不直接依赖数据库/网络/模型 SDK（技术契约 §1）。
package application

import "errors"

// APIError 携带稳定错误码（技术契约 §11.2）。
type APIError struct {
	Code       string
	Message    string
	Retryable  bool
	StatusCode int
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

// NewError 构造 API 错误。
func NewError(code, message string, retryable bool, status int) *APIError {
	return &APIError{Code: code, Message: message, Retryable: retryable, StatusCode: status}
}

var (
	ErrIdempotencyConflict = errors.New("idempotency conflict")
	ErrHeadConflict        = errors.New("head conflict")
	ErrStaleOption         = errors.New("stale option")
	ErrQueueFull           = errors.New("queue full")
	ErrNotFound            = errors.New("not found")
	ErrCancelled           = errors.New("cancelled")
	ErrProtocolInvalid     = errors.New("protocol invalid")
	ErrOperationRejected   = errors.New("operation rejected")
	ErrStateMismatch       = errors.New("state mismatch")
)

// 常用 API 错误实例。
func Err(code, message string, status int) *APIError {
	switch code {
	case "IDEMPOTENCY_CONFLICT":
		return NewError(code, message, false, statusOverride(status, 409))
	case "HEAD_CONFLICT", "STALE_OPTION":
		return NewError(code, message, true, statusOverride(status, 409))
	case "QUEUE_FULL":
		return NewError(code, message, false, statusOverride(status, 429))
	case "PROVIDER_UNAVAILABLE":
		return NewError(code, message, true, statusOverride(status, 502))
	case "PROTOCOL_INVALID", "OPERATION_REJECTED":
		return NewError(code, message, false, statusOverride(status, 422))
	case "BUDGET_EXCEEDED":
		return NewError(code, message, true, statusOverride(status, 507))
	case "STORAGE_UNAVAILABLE":
		return NewError(code, message, false, statusOverride(status, 503))
	default:
		return NewError(code, message, false, statusOverride(status, 400))
	}
}

func statusOverride(status, fallback int) int {
	if status > 0 {
		return status
	}
	return fallback
}
