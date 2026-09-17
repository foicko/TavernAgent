package application

import (
	"context"
	"testing"

	"tavernagent/internal/domain"
	"tavernagent/internal/protocol"
)

// ---- H2b：直接提交与模型回合走同一条路径 ----

// 直接提交（编辑助手正文用的入口）必须留下 outbox 事件与可查询的终态。
// 早前 BranchService 自己复刻的精简版没有这些——那正是 H2b 要消除的差异。
func TestSubmitBlocksProducesCommittedEvent(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(2))
	acceptAndWait(t, turnSvc, st, sessionID, branchID, "sb-1", "你好。")

	br, err := st.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	node, turn, err := turnSvc.SubmitBlocks(context.Background(), SubmitBlocksRequest{
		SessionID: sessionID, BranchID: branchID,
		HeadID: br.HeadNodeID, Version: br.Version,
		Blocks: []domain.TextBlock{{Kind: "narration", Text: "直接提交的正文。"}},
		Input:  domain.TurnInput{Kind: "text", Text: "你好。"},
		Mode:   protocol.ModeNarrative,
	})
	if err != nil {
		t.Fatalf("submit blocks: %v", err)
	}
	if node == nil || turn == nil {
		t.Fatalf("应返回节点与回合")
	}

	// 终态可查询，且指向结果节点。
	got, err := st.GetTurn(turn.TurnID)
	if err != nil {
		t.Fatalf("get turn: %v", err)
	}
	if got.Status != domain.TurnCommitted {
		t.Fatalf("终态 = %s, want committed", got.Status)
	}
	if got.ResultNodeID != node.NodeID {
		t.Fatalf("resultNodeId = %q, want %q", got.ResultNodeID, node.NodeID)
	}

	// outbox 里有完成事件（精简版缺失的部分）。
	evs, err := st.PollOutbox(turn.TurnID, 0, 100)
	if err != nil {
		t.Fatalf("poll outbox: %v", err)
	}
	var sawCommitted bool
	for _, e := range evs {
		if e.Type == "turn.committed" {
			sawCommitted = true
		}
	}
	if !sawCommitted {
		t.Fatalf("直接提交应留下 turn.committed 事件，实际 %+v", evs)
	}

	// 分支头推进到结果节点，回合的规则版本也被固定。
	if hb, _ := st.GetBranch(branchID); hb.HeadNodeID != node.NodeID {
		t.Fatalf("分支头 = %q, want %q", hb.HeadNodeID, node.NodeID)
	}
	if got.RulesetVersion != RulesetVersion {
		t.Fatalf("直接提交也应固定规则版本，实际 %q", got.RulesetVersion)
	}
}

// 直接提交失败时必须落到明确终态，而不是静默留空。
func TestSubmitBlocksFailsWithTerminalState(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(2))
	acceptAndWait(t, turnSvc, st, sessionID, branchID, "sb-2", "你好。")

	br, err := st.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}

	// 基准头不存在 → 提交前就失败，不产生回合。
	if _, _, err := turnSvc.SubmitBlocks(context.Background(), SubmitBlocksRequest{
		SessionID: sessionID, BranchID: branchID,
		HeadID: "node_missing", Version: br.Version,
		Blocks: []domain.TextBlock{{Kind: "narration", Text: "x"}},
		Mode:   protocol.ModeNarrative,
	}); err == nil {
		t.Fatalf("基准头不存在时应报错")
	}

	// 版本不符 → CAS 冲突，回合落到 conflicted 终态。
	_, turn, err := turnSvc.SubmitBlocks(context.Background(), SubmitBlocksRequest{
		SessionID: sessionID, BranchID: branchID,
		HeadID: br.HeadNodeID, Version: br.Version + 99,
		Blocks: []domain.TextBlock{{Kind: "narration", Text: "x"}},
		Mode:   protocol.ModeNarrative,
	})
	if err == nil {
		t.Fatalf("版本不符应报错")
	}
	if turn == nil {
		t.Fatalf("CAS 冲突应返回回合以供查询终态")
	}
	got, gerr := st.GetTurn(turn.TurnID)
	if gerr != nil {
		t.Fatalf("get turn: %v", gerr)
	}
	if got.Status != domain.TurnConflicted {
		t.Fatalf("终态 = %s, want conflicted（不能静默留空）", got.Status)
	}
	// 冲突不得推进分支头。
	if hb, _ := st.GetBranch(branchID); hb.HeadNodeID != br.HeadNodeID {
		t.Fatalf("冲突不应推进分支头")
	}
}

// 编辑助手正文改走统一入口后，行为与之前一致（零状态变化、mode=narrative）。
func TestEditAssistantTextStillNarrativeAfterUnify(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(2))
	svc := NewBranchService(st, turnSvc)

	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "sb-3", "你好。")
	second := acceptAndWait(t, turnSvc, st, sessionID, branchID, "sb-4", "继续。")
	beforeAff := affectionOf(t, st, second.ResultNodeID)

	res, err := svc.EditAssistantText(context.Background(), sessionID, AssistantEditRequest{
		NodeID: second.ResultNodeID,
		Blocks: []domain.TextBlock{{Kind: "narration", Text: "她什么也没说。"}},
	})
	if err != nil {
		t.Fatalf("edit assistant: %v", err)
	}

	// 零状态变化：候选节点的好感等于 parent，而不是叠加原提议。
	if got := affectionOf(t, st, res.Node.NodeID); got != affectionOf(t, st, first.ResultNodeID) {
		t.Fatalf("改写正文应按纯叙事候选提交（零状态变化），好感 = %d want %d",
			got, affectionOf(t, st, first.ResultNodeID))
	}
	if got := affectionOf(t, st, second.ResultNodeID); got != beforeAff {
		t.Fatalf("原节点状态被改动: %d", got)
	}
	// 节点标记为 narrative，且无状态事件。
	evs, err := st.GetEvents(res.Node.NodeID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	for _, ev := range evs {
		if ev.Type == domain.EventRelationshipDelta {
			t.Fatalf("纯叙事候选不应产生状态事件")
		}
	}
}
