package application

import (
	"tavernagent/internal/domain"
)

// SessionCapabilities 是"当前能做什么"的服务端投影（架构设计 §2.2）。
//
// 它只回答**服务端权威**的部分：分支上是否有在途回合、查看位置是否落后于
// 分支头、当前节点是否是一个回合。客户端本地的瞬时状态（用户是否主动在
// 浏览历史、自己刚发出的请求是否在途、提交后是否尚未刷新）不属于这里——
// 那些由客户端与本投影做一次合取，合取规则只有一条：
//
//	可用 = capabilities 允许 && 无本地阻塞
//
// 这样新增一个"服务端认可的状态"时，两端都不需要再去逐个改按钮。
type SessionCapabilities struct {
	// Busy 是分支上存在未终态回合（生成中 / 待续写 / 待确认）。
	Busy bool `json:"busy"`
	// ActiveTurnID 是在途回合 ID（Busy 为真时非空）。
	ActiveTurnID string `json:"activeTurnId,omitempty"`
	// CanSend 为假时输入台不可提交（服务端原因见 Block）。
	CanSend bool `json:"canSend"`
	// CanFork 表示可以从当前查看位置派生新分支（查看位置不等于分支头）。
	CanFork bool `json:"canFork"`
	// AtTurnNode 表示查看位置是一个回合节点（而非根节点）。
	AtTurnNode bool `json:"atTurnNode"`
	// ViewingHistory 表示查看位置落后于分支头（历史只读浏览）。
	// 它是**服务端可知**的事实，与"用户主动浏览历史"的本地标志共同决定只读。
	ViewingHistory bool `json:"viewingHistory"`
	// Block 是服务端侧的首要阻塞原因；为空表示服务端没有阻塞。
	Block *CapabilityBlock `json:"block,omitempty"`
}

// CapabilityBlock 是阻塞原因（界面直接展示 message，不再自行拼文案）。
type CapabilityBlock struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// sessionCapabilities 推导会话能力。
//
// 只依赖分支与查看节点：不引入新的存储读取，因此不会给视图接口增加查询成本。
func sessionCapabilities(branch *domain.Branch, viewNode *domain.PlotNode) SessionCapabilities {
	caps := SessionCapabilities{
		CanSend:    true,
		AtTurnNode: viewNode != nil && viewNode.Kind == domain.NodeKindTurn,
	}
	if branch != nil && branch.ActiveTurnID != "" {
		caps.Busy = true
		caps.ActiveTurnID = branch.ActiveTurnID
		caps.CanSend = false
		caps.Block = &CapabilityBlock{
			Code:    "TURN_IN_FLIGHT",
			Message: "上一回合仍在生成，完成或取消后才能提交新的输入",
		}
	}
	// 查看位置落后于分支头即为历史浏览：可读、可分叉，但提交前要先回到分支头。
	if branch != nil && viewNode != nil && viewNode.NodeID != branch.HeadNodeID {
		caps.ViewingHistory = true
		caps.CanFork = true
	}
	return caps
}

// TurnCapabilities 是单个回合的服务端能力投影。
//
// 与会话能力分开：回合级能力只取决于该回合的状态机位置，且只有持有回合的
// 服务能回答（会话视图不认识 TurnStore）。
type TurnCapabilities struct {
	// CanCancel 表示该回合仍在途，可以取消。
	CanCancel bool `json:"canCancel"`
	// CanContinue 表示该回合停在"待续写"，可以用续写继续。
	CanContinue bool `json:"canContinue"`
	// CanRetry 表示该回合已失败且可重试（retryable 由失败原因决定）。
	CanRetry bool `json:"canRetry"`
	// Terminal 表示回合已终态（committed / cancelled / failed / conflicted）。
	Terminal bool `json:"terminal"`
}

// CapabilitiesOfTurn 是回合能力的对外入口（HTTP 层只做协议转换）。
//
// 失败与冲突都可重试：真正的可重试性由失败原因决定，但那属于 SSE 失败事件
// 的载荷（携带 retryable）；这里只回答"状态机是否允许重试"，避免两处各判一次。
func CapabilitiesOfTurn(turn *domain.TurnRequest) TurnCapabilities {
	if turn == nil {
		return TurnCapabilities{}
	}
	return turnCapabilities(turn.Status)
}

func turnCapabilities(status domain.TurnStatus) TurnCapabilities {
	return TurnCapabilities{
		CanCancel:   !status.Terminal(),
		CanContinue: status == domain.TurnAwaitingContinuation,
		CanRetry:    status == domain.TurnFailed || status == domain.TurnConflicted,
		Terminal:    status.Terminal(),
	}
}
