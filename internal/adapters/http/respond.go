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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string, retryable bool, turnID string) {
	writeJSON(w, status, map[string]any{
		"code": code, "message": message, "retryable": retryable, "turnId": turnID,
	})
}

func writeAPIError(w http.ResponseWriter, err error) {
	// 用 errors.As 而非直接断言：APIError 可能被 %w 包裹后传上来。
	var appErr *application.APIError
	if errors.As(err, &appErr) {
		writeError(w, appErr.StatusCode, appErr.Code, appErr.Message, appErr.Retryable, "")
		return
	}
	// 未知错误：内部细节只进日志，对外统一给可操作的中文文案，不泄漏实现细节。
	log.Printf("内部错误: %v", err)
	writeError(w, 500, "INTERNAL", "服务内部错误，请稍后重试；若持续出现请查看服务端日志", true, "")
}
