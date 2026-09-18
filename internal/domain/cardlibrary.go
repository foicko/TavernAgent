package domain

import "time"

// CharacterCardEntry 是角色卡库中的一张卡（用户资产）。
//
// 它与 TemplateVersion 的分工是本设计的关键：
//   - TemplateVersion 是「会话创建时冻结的不可变快照」，被会话引用，只读；
//   - CharacterCardEntry 是「用户可查看/复用/删除的资产」，可改名、记最近使用时间。
//
// 因此两者分表存储：从卡库移除一张卡只删本表记录，绝不影响任何已存在的会话
// （它们的角色内容仍在各自的 template_versions 与初始状态里）。
type CharacterCardEntry struct {
	CardID string `json:"cardId"`
	Name   string `json:"name"`
	// ShortName 是展示用短名（按常见连接符截取主名），由服务端派生。
	ShortName string `json:"shortName,omitempty"`
	Avatar    string `json:"avatar,omitempty"`
	Format    string `json:"format,omitempty"`
	// Role 是从描述中提取的身份标语（列表行的副标题）。
	Role string `json:"role,omitempty"`
	// CharacterJSON 是归一化后的完整角色卡。列表接口会置空它以避免
	// 把每张卡携带的兆级 JSON 一次性下发给前端。
	CharacterJSON string    `json:"characterJson,omitempty"`
	ContentHash   string    `json:"contentHash,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	// LastUsedAt 是最近一次用它创建会话的时间；nil 表示尚未使用。
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}
