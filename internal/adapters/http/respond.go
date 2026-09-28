// 响应工具：JSON 成功体与统一错误体（形状即前端契约）。
//
// 从 server.go 抽出来：响应体形状被前端与测试共同依赖，集中一处便于逐字比对。
package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"tavernagent/internal/application"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	// 先编码再写头：编码失败（NaN/Inf、不支持的类型等）时尚未发出 200，
	// 可以干净地回 500，而不是留下 200 + 截断体。
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("响应编码失败: %v", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"INTERNAL","message":"服务内部错误，请稍后重试","retryable":true,"turnId":""}` + "\n"))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}

func writeError(w http.ResponseWriter, status int, code, message string, retryable bool, turnID string) {
	body := map[string]any{
		"code": code, "message": message, "retryable": retryable, "turnId": turnID,
	}
	// traceId 与响应头 X-Request-ID 同值（由 withLogging 预先写入）：用户报错时
	// 直接给出错误体里的 ID 即可对上服务端日志。不用 requestId 作键，避免与导演
	// 请求的业务字段 requestId 混淆。
	if rid := w.Header().Get(requestIDHeader); rid != "" {
		body["traceId"] = rid
	}
	writeJSON(w, status, body)
}

func writeAPIError(w http.ResponseWriter, err error) {
	// 用 errors.As 而非直接断言：APIError 可能被 %w 包裹后传上来。
	var appErr *application.APIError
	if errors.As(err, &appErr) {
		writeError(w, appErr.StatusCode, appErr.Code, appErr.Message, appErr.Retryable, "")
		return
	}
	// 未知错误：内部细节只进日志，对外统一给可操作的中文文案，不泄漏实现细节。
	log.Printf("内部错误 req=%s: %v", w.Header().Get(requestIDHeader), err)
	writeError(w, 500, "INTERNAL", "服务内部错误，请稍后重试；若持续出现请查看服务端日志", true, "")
}
