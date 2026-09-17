package application

import (
	"context"
	"encoding/json"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// detRand 是确定性随机源：按脚本依次返回。
type detRand struct {
	values []int
	i      int
}

func (d *detRand) Intn(n int) int {
	if d.i >= len(d.values) {
		return 0
	}
	v := d.values[d.i] % n
	d.i++
	return v
}

// 准备阶段掷骰并落一条 prepared 收据。
func TestPrepareCheckRollsOnceAndPersists(t *testing.T) {
	st, svc, _, sessID, branchID, root := newTestServices(t, nil)
	svc.SetRandom(&detRand{values: []int{13}}) // d20 = 14

	rc, err := svc.PrepareCheck(context.Background(), sessID, branchID, root,
		PrepareCheckRequest{ActionID: "check.strength", TurnID: "turn_a"})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if rc.Status != domain.ReceiptPrepared {
		t.Fatalf("收据状态 = %s, want prepared", rc.Status)
	}
	if rc.RollID == "" || rc.TurnID != "turn_a" {
		t.Fatalf("收据未绑定到回合: %+v", rc)
	}
	cr, err := rc.CheckResultOf()
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if cr.Natural != 14 || cr.AttributeMod != 0 || cr.Total != 14 || cr.Outcome != OutcomeTiers(14, 12) {
		t.Fatalf("检定结果异常: %+v", cr)
	}
	_ = st
}

// T26：同一个行动实例（同父节点 + 同动作 + 同规则版本）再次请求时
// 必须复用结果、不再掷骰——文字重生成不改变检定结果。
func TestPrepareCheckReusesRollForSameActionInstance(t *testing.T) {
	_, svc, _, sessID, branchID, root := newTestServices(t, nil)
	rng := &detRand{values: []int{13}}
	svc.SetRandom(rng)

	first, err := svc.PrepareCheck(context.Background(), sessID, branchID, root,
		PrepareCheckRequest{ActionID: "check.strength", TurnID: "turn_a"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	used := rng.i

	// 同一行动实例、不同回合（等价于文字重生成建了新回合）。
	second, err := svc.PrepareCheck(context.Background(), sessID, branchID, root,
		PrepareCheckRequest{ActionID: "check.strength", TurnID: "turn_b"})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if rng.i != used {
		t.Fatalf("复用路径又掷了一次骰（T26 被破坏）")
	}
	if first.RollID != second.RollID {
		t.Fatalf("rollId 不一致: %q vs %q", first.RollID, second.RollID)
	}
	a, _ := first.CheckResultOf()
	b, _ := second.CheckResultOf()
	if a.Natural != b.Natural || a.Outcome != b.Outcome {
		t.Fatalf("复用时结果改变: %+v vs %+v", a, b)
	}
	if first.ReceiptID == second.ReceiptID {
		t.Fatalf("应为新回合建独立收据行（契约要求不能复用旧请求）")
	}
}

// 基准父节点推进 = 玩家发起了新的检定动作 → 必须掷出新的结果。
func TestPrepareCheckRollsAgainOnNewBaseHead(t *testing.T) {
	st, svc, _, sessID, branchID, root := newTestServices(t, nil)
	svc.SetRandom(&detRand{values: []int{13, 4}})

	first, err := svc.PrepareCheck(context.Background(), sessID, branchID, root,
		PrepareCheckRequest{ActionID: "check.strength", TurnID: "turn_a"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	// 玩家行动使父节点推进（这里直接提交一个节点模拟）。
	newHead := commitHead(t, st, sessID, branchID, root)
	second, err := svc.PrepareCheck(context.Background(), sessID, branchID, newHead,
		PrepareCheckRequest{ActionID: "check.strength", TurnID: "turn_b"})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.RollID == second.RollID {
		t.Fatalf("不同基准应得到不同 rollId")
	}
	a, _ := first.CheckResultOf()
	b, _ := second.CheckResultOf()
	if a.Natural == b.Natural {
		t.Fatalf("不同基准应掷出不同结果（脚本给的是不同骰值）")
	}
}

// 授权面：未登记的动作一律拒绝——这是堵住 actionRef 空集漏洞的另一半。
func TestPrepareCheckRejectsUnauthorizedAction(t *testing.T) {
	_, svc, _, sessID, branchID, root := newTestServices(t, nil)
	if _, err := svc.PrepareCheck(context.Background(), sessID, branchID, root,
		PrepareCheckRequest{ActionID: "model.invented"}); err == nil {
		t.Fatalf("未登记的动作应被拒绝")
	}
}

// 前置条件不满足时不执行；规则求值失败必须显式报错而不是静默放行。
func TestPrepareCheckHonoursPreconditions(t *testing.T) {
	_, svc, _, sessID, branchID, root := newTestServices(t, nil)
	rs := domain.DefaultRuleset()
	rs.Actions["check.gated"] = domain.ActionRule{
		ActionID: "check.gated", Attribute: "strength", DC: 12,
		Requires: &domain.RuleExpr{Op: domain.OpHasItem, Arg: "item_absent"},
	}
	svc.SetRuleset(rs)

	if _, err := svc.PrepareCheck(context.Background(), sessID, branchID, root,
		PrepareCheckRequest{ActionID: "check.gated"}); err == nil {
		t.Fatalf("前置条件不满足应拒绝")
	}

	broken := domain.DefaultRuleset()
	broken.Actions["check.bad"] = domain.ActionRule{
		ActionID: "check.bad", Attribute: "strength", DC: 12,
		Requires: &domain.RuleExpr{Op: domain.RuleOp("exec")},
	}
	svc.SetRuleset(broken)
	if _, err := svc.PrepareCheck(context.Background(), sessID, branchID, root,
		PrepareCheckRequest{ActionID: "check.bad"}); err == nil {
		t.Fatalf("非法规则应显式报错")
	}
}

// 属性来自规则包与状态投影；模型给不了、也改不了。
func TestPrepareCheckUsesStateAttribute(t *testing.T) {
	st, svc, _, sessID, branchID, root := newTestServices(t, nil)

	// 给玩家一个 18 力量（修正 +4）。
	snap, err := st.StateAt(root)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	state, err := domain.UnmarshalWorld(snap.StateJSON)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	p := state.Characters[PlayerCharacterID]
	p.Attributes = map[string]int{"strength": 18}
	state.Characters[PlayerCharacterID] = p
	next, err := state.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := st.SaveSnapshot(&domain.StateSnapshot{
		NodeID: root, SnapshotVersion: snap.SnapshotVersion + 1,
		RulesetVersion: "ruleset.simplified.v1", StateJSON: next, StateHash: state.HashID(),
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	svc.SetRandom(&detRand{values: []int{9}}) // d20 = 10
	rc, err := svc.PrepareCheck(context.Background(), sessID, branchID, root, PrepareCheckRequest{ActionID: "check.strength"})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	cr, _ := rc.CheckResultOf()
	if cr.AttributeVal != 18 || cr.AttributeMod != 4 || cr.Total != 14 {
		t.Fatalf("属性未生效: %+v", cr)
	}
}

// 提交结算：收据 prepared → committed，且幂等。
func TestSettleReceiptsIsIdempotent(t *testing.T) {
	st, svc, _, sessID, branchID, root := newTestServices(t, nil)
	svc.SetRandom(&detRand{values: []int{13}})

	rc, err := svc.PrepareCheck(context.Background(), sessID, branchID, root,
		PrepareCheckRequest{ActionID: "check.strength", TurnID: "turn_x"})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if rc.TurnID != "turn_x" {
		t.Fatalf("收据未绑定到回合")
	}
	if err := st.SettleReceipts([]string{rc.ReceiptID}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	// 幂等：重复结算不报错。
	if err := st.SettleReceipts([]string{rc.ReceiptID}); err != nil {
		t.Fatalf("settle twice: %v", err)
	}
	got, err := st.GetReceipts("turn_x")
	if err != nil || len(got) != 1 {
		t.Fatalf("receipts: %d err=%v", len(got), err)
	}
	if got[0].Status != domain.ReceiptCommitted {
		t.Fatalf("状态 = %s, want committed", got[0].Status)
	}
	var cr domain.CheckResult
	if err := json.Unmarshal([]byte(got[0].ResultJSON), &cr); err != nil || cr.Natural != 14 {
		t.Fatalf("结算不应改动结果: %+v err=%v", cr, err)
	}
}

func OutcomeTiers(natural, dc int) domain.CheckOutcome {
	mod := domain.AttributeMod(domain.DefaultAttribute)
	total := natural + mod
	switch {
	case natural == 20:
		return domain.OutcomeCriticalSuccess
	case natural == 1:
		return domain.OutcomeCriticalFailure
	case total >= dc+10:
		return domain.OutcomeCriticalSuccess
	case total >= dc:
		return domain.OutcomeSuccess
	case total <= dc-10:
		return domain.OutcomeCriticalFailure
	default:
		return domain.OutcomeFailure
	}
}

var _ = ports.RealRandom{}

// marshalStateForTest 序列化世界状态（sqlite 包的 MarshalState 是测试内部助手）。
func marshalStateForTest(state *domain.WorldState) string {
	b, err := state.Marshal()
	if err != nil {
		return "{}"
	}
	return b
}

// commitHead 在给定父节点上提交一个空回合节点，返回新头。
// 检定的"新基准"必须是真实存在的节点，否则状态投影无从读取。
func commitHead(t *testing.T, st *sqlite.Store, sessionID, branchID, parentID string) string {
	t.Helper()
	parent, err := st.GetNode(parentID)
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	branch, err := st.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	bs, err := st.StateAt(parentID)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	state, err := domain.UnmarshalWorld(bs.StateJSON)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	turnID := "turn_head_" + parentID
	if err := st.CreateTurnRequest(&domain.TurnRequest{
		TurnID: turnID, SessionID: sessionID, BranchID: branchID,
		IdempotencyKey: turnID, PayloadHash: "h", ExpectedHeadID: parentID,
		Status: domain.TurnQueued, Mode: "structured",
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if ok, err := st.ClaimActiveTurn(branchID, turnID); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	nodeID := "node_head_" + parentID
	res, err := st.CommitTurn(&ports.CommitPlan{
		TurnID: turnID, ExpectedHeadID: parentID, ExpectedVersion: branch.Version,
		BaseStateHash: state.HashID(), RulesetVersion: "ruleset.simplified.v1",
		Node: &domain.PlotNode{
			NodeID: nodeID, SessionID: sessionID, ParentID: parentID, Kind: domain.NodeKindTurn,
			Depth: parent.Depth + 1, TurnNumber: parent.TurnNumber + 1, SchemaVersion: 1,
			ContentJSON: `{"inputText":"推进。","blocks":[{"kind":"narration","text":"推进。"}]}`,
		},
		NewStateHash: state.HashID(), NewStateJSON: marshalStateForTest(state),
	})
	if err != nil || !res.Committed {
		t.Fatalf("commit: %v %+v", err, res)
	}
	return nodeID
}
