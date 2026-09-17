package context

import (
	"fmt"

	"tavernagent/internal/domain"
)

func (c *Compiler) foldSummarizedHistory(history []*domain.PlotNode, summaries []*domain.SummaryArtifact) ([]*domain.PlotNode, error) {
	tail := c.options.CompactionPolicy.TailWindowTurns
	if tail <= 0 {
		tail = DefaultCompactionPolicy().TailWindowTurns
	}
	cutoff := len(history) - tail
	if cutoff <= 0 {
		return history, nil
	}
	byID := map[string]*domain.PlotNode{}
	for _, node := range history {
		byID[node.NodeID] = node
	}
	get := func(id string) (*domain.PlotNode, error) {
		if node := byID[id]; node != nil {
			return node, nil
		}
		node, err := c.store.GetNode(id)
		if err != nil || node == nil {
			return nil, fmt.Errorf("read summary source %s: %v", id, err)
		}
		byID[id] = node
		return node, nil
	}
	folded := map[string]bool{}
	for _, summary := range summaries {
		from, err := get(summary.FromNodeID)
		if err != nil {
			return nil, err
		}
		to, err := get(summary.ToNodeID)
		if err != nil {
			return nil, err
		}
		for _, node := range history[:cutoff] {
			if node.Depth >= from.Depth && node.Depth <= to.Depth {
				folded[node.NodeID] = true
			}
		}
	}
	out := make([]*domain.PlotNode, 0, len(history))
	for _, node := range history {
		if !folded[node.NodeID] {
			out = append(out, node)
		}
	}
	return out, nil
}

// summaryRedundant 判断一条摘要覆盖的区间是否**已经完整出现在保留的历史里**。
//
// 这是"摘要不可静默消失"这条不变量的唯一判据，三种情况一套逻辑：
//   - 区间原文已被折叠移除 → 不冗余（摘要就是那段历史仅存的载体）
//   - 区间比加载到的历史更早（从未进入窗口）→ 不冗余（同理）
//   - 区间内的回合都还在历史里 → 冗余（只是重复的副本，丢弃不丢信息）
//
// 读不到端点（存储缺失或测试用 nil store）时按"不冗余"处理：宁可报错，
// 也不要把唯一载体丢掉。
func (c *Compiler) summaryRedundant(summary *domain.SummaryArtifact, retained []*domain.PlotNode, cache map[string][2]int) bool {
	if summary == nil {
		return false
	}
	if c.store == nil {
		return false
	}
	present := map[int]bool{}
	minDepth, maxDepth := 0, 0
	for _, node := range retained {
		if node == nil || node.Kind != domain.NodeKindTurn {
			continue
		}
		present[node.Depth] = true
		if minDepth == 0 || node.Depth < minDepth {
			minDepth = node.Depth
		}
		if node.Depth > maxDepth {
			maxDepth = node.Depth
		}
	}
	if len(present) == 0 {
		return false
	}
	span, ok := cache[summary.SummaryID]
	if !ok {
		from, err := c.store.GetNode(summary.FromNodeID)
		if err != nil || from == nil {
			return false
		}
		to, err := c.store.GetNode(summary.ToNodeID)
		if err != nil || to == nil {
			return false
		}
		span = [2]int{from.Depth, to.Depth}
		cache[summary.SummaryID] = span
	}
	if span[0] > span[1] {
		return false
	}
	// 深度差超过保留历史总跨度时不可能完整覆盖，直接判不冗余（也避免长循环）。
	if span[1]-span[0] > maxDepth-minDepth {
		return false
	}
	for depth := span[0]; depth <= span[1]; depth++ {
		if !present[depth] {
			return false
		}
	}
	return true
}
