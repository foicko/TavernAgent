// Package id 生成 UUIDv7 标识（技术契约 §2：外部 ID 不透明，时间信息不作为排序依据）。
package id

import "github.com/google/uuid"

// New 返回新的 UUIDv7 字符串。
func New() string {
	u, err := uuid.NewV7()
	if err != nil {
		// 极不可能失败；回退到 UUIDv4 保证可用性
		return uuid.NewString()
	}
	return u.String()
}
