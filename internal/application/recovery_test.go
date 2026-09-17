package application

import (
	"context"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/domain"
)

// ---- E6/E7：可恢复性（T12 / T13）----

// T12：提交成功后即使丢失完成事件，也能通过查询找回原节点，
// 且不会触发第二次生成或第二次结算。
func TestCommittedTurnRecoverableWithoutRegeneration(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, affectionScript(2))

	turn := acceptAndWait(t, turnSvc, st, sessionID, branchID, "r-1", "你好。")
	if turn.ResultNodeID == "" {
		t.Fatalf("提交后应有结果节点")
	}
	countBefore := countNodes(t, st, branchID)

	// 重连：按 turnId 查询终态（等价于 GET /turns/{id}）。
	got, err := turnSvc.Get(turn.TurnID)
	if err != nil {
		t.Fatalf("get turn: %v", err)
	}
	if got.Status != domain.TurnCommitted || got.ResultNodeID != turn.ResultNodeID {
		t.Fatalf("重连后终态不一致: %+v", got)
	}
	// Outbox 里的完成事件可被重放补发（SSE Last-Event-ID 的数据来源）。
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
		t.Fatalf("缺少 turn.committed 事件，重连无法补发: %+v", evs)
	}

	// 重放不产生第二次生成：节点数不变。
	if after := countNodes(t, st, branchID); after != countBefore {
		t.Fatalf("重连触发了额外生成: %d → %d", countBefore, after)
	}

	// 同幂等键重发返回同一回合（不新建尝试）。
	again, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "r-1", ExpectedHeadID: turn.ExpectedHeadID, ExpectedVersion: turn.ExpectedVersion,
		Input: domain.TurnInput{Kind: "text", Text: "你好。"},
	})
	if err != nil {
		t.Fatalf("重发: %v", err)
	}
	if again.TurnID != turn.TurnID {
		t.Fatalf("同幂等键应复用原回合: %s vs %s", again.TurnID, turn.TurnID)
	}
	if after := countNodes(t, st, branchID); after != countBefore {
		t.Fatalf("重发触发了额外生成: %d → %d", countBefore, after)
	}
	_ = rootID
}

// T13：两个客户端基于同一头同时写入时，只有一个能提交，
// 另一个拿到可恢复的冲突（客户端刷新后重试即可）。
func TestConcurrentWriteOnSameHeadConflicts(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, affectionScript(2))

	// 设备 A 先提交，分支头推进。
	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "c-1", "我先来。")

	// 设备 B 还拿着旧头（root + version 0）提交 → 可恢复的冲突。
	_, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "c-2", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "我也来。"},
	})
	if err == nil {
		t.Fatalf("基于过期头的写入应被拒绝")
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "HEAD_CONFLICT" {
		t.Fatalf("错误码 = %v, want HEAD_CONFLICT", err)
	}
	if !apiErr.Retryable {
		t.Fatalf("HEAD_CONFLICT 应标记为可恢复（客户端刷新后重试）")
	}

	// 分支头未被第二个请求改动。
	br, err := st.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	if br.HeadNodeID != first.ResultNodeID {
		t.Fatalf("分支头被并发请求改动: %s", br.HeadNodeID)
	}

	// 设备 B 刷新（取到新头）后可以正常继续。
	retried := acceptAndWait(t, turnSvc, st, sessionID, branchID, "c-3", "我也来。")
	if retried.Status != domain.TurnCommitted {
		t.Fatalf("刷新后重试应成功: %s", retried.Status)
	}
}

// 同一分支上已有进行中的回合时，新请求被队列保护拒绝（不并发跑两次生成）。
func TestInFlightTurnBlocksSameBranch(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, affectionScript(2))

	// 手工占住分支锁，模拟另一个回合正在生成。
	occupied := "turn_occupied"
	if ok, err := st.ClaimActiveTurn(branchID, occupied); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}

	_, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "q-1", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "排队。"},
	})
	if err == nil {
		t.Fatalf("分支被占用时应拒绝新回合")
	}
	if apiErr, ok := err.(*APIError); !ok || apiErr.Code != "QUEUE_FULL" {
		t.Fatalf("错误码 = %v, want QUEUE_FULL", err)
	}
}

// countNodes 返回分支头所在路径的节点数，用于断言"没有发生第二次生成"。
func countNodes(t *testing.T, st *sqlite.Store, branchID string) int {
	t.Helper()
	br, err := st.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	chain, err := st.AncestorChain(br.HeadNodeID, true)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	return len(chain)
}
