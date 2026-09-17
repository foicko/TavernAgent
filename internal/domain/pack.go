package domain

// PackFormatVersion 是剧情包的格式版本（manifest 的 formatVersion）。
// 版本变化意味着文件布局或字段语义有不兼容改动，导入端据此拒绝或迁移。
const PackFormatVersion = 1

// Director events require v2 readers; stories without them keep v1 portability.
const DirectorPackFormatVersion = 2

func SupportedPackVersion(v int) bool {
	return v == PackFormatVersion || v == DirectorPackFormatVersion
}

func PackVersionForEvents(events []*DomainEvent) int {
	for _, e := range events {
		if e != nil && e.Type == EventDirectorChange {
			return DirectorPackFormatVersion
		}
	}
	return PackFormatVersion
}

// SessionBundle 是一个会话的完整可携带形态（剧情包的领域侧表示）。
//
// 它是纯数据快照，刻意**不含**：
//   - 草稿帧与进行中的回合（草稿不是历史，放弃后即作废）
//   - API Key、访问会话与日志（T28：不得出现在剧情包中）
//   - 全量事件之外的派生物（FTS 索引等可在导入后重建）
//
// 记忆单独成文件而不是靠事件重建：用户的修订（纠正/置顶/隐藏）是
// copy-on-write 的覆盖记录，不产生领域事件，只能随包携带。
type SessionBundle struct {
	FormatVersion  int
	RulesetVersion string
	AppVersion     string
	ExportedAt     string

	// Scope 记录导出范围。
	Scope ExportScope

	Session    *Session
	RootNodeID string
	Templates  []*TemplateVersion
	Nodes      []*PlotNode
	Events     []*DomainEvent
	Branches   []*Branch
	Memories   []*MemoryRecord
	Receipts   []PortableReceipt
	Bookmarks  []Bookmark
	// Snapshots 是稀疏检查点（可选）。导入时若校验通过可直接采用，
	// 否则由 StateAt 用「根快照 + 事件重放」重建。
	Snapshots []*StateSnapshot
}

// PortableReceipt links a committed ruling to its immutable result node.
type PortableReceipt struct {
	NodeID  string         `json:"nodeId"`
	Receipt *ActionReceipt `json:"receipt"`
}

// ExportScope 描述导出范围（整会话，或仅某条分支及其必要祖先）。
type ExportScope struct {
	Full       bool
	BranchID   string
	BranchName string
}
