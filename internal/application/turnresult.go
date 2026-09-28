package application

import "tavernagent/internal/domain"

// 回合受理/派生/编辑的对外结果。此前这些是 HTTP 层临时拼的 map[string]any，
// 没有可反射的 Go 类型，导致三端 DTO 只能手写、易漂移；改成具名 struct 后
// 由 cmd/gencontract 生成 TS/Kotlin（见 docs/spec/06-dto.md）。
//
// 注意：这里的 DeriveResponse / AssistantEditResponse 是**对外 wire 类型**，
// 与 branch.go 里的服务返回值 DeriveResult / AssistantEditResult 不同（后者携带领域对象）。

// AcceptResult 是 POST .../turns 的受理结果。
type AcceptResult struct {
	TurnID    string            `json:"turnId"`
	Status    domain.TurnStatus `json:"status"`
	StatusURL string            `json:"statusUrl"`
	EventsURL string            `json:"eventsUrl"`
}

// DeriveResponse 是分支派生（分叉 / 重生成 / 编辑玩家输入）的受理结果。
type DeriveResponse struct {
	Branch     BranchView        `json:"branch"`
	BranchID   string            `json:"branchId"`
	TurnID     string            `json:"turnId"`
	Status     domain.TurnStatus `json:"status"`
	StatusURL  string            `json:"statusUrl,omitempty"`
	EventsURL  string            `json:"eventsUrl,omitempty"`
	HeadNodeID string            `json:"headNodeId,omitempty"`
}

// AssistantEditResponse 是编辑助手正文的结果：同步提交的纯叙事候选。
type AssistantEditResponse struct {
	Branch   BranchView `json:"branch"`
	BranchID string     `json:"branchId"`
	NodeID   string     `json:"nodeId"`
	Mode     string     `json:"mode"`
}

// BranchViewOf 把领域分支转为对外展示视图（不含 sessionId）。
func BranchViewOf(b *domain.Branch) BranchView {
	if b == nil {
		return BranchView{}
	}
	return BranchView{
		BranchID:     b.BranchID,
		Name:         b.Name,
		HeadNodeID:   b.HeadNodeID,
		Version:      b.Version,
		ActiveTurnID: b.ActiveTurnID,
	}
}
