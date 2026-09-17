package context

// ---- 编译层错误 ----

// Error 是编译层的错误（带协议错误码）。
// HTTP 层按 Code 映射状态码；回合编排按 Code 决定失败终态的 failure_code。
type Error struct {
	Code    string
	Message string
	Status  int
}

func (e *Error) Error() string { return e.Message }
