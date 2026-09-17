package application

import (
	"context"
	"testing"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// TestTurnUsagePersisted 锁定"用量被采集并落库"这条链路（T0.1）。
//
// 这是后续所有优化的前提：没有真实读数，压缩阈值、预算裁剪与前缀缓存
// 优化都无法被验证。用例同时校验真实值与估算值都落库——只存其一无法
// 发现编译侧 token 估算的漂移。
//
// 有齿验证：注释掉 afterStream 里的 s.recordUsage(...) 调用，
// 本用例会在 "rows = 0" 处失败。
func TestTurnUsagePersisted(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, happyScript())

	tr, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "usage-1", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "我记得那个约定。"}, Mode: "structured",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	got := waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted)
	if got.ResultNodeID == "" {
		t.Fatal("未提交")
	}

	rows, err := st.RecentTurnUsage(10)
	if err != nil {
		t.Fatalf("recent usage: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("turn_usage 无记录：用量未被采集")
	}

	var rec ports.TurnUsageRecord
	found := false
	for _, r := range rows {
		if r.TurnID == tr.TurnID {
			rec = r
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("turn_usage 缺少本回合记录：%+v", rows)
	}
	if !rec.Reported {
		t.Fatalf("Reported = false，供应商未回传用量（mock 应确定性合成）: %+v", rec)
	}
	if rec.Completion <= 0 {
		t.Fatalf("completion_tokens = %d, want > 0（mock 按交付字节合成）: %+v", rec.Completion, rec)
	}
	// 估算值是校准的基准，必须一并落库，否则无法发现估算漂移。
	if rec.Estimated <= 0 {
		t.Fatalf("estimated_tokens = %d, want > 0（编译侧估算应随请求上行）: %+v", rec.Estimated, rec)
	}

	totals, err := st.TurnUsageTotals()
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if totals.Calls != len(rows) {
		t.Fatalf("totals.Calls = %d, want %d", totals.Calls, len(rows))
	}
	if totals.ReportedCalls == 0 {
		t.Fatalf("totals.ReportedCalls = 0, want > 0: %+v", totals)
	}
}

// TestUsageRecordingNeverBreaksTurnCommit 确认用量台账是纯观测行为：
// 即使台账不可用，回合仍然正常提交（观测数据不得影响业务语义）。
func TestUsageRecordingNeverBreaksTurnCommit(t *testing.T) {
	_, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, happyScript())
	// 把台账端口置空，模拟"用量存储不可用"。
	turnSvc.usage = nil

	tr, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "usage-off", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "继续。"}, Mode: "structured",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	got := waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted)
	if got.ResultNodeID == "" {
		t.Fatal("台账不可用时回合应照常提交")
	}
}
