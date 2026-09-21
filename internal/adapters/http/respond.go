// 响应工具：JSON 成功体与统一错误体（形状即前端契约）。
//
// 从 server.go 抽出来：响应体形状被前端与测试共同依赖，集中一处便于逐字比对。
package http

import (
	"encoding/json"
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
	if appErr, ok := err.(*application.APIError); ok {
		writeError(w, appErr.StatusCode, appErr.Code, appErr.Message, appErr.Retryable, "")
		return
	}
	writeError(w, 500, "INTERNAL", err.Error(), true, "")
}
