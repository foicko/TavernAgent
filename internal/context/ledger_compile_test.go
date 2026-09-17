package context_test

import (
	"strconv"
	"strings"
	"testing"

	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// ---- M4h 集成：真实 SQLite 链路上，已结算收据随 Compile 进入账本（T29）----

// commitTurnWithReceipt 提交一个带已结算检定收据的回合，返回新节点 ID。
// 收据挂到回合上（turn_requests.result_node_id 关联），并随提交事务置 committed。
func commitTurnWithReceipt(t *testing.T, f *fixture, branchID, headID string, state *domain.WorldState, receipt *domain.ActionReceipt) string {
	t.Helper()
	parent, err := f.store.GetNode(headID)
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	nextJSON, err := state.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	commitSeq.Add(1)
	seq := strconv.FormatInt(commitSeq.Load(), 10)
	turnID := "turn_led_" + seq
	nodeID := "node_led_" + seq
	branch, err := f.store.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	if err := f.store.CreateTurnRequest(&domain.TurnRequest{
		TurnID: turnID, SessionID: testSessionID, BranchID: branchID,
		IdempotencyKey: turnID, PayloadHash: "h", ExpectedHeadID: headID,
		Status: domain.TurnQueued, Mode: "structured",
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if ok, err := f.store.ClaimActiveTurn(branchID, turnID); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	// 收据必须挂到本回合上：ReceiptsAtResultNodes 经 turn_id 反查。
	receipt.TurnID = turnID
	if err := f.store.SaveReceipt(receipt); err != nil {
		t.Fatalf("save receipt: %v", err)
	}
	res, err := f.store.CommitTurn(&ports.CommitPlan{
		TurnID: turnID, ExpectedHeadID: headID, ExpectedVersion: branch.Version,
		Node: &domain.PlotNode{
			NodeID: nodeID, SessionID: testSessionID, ParentID: headID, Kind: domain.NodeKindTurn,
			Depth: parent.Depth + 1, TurnNumber: parent.TurnNumber + 1, SchemaVersion: 1,
			ContentJSON: `{"inputText":"推进。","blocks":[{"kind":"narration","text":"推进。"}]}`,
		},
		NewStateHash: state.HashID(), NewStateJSON: nextJSON,
		SettleReceipts: []string{receipt.ReceiptID},
	})
	if err != nil || !res.Committed {
		t.Fatalf("commit: %v %+v", err, res)
	}
	return nodeID
}

// T29：摘要可以说"丢了什么"，但硬下界账本以状态投影与已结算收据为准——
// 四子账本全部出现在编译产物的 system 提示中，摘要无法覆盖。
func TestLedgerInjectedThroughFullCompile(t *testing.T) {
	f := newFixtureFull(t, nil, "", "", ctxpkg.DefaultOptions())

	// 两个带检定的历史回合：DC15 失败（入选账本）与 DC8 失败（不入账本）。
	base := domain.NewWorldState()
	head := commitTurnWithReceipt(t, f, testBranchID, testRootID, base, &domain.ActionReceipt{
		ReceiptID: "lr1", ActionID: "stealth", RollID: "lr1",
		BaseHeadID: testRootID, RulesetVersion: "v1",
		ResultJSON: `{"rollId":"lr1","actionId":"stealth","attribute":"敏捷","attributeValue":12,"attributeModifier":1,"natural":8,"total":9,"dc":15,"outcome":"failure","rulesetVersion":"v1"}`,
		Status:     domain.ReceiptPrepared,
	})
	head = commitTurnWithReceipt(t, f, testBranchID, head, base, &domain.ActionReceipt{
		ReceiptID: "lr2", ActionID: "athletics", RollID: "lr2",
		BaseHeadID: head, RulesetVersion: "v1",
		ResultJSON: `{"rollId":"lr2","actionId":"athletics","attribute":"力量","attributeValue":10,"attributeModifier":0,"natural":5,"total":5,"dc":8,"outcome":"failure","rulesetVersion":"v1"}`,
		Status:     domain.ReceiptPrepared,
	})

	// 当前状态投影：信物 + 生效誓言 + 里程碑。
	state := domain.NewWorldState()
	state.Characters["elena"] = domain.CharacterInfo{CharacterID: "elena", Name: "艾莲娜", Participant: true}
	state.Items["watch"] = domain.ItemInstance{InstanceID: "watch", Name: "银质怀表", OwnerID: "player", Quantity: 1, Keepsake: true}
	state.Promises["p1"] = domain.Promise{
		PromiseID: "p1", ParticipantIDs: []string{"player", "elena"},
		Content: "日落前在老钟楼汇合", State: domain.PromiseActive,
	}
	state.Milestones["bell_tower_riot"] = "钟楼暴乱已爆发"

	sys := f.systemPromptWithState(t, head, "我摸向口袋。", state)

	if !strings.Contains(sys, `<domain_state_ledger authoritative="true">`) {
		t.Fatalf("账本未注入 system 提示:\n%s", sys)
	}
	if !strings.Contains(sys, "银质怀表") || !strings.Contains(sys, "无法丢弃") {
		t.Fatalf("信物未带「无法丢弃」标注:\n%s", sys)
	}
	if !strings.Contains(sys, "艾莲娜对测试者立誓") || !strings.Contains(sys, "生效中") {
		t.Fatalf("誓言账本缺失:\n%s", sys)
	}
	// 历史大检定：DC15 失败入选（带 Turn 标注），DC8 失败排除。
	if !strings.Contains(sys, "历史大检定裁决") || !strings.Contains(sys, "敏捷检定") || !strings.Contains(sys, "Turn#1") {
		t.Fatalf("大检定账本缺失:\n%s", sys)
	}
	ledger := strings.SplitN(strings.SplitN(sys, `<domain_state_ledger authoritative="true">`, 2)[1], "</domain_state_ledger>", 2)[0]
	if strings.Contains(ledger, "力量检定") {
		t.Fatalf("DC8 普通失败不应入账本:\n%s", sys)
	}
	if !strings.Contains(sys, "钟楼暴乱已爆发") {
		t.Fatalf("里程碑账本缺失:\n%s", sys)
	}
	if strings.Contains(sys, "当前持有物品") {
		t.Fatalf("旧平铺物品段应删除（账本统一承载）:\n%s", sys)
	}
}
