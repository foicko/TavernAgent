package sqlite

import (
	"encoding/json"
	"log/slog"
)

// decodeIDList 容错解析存储的 JSON 字符串数组（owners/entities 等）。
//
// 历史库可能包含旧版本写入的坏数据，读取端不应因此整体失败；这里保持"按空值
// 继续"的容错语义，但把解析失败显式告警出来，避免数据静默丢失。
func decodeIDList(raw, field string, dst *[]string) {
	if raw == "" {
		return
	}
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		slog.Warn("损坏的 JSON 列表已按空值容错", "field", field, "error", err)
	}
}
