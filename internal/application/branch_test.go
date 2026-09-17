package application

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/domain"
)

// ---- E1/E2/E3：分叉、候选版本与编辑 ----

// affectionScript 返回一轮脚本：一段正文 + 一条对 npc_elena 的好感增量。
func affectionScript(delta int) []mock.Item {
	return []mock.Item{
		mock.Frame(mockBlock(1, "narration", "她的神情缓和了一些。")),
		mock.Frame(mockFinal(2,
			`{"proposalId":"p1","type":"relationship_delta","characterId":"npc_elena","field":"affection","delta":`+
				strconv.Itoa(delta)+`}`, "")),
	}
}

// acceptAndWait 以分支当前头为基准受理一轮并等待提交。
func acceptAndWait(t *testing.T, svc *TurnService, st *sqlite.Store, sessionID, branchID, key, text string) *domain.TurnRequest {
	t.Helper()
	br, err := st.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	tr, err := svc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: key, ExpectedHeadID: br.HeadNodeID, ExpectedVersion: br.Version,
		Input: domain.TurnInput{Kind: "text", Text: text},
	})
	if err != nil {
		t.Fatalf("accept %s: %v", key, err)
	}
	return waitTurn(t, svc, tr.TurnID, domain.TurnCommitted)
}

// affectionOf 读取某节点状态投影里的好感值。
func affectionOf(t *testing.T, st *sqlite.Store, nodeID string) int {
	t.Helper()
	sn, err := st.StateAt(nodeID)
	if err != nil {
		t.Fatalf("state at %s: %v", nodeID, err)
	}
	ws, err := domain.UnmarshalWorld(sn.StateJSON)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return ws.Relationships["npc_elena"].Affection
}

// fork(X) 建立 head=X 的新分支，不生成任何内容，也不改动既有分支。
func TestForkCreatesBranchAtNode(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, affectionScript(2))
	svc := NewBranchService(st, turnSvc)

	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "f-1", "你好。")

	b, err := svc.Fork(context.Background(), sessionID, ForkRequest{FromNodeID: first.ResultNodeID})
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if b.HeadNodeID != first.ResultNodeID {
		t.Fatalf("新分支头 = %s, want %s", b.HeadNodeID, first.ResultNodeID)
	}
	if b.Version != 0 {
		t.Fatalf("新分支版本应为 0，实际 %d", b.Version)
	}
	if b.Name == "" {
		t.Fatalf("分支应有默认展示名")
	}
	// 原分支指针不受影响。
	orig, err := st.GetBranch(branchID)
	if err != nil {
		t.Fatalf("get branch: %v", err)
	}
	if orig.HeadNodeID != first.ResultNodeID || orig.Version != 1 {
		t.Fatalf("原分支被改动: %+v", orig)
	}
	// fork 不产生节点。
	kids, err := st.ListChildren(sessionID, first.ResultNodeID)
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	if len(kids) != 0 {
		t.Fatalf("fork 不应生成节点: %+v", kids)
	}
	_ = rootID
}

// 跨会话的节点不能用作分叉起点。
func TestForkRejectsForeignNode(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(1))
	svc := NewBranchService(st, turnSvc)
	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "f-2", "你好。")

	if _, err := svc.Fork(context.Background(), "sess_other", ForkRequest{FromNodeID: first.ResultNodeID}); err == nil {
		t.Fatalf("跨会话分叉应被拒绝")
	}
	if _, err := svc.Fork(context.Background(), sessionID, ForkRequest{FromNodeID: "node_missing"}); err == nil {
		t.Fatalf("不存在的节点应被拒绝")
	}
}

// regenerate(N)：新回复与 N 是兄弟节点，N 本身完全不变。
func TestRegenerateCreatesSiblingCandidate(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, affectionScript(2))
	svc := NewBranchService(st, turnSvc)

	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "r-1", "你好。")
	second := acceptAndWait(t, turnSvc, st, sessionID, branchID, "r-2", "继续。")

	before, err := st.GetNode(second.ResultNodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}

	res, err := svc.DeriveTurn(context.Background(), sessionID, DeriveRequest{NodeID: second.ResultNodeID})
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if res.Branch.HeadNodeID != first.ResultNodeID {
		t.Fatalf("候选分支头应为 parent(N)=%s，实际 %s", first.ResultNodeID, res.Branch.HeadNodeID)
	}
	committed := waitTurn(t, turnSvc, res.Turn.TurnID, domain.TurnCommitted)

	// 兄弟关系：新节点与被重生成节点的父节点相同。
	got, err := st.GetNode(committed.ResultNodeID)
	if err != nil {
		t.Fatalf("get new node: %v", err)
	}
	targetNode, err := st.GetNode(second.ResultNodeID)
	if err != nil {
		t.Fatalf("get target node: %v", err)
	}
	if got.ParentID != targetNode.ParentID || got.ParentID != first.ResultNodeID {
		t.Fatalf("新节点父节点 = %s, want %s", got.ParentID, first.ResultNodeID)
	}
	if got.NodeID == second.ResultNodeID {
		t.Fatalf("候选节点不应复用原节点 ID")
	}

	// 原节点不可改写（D02）：内容与创建时间都不变。
	after, err := st.GetNode(second.ResultNodeID)
	if err != nil {
		t.Fatalf("re-get node: %v", err)
	}
	if after.ContentJSON != before.ContentJSON {
		t.Fatalf("原节点内容被改写")
	}

	// 原分支指针不受影响（候选走新分支）。
	orig, _ := st.GetBranch(branchID)
	if orig.HeadNodeID != second.ResultNodeID {
		t.Fatalf("原分支头被改动: %s", orig.HeadNodeID)
	}
	_ = rootID
}

// 关键：基准状态取 parent(N)，不是 N 应用后的状态——否则数值会被扣第二次。
func TestRegenerateUsesParentStateNotTargetState(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(2))
	svc := NewBranchService(st, turnSvc)

	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "p-1", "你好。")
	second := acceptAndWait(t, turnSvc, st, sessionID, branchID, "p-2", "继续。")

	if got := affectionOf(t, st, first.ResultNodeID); got != 2 {
		t.Fatalf("第 1 轮好感 = %d, want 2", got)
	}
	if got := affectionOf(t, st, second.ResultNodeID); got != 4 {
		t.Fatalf("第 2 轮好感 = %d, want 4", got)
	}

	res, err := svc.DeriveTurn(context.Background(), sessionID, DeriveRequest{NodeID: second.ResultNodeID})
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	committed := waitTurn(t, turnSvc, res.Turn.TurnID, domain.TurnCommitted)

	// parent(N) 是 2，本轮再 +2 → 4。若错误地以 N(4) 为基准，会得到 6。
	if got := affectionOf(t, st, committed.ResultNodeID); got != 4 {
		t.Fatalf("候选版本好感 = %d, want 4（基准应取 parent(N) 而非 N）", got)
	}
}

// 根节点没有可重生成的回合。
func TestRegenerateRejectsRootAndNonTurn(t *testing.T) {
	st, turnSvc, _, sessionID, _, rootID := newTestServices(t, affectionScript(1))
	svc := NewBranchService(st, turnSvc)

	if _, err := svc.DeriveTurn(context.Background(), sessionID, DeriveRequest{NodeID: rootID}); err == nil {
		t.Fatalf("根节点重生成应被拒绝")
	}
	if _, err := svc.DeriveTurn(context.Background(), sessionID, DeriveRequest{NodeID: "node_missing"}); err == nil {
		t.Fatalf("不存在的节点应被拒绝")
	}
}

// 编辑玩家输入：从 parent(N) 出分支并重新生成，原后续不复用。
func TestEditPlayerInputDerivesNewBranch(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(2))
	svc := NewBranchService(st, turnSvc)

	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "e-1", "你好。")
	second := acceptAndWait(t, turnSvc, st, sessionID, branchID, "e-2", "继续。")

	res, err := svc.DeriveTurn(context.Background(), sessionID, DeriveRequest{
		NodeID: second.ResultNodeID,
		Input:  &domain.TurnInput{Kind: "text", Text: "我转身离开。"},
	})
	if err != nil {
		t.Fatalf("edit input: %v", err)
	}
	committed := waitTurn(t, turnSvc, res.Turn.TurnID, domain.TurnCommitted)

	// 新节点挂在 parent(N) 下，携带的是编辑后的输入。
	got, err := st.GetNode(committed.ResultNodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if got.ParentID != first.ResultNodeID {
		t.Fatalf("编辑应从 parent(N) 出分支: %s", got.ParentID)
	}
	turn, err := st.GetTurn(res.Turn.TurnID)
	if err != nil {
		t.Fatalf("get turn: %v", err)
	}
	if in := inputOf(turn); in.Text != "我转身离开。" {
		t.Fatalf("输入未替换: %+v", in)
	}
	// 旧节点保持不变。
	after, _ := st.GetNode(second.ResultNodeID)
	if after.ParentID != first.ResultNodeID || after.ContentJSON == "" {
		t.Fatalf("原节点被改动")
	}
}

// 编辑空输入被拒绝（不静默生成一个空回合）。
func TestEditPlayerInputRejectsEmpty(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(1))
	svc := NewBranchService(st, turnSvc)
	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "e-3", "你好。")

	if _, err := svc.DeriveTurn(context.Background(), sessionID, DeriveRequest{
		NodeID: first.ResultNodeID, Input: &domain.TurnInput{Kind: "text", Text: "   "},
	}); err == nil {
		t.Fatalf("空输入应被拒绝")
	}
}

// 编辑助手正文：降级为纯叙事候选——零状态变化，原提议被丢弃。
func TestEditAssistantTextIsNarrativeCandidate(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(2))
	svc := NewBranchService(st, turnSvc)

	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "a-1", "你好。")
	second := acceptAndWait(t, turnSvc, st, sessionID, branchID, "a-2", "继续。")
	beforeAffection := affectionOf(t, st, second.ResultNodeID)

	res, err := svc.EditAssistantText(context.Background(), sessionID, AssistantEditRequest{
		NodeID: second.ResultNodeID,
		Blocks: []domain.TextBlock{{Kind: "narration", Text: "她沉默了很久，最终什么也没说。"}},
	})
	if err != nil {
		t.Fatalf("edit assistant: %v", err)
	}
	if res.Branch.HeadNodeID != first.ResultNodeID {
		t.Fatalf("候选分支头 = %s, want %s", res.Branch.HeadNodeID, first.ResultNodeID)
	}
	if res.Node.ParentID != first.ResultNodeID {
		t.Fatalf("候选节点应挂在 parent(N) 下: %s", res.Node.ParentID)
	}

	// 零状态变化：新候选节点的好感与 parent 相同，而不是叠加原提议。
	parentAff := affectionOf(t, st, first.ResultNodeID)
	if got := affectionOf(t, st, res.Node.NodeID); got != parentAff {
		t.Fatalf("改写正文应按纯叙事候选提交（零状态变化），好感 = %d want %d", got, parentAff)
	}
	if got := affectionOf(t, st, res.Node.NodeID); got == beforeAffection+2 {
		t.Fatalf("原提议被错误保留")
	}

	// 节点内容标记为 narrative，且无提议、无选项。
	var tc domain.TurnContent
	if err := json.Unmarshal([]byte(res.Node.ContentJSON), &tc); err != nil {
		t.Fatalf("unmarshal turn content: %v", err)
	}
	if tc.Provenance == nil || tc.Provenance.Mode != "narrative" {
		t.Fatalf("候选节点 mode 应为 narrative: %+v", tc.Provenance)
	}
	if len(tc.Options) != 0 {
		t.Fatalf("纯叙事候选不应携带选项")
	}
	if len(tc.Blocks) != 1 || tc.Blocks[0].Text != "她沉默了很久，最终什么也没说。" {
		t.Fatalf("正文未按改写内容保存: %+v", tc.Blocks)
	}
	// 事件里不应出现 relationship_delta。
	evs, err := st.GetEvents(res.Node.NodeID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	for _, ev := range evs {
		if ev.Type == domain.EventRelationshipDelta {
			t.Fatalf("纯叙事候选不应产生状态事件")
		}
	}

	// 原节点与其它路径不受影响。
	if got := affectionOf(t, st, second.ResultNodeID); got != beforeAffection {
		t.Fatalf("原节点状态被改动: %d", got)
	}
}

// 编辑正文需要非空正文，且只接受回合节点。
func TestEditAssistantTextValidation(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, affectionScript(1))
	svc := NewBranchService(st, turnSvc)
	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "a-2", "你好。")

	if _, err := svc.EditAssistantText(context.Background(), sessionID, AssistantEditRequest{
		NodeID: first.ResultNodeID, Blocks: []domain.TextBlock{{Kind: "narration", Text: "  "}},
	}); err == nil {
		t.Fatalf("空正文应被拒绝")
	}
	if _, err := svc.EditAssistantText(context.Background(), sessionID, AssistantEditRequest{
		NodeID: rootID, Blocks: []domain.TextBlock{{Kind: "narration", Text: "x"}},
	}); err == nil {
		t.Fatalf("非回合节点应被拒绝")
	}
}

// 候选版本与主动分叉都只新增分支：既有分支数量只增不减，且节点树是单父的。
func TestDeriveDoesNotCreateMultiParentNode(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(2))
	svc := NewBranchService(st, turnSvc)

	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "m-1", "你好。")
	second := acceptAndWait(t, turnSvc, st, sessionID, branchID, "m-2", "继续。")

	res, err := svc.DeriveTurn(context.Background(), sessionID, DeriveRequest{NodeID: second.ResultNodeID})
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	committed := waitTurn(t, turnSvc, res.Turn.TurnID, domain.TurnCommitted)

	kids, err := st.ListChildren(sessionID, first.ResultNodeID)
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	if len(kids) != 2 {
		t.Fatalf("parent 下应有 2 个兄弟（原回合 + 候选），实际 %d", len(kids))
	}
	seen := map[string]bool{}
	for _, k := range kids {
		if seen[k.NodeID] {
			t.Fatalf("节点重复: %s", k.NodeID)
		}
		seen[k.NodeID] = true
		if k.ParentID != first.ResultNodeID {
			t.Fatalf("子节点父引用错误: %+v", k)
		}
	}
	if !seen[second.ResultNodeID] || !seen[committed.ResultNodeID] {
		t.Fatalf("兄弟集合不完整: %+v", seen)
	}
}
