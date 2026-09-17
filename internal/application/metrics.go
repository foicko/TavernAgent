package application

import (
	"sync/atomic"

	ctxpkg "tavernagent/internal/context"
)

// RuntimeMetrics 是进程级运行计数器（T0.2）。
//
// 存在的理由：此前项目没有任何运行读数——无法回答"这一轮花了多少 token"、
// "前缀缓存命中多少"、"压缩是否生效"。对标项目的做法是每层都配一个仪表盘
// （Reasonix 的 Usage + Pricing.Cost，LiveAgent 的 atomic 计数器 + /api/status）。
//
// 约束：
//  1. 只读观测，绝不参与任何业务判定，计数失败不影响正确性；
//  2. 必须是并发安全的原子计数（回合生成在 goroutine 中）；
//  3. 不引入第三方指标库——当前只需要进程内累计值。
type RuntimeMetrics struct {
	turnsAccepted    atomic.Int64
	turnsCommitted   atomic.Int64
	turnsFailed      atomic.Int64
	turnsCancelled   atomic.Int64
	continuations    atomic.Int64
	budgetExceeded   atomic.Int64
	memoriesInjected atomic.Int64
	// 上下文预算与摘要维护的观测（这批计数回答"模型为什么没看到某段历史"）：
	// droppedHistory/droppedSummaries 是两类被裁掉的材料，budgetUnfit 是承重摘要
	// 装不下导致的整轮失败，summaryBlocked/summaryNormalized 是模型输出质量的读数。
	budgetDroppedHistory   atomic.Int64
	budgetDroppedSummaries atomic.Int64
	budgetUnfit            atomic.Int64
	summaryBlocked         atomic.Int64
	summaryNormalized      atomic.Int64
	// 后台队列的关停读数：丢弃的 pending 与关闭后的提交。
	// 这类任务（记忆抽取/摘要维护）不重跑，静默丢弃等于那一轮永远没发生。
	queueDropped  atomic.Int64
	queueRejected atomic.Int64
	// 用量台账写入失败次数。台账不参与业务判定（写失败不阻断回合），
	// 但"账没记上"必须能被读到——否则磁盘满这类问题只剩一行日志。
	usageWriteFailures atomic.Int64
}

// MetricsSnapshot 是计数器的不可变快照。
type MetricsSnapshot struct {
	TurnsAccepted    int64 `json:"turnsAccepted"`
	TurnsCommitted   int64 `json:"turnsCommitted"`
	TurnsFailed      int64 `json:"turnsFailed"`
	TurnsCancelled   int64 `json:"turnsCancelled"`
	Continuations    int64 `json:"continuations"`
	BudgetExceeded   int64 `json:"budgetExceeded"`
	MemoriesInjected int64 `json:"memoriesInjected"`

	BudgetDroppedHistory   int64 `json:"budgetDroppedHistory"`
	BudgetDroppedSummaries int64 `json:"budgetDroppedSummaries"`
	BudgetUnfit            int64 `json:"budgetUnfit"`
	SummaryBlocked         int64 `json:"summaryBlocked"`
	SummaryNormalized      int64 `json:"summaryNormalized"`

	QueueDropped  int64 `json:"queueDropped"`
	QueueRejected int64 `json:"queueRejected"`

	UsageWriteFailures int64 `json:"usageWriteFailures"`
}

// Snapshot 返回当前计数快照。
func (m *RuntimeMetrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	return MetricsSnapshot{
		TurnsAccepted:    m.turnsAccepted.Load(),
		TurnsCommitted:   m.turnsCommitted.Load(),
		TurnsFailed:      m.turnsFailed.Load(),
		TurnsCancelled:   m.turnsCancelled.Load(),
		Continuations:    m.continuations.Load(),
		BudgetExceeded:   m.budgetExceeded.Load(),
		MemoriesInjected: m.memoriesInjected.Load(),

		BudgetDroppedHistory:   m.budgetDroppedHistory.Load(),
		BudgetDroppedSummaries: m.budgetDroppedSummaries.Load(),
		BudgetUnfit:            m.budgetUnfit.Load(),
		SummaryBlocked:         m.summaryBlocked.Load(),
		SummaryNormalized:      m.summaryNormalized.Load(),

		QueueDropped:  m.queueDropped.Load(),
		QueueRejected: m.queueRejected.Load(),

		UsageWriteFailures: m.usageWriteFailures.Load(),
	}
}

// UsageWriteFailed 记一次用量台账写入失败（由观测供应商在写失败时调用）。
func (m *RuntimeMetrics) UsageWriteFailed() {
	if m != nil {
		m.usageWriteFailures.Add(1)
	}
}

func (m *RuntimeMetrics) addAccepted() {
	if m != nil {
		m.turnsAccepted.Add(1)
	}
}
func (m *RuntimeMetrics) addCommitted() {
	if m != nil {
		m.turnsCommitted.Add(1)
	}
}
func (m *RuntimeMetrics) addFailed() {
	if m != nil {
		m.turnsFailed.Add(1)
	}
}
func (m *RuntimeMetrics) addCancelled() {
	if m != nil {
		m.turnsCancelled.Add(1)
	}
}
func (m *RuntimeMetrics) addContinuation() {
	if m != nil {
		m.continuations.Add(1)
	}
}
func (m *RuntimeMetrics) addQueueDropped() {
	if m != nil {
		m.queueDropped.Add(1)
	}
}

func (m *RuntimeMetrics) addQueueRejected() {
	if m != nil {
		m.queueRejected.Add(1)
	}
}

func (m *RuntimeMetrics) addBudgetExceeded() {
	if m != nil {
		m.budgetExceeded.Add(1)
	}
}
func (m *RuntimeMetrics) addMemories(n int) {
	if m != nil && n > 0 {
		m.memoriesInjected.Add(int64(n))
	}
}

// AddBudgetReport 接入 CompilerOptions.OnBudget：把"这次编译裁掉了什么"
// 变成可读的运行读数。此前 BudgetReport 全仓没有生产消费者，
// 静默丢历史因此完全不可见。
func (m *RuntimeMetrics) AddBudgetReport(report ctxpkg.BudgetReport) {
	if m == nil {
		return
	}
	if report.DroppedHistory > 0 {
		m.budgetDroppedHistory.Add(int64(report.DroppedHistory))
	}
	if report.DroppedSummaries > 0 {
		m.budgetDroppedSummaries.Add(int64(report.DroppedSummaries))
	}
}

func (m *RuntimeMetrics) addBudgetUnfit() {
	if m != nil {
		m.budgetUnfit.Add(1)
	}
}

func (m *RuntimeMetrics) addSummaryBlocked() {
	if m != nil {
		m.summaryBlocked.Add(1)
	}
}

// addSummaryNormalized 记录"归一器修好了模型的结构漂移"的次数。
// 宽容不能是静默的：这个计数涨得快，说明提示词该改，而不是归一器在兜底。
func (m *RuntimeMetrics) addSummaryNormalized() {
	if m != nil {
		m.summaryNormalized.Add(1)
	}
}
