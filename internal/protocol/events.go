package protocol

// SSE / outbox 事件名常量与清单。
//
// 唯一契约来源是 testdata/contract/sse-events.json：Go 的契约测试直接读它，
// TS 与 Kotlin 的清单由 scripts/gen_sse_contract.py 生成，避免三端事件名漂移。
// 发出事件处仍可写字符串字面量——契约测试会扫描源码，任何不在清单里的事件名都会让它失败。
const (
	EventTurnStarted    = "turn.started"
	EventTurnThinking   = "turn.thinking"
	EventBlockDelta     = "block.delta"
	EventBlockAppended  = "block.appended"
	EventTurnTruncated  = "turn.truncated"
	EventTurnCommitted  = "turn.committed"
	EventTurnCancelled  = "turn.cancelled"
	EventTurnFailed     = "turn.failed"
	EventTurnConflicted = "turn.conflicted"

	EventDirectorStarted     = "director.started"
	EventDirectorCompleted   = "director.completed"
	EventDirectorFailed      = "director.failed"
	EventDirectorCancelled   = "director.cancelled"
	EventDirectorInterrupted = "director.interrupted"

	EventTurnStateUpdated = "turn.state.updated"
)

// TurnEvents 是发给客户端的回合事件全集。
var TurnEvents = []string{
	EventTurnStarted, EventTurnThinking, EventBlockDelta, EventBlockAppended,
	EventTurnTruncated, EventTurnCommitted, EventTurnCancelled, EventTurnFailed, EventTurnConflicted,
}

// TerminalTurnEvents 是回合终态事件（到达即结束订阅）。
var TerminalTurnEvents = []string{
	EventTurnCommitted, EventTurnCancelled, EventTurnFailed, EventTurnConflicted,
}

// DirectorEvents 是导演请求事件全集。
var DirectorEvents = []string{
	EventDirectorStarted, EventDirectorCompleted, EventDirectorFailed,
	EventDirectorCancelled, EventDirectorInterrupted,
}

// InternalEvents 是仅用于服务端 outbox/广播、不作为 SSE 契约暴露的事件。
var InternalEvents = []string{EventTurnStateUpdated}
