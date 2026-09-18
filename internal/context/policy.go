package context

import (
	"tavernagent/internal/domain"
)

// CompactionPolicy 定义上下文压缩与修剪的触发策略。
// 借鉴 LiveAgent 的分段保护与冷却阶梯机制。
type CompactionPolicy struct {
	// TailWindowTurns 是活跃尾部保护窗口（轮数）。
	// 处于尾部窗口内的最新回合保持 100% 原始上下文（含 <thought> 心声与选项），
	// 保证当下的接话自然度与即时记忆。为适配现代 128K~300K 大模型，默认放宽至 20 轮。
	TailWindowTurns int

	// MinUncompactedTurns 是触发新压缩所需的最小未压缩轮数。
	// 只有当历史中未被摘要覆盖、且超出尾部窗口的回合数达到此阈值时才触发。
	// 默认放宽至 20 轮，避免频繁切分细碎摘要，保证大段史诗章节的整体感。
	MinUncompactedTurns int

	// PruneThoughtDepth 表示从当前 head 往回倒数超过该深度的历史节点，
	// 在 Compile 时自动剥离 <thought> 块（节省 30%~50% 历史 Token）。
	// 默认放宽至 20 轮，让近期的角色内心活动充分保留。
	PruneThoughtDepth int

	// Disabled 彻底关闭压缩（消融用，ADS-7.8-01）。
	// 关掉后 DecideCompaction 恒判"不压缩"，后台不再生成新摘要；
	// 已存在的摘要是否仍注入由 CompilerOptions.DisableSummaries 单独控制——
	// 两个开关分开，才能区分"不生成"与"不使用"两种消融配置。
	Disabled bool
}

// DefaultCompactionPolicy 返回推荐的跑团演义压缩策略默认值。
// 针对现代 128K~300K 上下文模型深度优化：
// 最新 20 轮绝对保护、折叠批次 8 轮（累积达到 28 轮后触发小批次提炼）、心声保留 20 轮。
func DefaultCompactionPolicy() CompactionPolicy {
	return CompactionPolicy{
		TailWindowTurns:     20,
		MinUncompactedTurns: 8,
		PruneThoughtDepth:   20,
	}
}

// CompactionDecision 描述一次压缩决策的判定结果。
type CompactionDecision struct {
	// ShouldCompact 为 true 表示应当触发后台压缩。
	ShouldCompact bool
	// Reason 记录判定原因（如 "below-threshold", "eligible" 等）。
	Reason string
	// FromNodeID 是本次待压缩区间的起始节点（时间上更早）。
	FromNodeID string
	// ToNodeID 是本次待压缩区间的结束节点（时间上更晚，仍在不可变祖先路径上）。
	ToNodeID string
	// NodesToFold 是本次将被提炼压缩的具体回合节点列表（按拓扑时序由旧到新排列）。
	NodesToFold []*domain.PlotNode
}

// DecideCompaction 根据给定的祖先链与路径上已有的摘要，评估是否需要启动新的历史压缩。
// ancestors 必须是拓扑升序（从 root 到当前 headNode）。
func DecideCompaction(ancestors []*domain.PlotNode, existingSummaries []*domain.SummaryArtifact, policy CompactionPolicy) *CompactionDecision {
	if policy.Disabled {
		return &CompactionDecision{ShouldCompact: false, Reason: "disabled"}
	}
	if policy.TailWindowTurns <= 0 {
		policy.TailWindowTurns = 20
	}
	if policy.MinUncompactedTurns <= 0 {
		policy.MinUncompactedTurns = 8
	}

	// 1. 过滤并提取祖先链上的所有 Turn 节点（按时序从根到叶）
	var turnNodes []*domain.PlotNode
	for _, n := range ancestors {
		if n.Kind == domain.NodeKindTurn {
			turnNodes = append(turnNodes, n)
		}
	}

	totalTurns := len(turnNodes)
	// 如果总回合数还没有超出保护尾部 + 最小压缩量，直接不触发
	if totalTurns < policy.TailWindowTurns+policy.MinUncompactedTurns {
		return &CompactionDecision{
			ShouldCompact: false,
			Reason:        "insufficient-history",
		}
	}

	// Select the first uncovered interval, including gaps left by invalidated
	// summaries. A later independent summary cannot hide an earlier gap.
	positions := map[string]int{}
	for i, n := range ancestors {
		positions[n.NodeID] = i
	}
	covered := map[string]bool{}
	for _, s := range existingSummaries {
		from, fok := positions[s.FromNodeID]
		to, tok := positions[s.ToNodeID]
		if !fok || !tok || from > to {
			continue
		}
		for _, n := range ancestors[from : to+1] {
			covered[n.NodeID] = true
		}
	}
	startIdx := 0
	endIdx := totalTurns - policy.TailWindowTurns
	for startIdx < endIdx && covered[turnNodes[startIdx].NodeID] {
		startIdx++
	}
	for i := startIdx; i < endIdx; i++ {
		if covered[turnNodes[i].NodeID] {
			endIdx = i
			break
		}
	}

	uncompactedCount := endIdx - startIdx
	if uncompactedCount < policy.MinUncompactedTurns {
		return &CompactionDecision{
			ShouldCompact: false,
			Reason:        "below-threshold",
		}
	}

	nodesToFold := turnNodes[startIdx:endIdx]
	return &CompactionDecision{
		ShouldCompact: true,
		Reason:        "eligible",
		FromNodeID:    nodesToFold[0].NodeID,
		ToNodeID:      nodesToFold[len(nodesToFold)-1].NodeID,
		NodesToFold:   nodesToFold,
	}
}
