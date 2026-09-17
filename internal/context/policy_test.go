package context_test

import (
	"strconv"
	"testing"

	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
)

func makeFakeNodes(n int) []*domain.PlotNode {
	nodes := make([]*domain.PlotNode, 0, n+1)
	nodes = append(nodes, &domain.PlotNode{
		NodeID: "root",
		Kind:   domain.NodeKindRoot,
	})
	for i := 1; i <= n; i++ {
		nodes = append(nodes, &domain.PlotNode{
			NodeID:     "turn_" + strconv.Itoa(i),
			Kind:       domain.NodeKindTurn,
			TurnNumber: i,
		})
	}
	return nodes
}

func TestDecideCompaction_InsufficientHistory(t *testing.T) {
	policy := ctxpkg.CompactionPolicy{
		TailWindowTurns:     6,
		MinUncompactedTurns: 8,
	}
	nodes := makeFakeNodes(10) // 10 < 6 + 8
	decision := ctxpkg.DecideCompaction(nodes, nil, policy)
	if decision.ShouldCompact {
		t.Fatalf("expected shouldCompact=false, got true")
	}
	if decision.Reason != "insufficient-history" {
		t.Fatalf("unexpected reason: %s", decision.Reason)
	}
}

func TestDecideCompaction_Eligible(t *testing.T) {
	policy := ctxpkg.CompactionPolicy{
		TailWindowTurns:     6,
		MinUncompactedTurns: 8,
	}
	// 15 回合：可压缩区间为 0 .. (15-6) = 0..8（9 个回合）>= 8
	nodes := makeFakeNodes(15)
	decision := ctxpkg.DecideCompaction(nodes, nil, policy)
	if !decision.ShouldCompact {
		t.Fatalf("expected shouldCompact=true, got false: %s", decision.Reason)
	}
	if decision.FromNodeID != "turn_1" {
		t.Errorf("expected fromNodeID=turn_1, got %s", decision.FromNodeID)
	}
	if decision.ToNodeID != "turn_9" {
		t.Errorf("expected toNodeID=turn_9, got %s", decision.ToNodeID)
	}
	if len(decision.NodesToFold) != 9 {
		t.Errorf("expected 9 nodes to fold, got %d", len(decision.NodesToFold))
	}
}

func TestDecideCompaction_WithExistingSummary(t *testing.T) {
	policy := ctxpkg.CompactionPolicy{
		TailWindowTurns:     6,
		MinUncompactedTurns: 8,
	}
	// 已有摘要覆盖到 turn_8
	existing := []*domain.SummaryArtifact{
		{
			SummaryID:  "sum_1",
			FromNodeID: "turn_1",
			ToNodeID:   "turn_8",
		},
	}

	// 此时总共 23 轮：候选区间为 turn_9 .. turn_(23-6) = turn_9 .. turn_17（9 轮）>= 8
	nodes := makeFakeNodes(23)
	decision := ctxpkg.DecideCompaction(nodes, existing, policy)
	if !decision.ShouldCompact {
		t.Fatalf("expected shouldCompact=true, got false: %s", decision.Reason)
	}
	if decision.FromNodeID != "turn_9" {
		t.Errorf("expected fromNodeID=turn_9, got %s", decision.FromNodeID)
	}
	if decision.ToNodeID != "turn_17" {
		t.Errorf("expected toNodeID=turn_17, got %s", decision.ToNodeID)
	}
	if len(decision.NodesToFold) != 9 {
		t.Errorf("expected 9 nodes to fold, got %d", len(decision.NodesToFold))
	}

	// 若只有 18 轮：候选区间为 turn_9 .. turn_(18-6) = turn_9 .. turn_12（4 轮）< 8 -> 不压缩
	nodes18 := makeFakeNodes(18)
	decision18 := ctxpkg.DecideCompaction(nodes18, existing, policy)
	if decision18.ShouldCompact {
		t.Fatalf("expected shouldCompact=false for 18 nodes, got true")
	}
	if decision18.Reason != "below-threshold" {
		t.Errorf("unexpected reason: %s", decision18.Reason)
	}
}

func TestDefaultCompactionPolicy(t *testing.T) {
	def := ctxpkg.DefaultCompactionPolicy()
	if def.TailWindowTurns != 20 {
		t.Errorf("expected TailWindowTurns=20, got %d", def.TailWindowTurns)
	}
	if def.MinUncompactedTurns != 8 {
		t.Errorf("expected MinUncompactedTurns=8, got %d", def.MinUncompactedTurns)
	}
	if def.PruneThoughtDepth != 20 {
		t.Errorf("expected PruneThoughtDepth=20, got %d", def.PruneThoughtDepth)
	}

	// 25 回合：小于 20 + 8 = 28，历史不充足，不触发压缩
	nodes25 := makeFakeNodes(25)
	dec25 := ctxpkg.DecideCompaction(nodes25, nil, def)
	if dec25.ShouldCompact {
		t.Fatalf("expected shouldCompact=false for 25 turns under default policy, got true")
	}
	if dec25.Reason != "insufficient-history" {
		t.Fatalf("unexpected reason: %s", dec25.Reason)
	}

	// 30 回合：大于 28。可压缩区间为 turn_1 .. turn_(30-20) = turn_1 .. turn_10（共 10 轮）>= 8
	// 尾部 20 轮（turn_11 .. turn_30）受保护不被折叠
	nodes30 := makeFakeNodes(30)
	dec30 := ctxpkg.DecideCompaction(nodes30, nil, def)
	if !dec30.ShouldCompact {
		t.Fatalf("expected shouldCompact=true for 30 turns, got false: %s", dec30.Reason)
	}
	if dec30.FromNodeID != "turn_1" {
		t.Errorf("expected fromNodeID=turn_1, got %s", dec30.FromNodeID)
	}
	if dec30.ToNodeID != "turn_10" {
		t.Errorf("expected toNodeID=turn_10, got %s", dec30.ToNodeID)
	}
	if len(dec30.NodesToFold) != 10 {
		t.Errorf("expected 10 nodes to fold, got %d", len(dec30.NodesToFold))
	}

	// 45 回合：大于 28。可压缩区间为 turn_1 .. turn_(45-20) = turn_1 .. turn_25（共 25 轮）>= 8
	// 尾部 20 轮（turn_26 .. turn_45）受保护不被折叠
	nodes45 := makeFakeNodes(45)
	dec45 := ctxpkg.DecideCompaction(nodes45, nil, def)
	if !dec45.ShouldCompact {
		t.Fatalf("expected shouldCompact=true for 45 turns, got false: %s", dec45.Reason)
	}
	if dec45.FromNodeID != "turn_1" {
		t.Errorf("expected fromNodeID=turn_1, got %s", dec45.FromNodeID)
	}
	if dec45.ToNodeID != "turn_25" {
		t.Errorf("expected toNodeID=turn_25, got %s", dec45.ToNodeID)
	}
	if len(dec45.NodesToFold) != 25 {
		t.Errorf("expected 25 nodes to fold, got %d", len(dec45.NodesToFold))
	}
}
