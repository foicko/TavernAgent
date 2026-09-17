package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/util/id"
)

// 回归：状态投影读不了时，必须暴露真实原因。
//
// 曾经的写法是 `if sn, serr := StateAt(...); serr == nil && sn != nil`，把 serr 静默丢掉，
// 于是 state 保持 nil，紧接着 UnmarshalWorld("") 抛出 "unexpected end of JSON input"——
// 真正的原因（指纹不符）被完全掩盖，线上表现就是一个无从下手的 500。
func TestViewSurfacesRealReasonWhenStateUnreadable(t *testing.T) {
	st, _, sessSvc, sessionID, _, rootID := newTestServices(t, affectionScript(1))

	sess, err := st.GetSession(sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	snap, err := st.StateAt(rootID)
	if err != nil {
		t.Fatalf("state at root: %v", err)
	}
	// 把指纹换成一个对不上的值，模拟快照与状态不同步。
	stale := sha256.Sum256([]byte("fingerprint that no longer matches the snapshot"))
	if err := st.SaveSnapshot(&domain.StateSnapshot{
		NodeID: rootID, SnapshotVersion: 1, RulesetVersion: sess.RulesetVersion,
		StateJSON: snap.StateJSON, StateHash: hex.EncodeToString(stale[:]),
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	_, err = sessSvc.View(sessionID, ViewQuery{})
	if err == nil {
		t.Fatal("指纹不符却没有报错")
	}
	if !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("错误应带着真实原因，实际: %v", err)
	}
	if strings.Contains(err.Error(), "unexpected end of JSON input") {
		t.Fatalf("不应再出现被掩盖成 JSON 解析失败的提示: %v", err)
	}
}

// ---- I3：会话视图窗口化与向上翻页 ----

// 视图只返回窗口内的回合：万级节点的会话不能把整条链一次交给前端。
func TestViewWindowLimitsTurns(t *testing.T) {
	st, turnSvc, sessSvc, sessionID, branchID, _ := newTestServices(t, affectionScript(1))

	const rounds = 6
	for i := 0; i < rounds; i++ {
		acceptAndWait(t, turnSvc, st, sessionID, branchID, "vw-"+string(rune('a'+i)), "继续。")
	}

	v, err := sessSvc.View(sessionID, ViewQuery{Limit: 3})
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	// 根节点始终携带 + 窗口内 3 个回合。
	if len(v.Nodes) != 4 {
		t.Fatalf("节点数 = %d, want 4（根 + 3 回合）", len(v.Nodes))
	}
	if !v.HasMore {
		t.Fatalf("更早的回合存在时应标记 hasMore")
	}
	if v.OldestTurnID == "" {
		t.Fatalf("应给出翻页游标")
	}
	// 窗口内最后一个回合就是分支头。
	last := v.Nodes[len(v.Nodes)-1]
	if last.NodeID != v.ViewNodeID {
		var desc []string
		for _, n := range v.Nodes {
			desc = append(desc, string(n.Kind)+"@"+itoaOf(n.Depth))
		}
		t.Fatalf("窗口末节点 = %s (kind=%s depth=%d), want viewNode %s\n  窗口内容: %v\n  hasMore=%v oldest=%s",
			last.NodeID, last.Kind, last.Depth, v.ViewNodeID, desc, v.HasMore, v.OldestTurnID)
	}
}

// 用 before 游标能翻到更早的回合，且翻到底后 hasMore 变 false。
func TestViewPagingWithBeforeCursor(t *testing.T) {
	st, turnSvc, sessSvc, sessionID, branchID, _ := newTestServices(t, affectionScript(1))

	const rounds = 6
	seen := map[string]bool{}
	for i := 0; i < rounds; i++ {
		tr := acceptAndWait(t, turnSvc, st, sessionID, branchID, "pg-"+string(rune('a'+i)), "继续。")
		seen[tr.ResultNodeID] = true
	}

	first, err := sessSvc.View(sessionID, ViewQuery{Limit: 3})
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	second, err := sessSvc.View(sessionID, ViewQuery{Limit: 3, Before: first.OldestTurnID})
	if err != nil {
		t.Fatalf("view page 2: %v", err)
	}

	// 两页的回合不重叠，且第二页更早。
	firstIDs := map[string]bool{}
	for _, n := range first.Nodes {
		firstIDs[n.NodeID] = true
	}
	overlap := 0
	for _, n := range second.Nodes {
		if firstIDs[n.NodeID] {
			overlap++
		}
	}
	if overlap > 1 {
		t.Fatalf("两页之间重复了 %d 个节点（只应共享根节点）: %v", overlap, second.Nodes)
	}
	for _, n := range second.Nodes {
		if n.Kind == "turn" && firstIDs[n.NodeID] {
			t.Fatalf("翻页不应重复返回同一回合: %s", n.NodeID)
		}
	}

	// 再翻一页：回合已取完，hasMore 应为 false。
	third, err := sessSvc.View(sessionID, ViewQuery{Limit: 3, Before: second.OldestTurnID})
	if err != nil {
		t.Fatalf("view page 3: %v", err)
	}
	if third.HasMore {
		t.Fatalf("翻到底后 hasMore 应为 false，实际 true（OldestTurnID=%s）", third.OldestTurnID)
	}
}

// limit 有硬上限，避免"窗口化"被一个大参数绕过去。
func TestViewLimitIsCapped(t *testing.T) {
	st, turnSvc, sessSvc, sessionID, branchID, _ := newTestServices(t, affectionScript(1))
	acceptAndWait(t, turnSvc, st, sessionID, branchID, "cap-1", "继续。")

	v, err := sessSvc.View(sessionID, ViewQuery{Limit: MaxViewLimit * 10})
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if v.HasMore {
		t.Fatalf("小会话不应标记 hasMore")
	}
	// 不校验具体节点数（数据量小），只确认请求没有因为超大 limit 失败。
	if len(v.Nodes) == 0 {
		t.Fatalf("应返回节点")
	}
}

func itoaOf(n int) string {
	if n == 0 {
		return "0"
	}
	buf := []byte{}
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// M4e：图谱只取目标节点周围的子图，且包含候选版本。
// 万级会话整树下发本身就违背"可交互 p95 ≤ 500ms"的前提。
func TestGraphReturnsBoundedSubgraphWithCandidates(t *testing.T) {
	st, _, sessSvc, sessionID, branchID, root := newTestServices(t, affectionScript(1))

	const rounds = 3
	head := root
	for i := 0; i < rounds; i++ {
		head = commitBareTurn(t, st, sessionID, branchID, head)
	}
	// root 下再造一条兄弟分支（候选）。
	if err := st.CreateBranch(&domain.Branch{
		BranchID: "branch_cand", SessionID: sessionID, Name: "cand", HeadNodeID: root, Version: 0,
	}); err != nil {
		t.Fatalf("create candidate branch: %v", err)
	}
	sibling := commitBareTurn(t, st, sessionID, "branch_cand", root)

	g, err := sessSvc.Graph(sessionID, head, 3, 1)
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	byID := map[string]GraphNode{}
	for _, n := range g.Nodes {
		byID[n.NodeID] = n
	}
	if _, ok := byID[head]; !ok {
		t.Fatalf("子图缺目标节点")
	}
	if _, ok := byID[root]; !ok {
		t.Fatalf("子图缺向上层（root）")
	}
	sib, ok := byID[sibling]
	if !ok {
		t.Fatalf("子图缺候选兄弟节点: %+v", g.Nodes)
	}
	if !sib.IsCandidate {
		t.Fatalf("兄弟节点未被标记为候选: %+v", sib)
	}
	// up=3 + 目标 + 1 个候选兄弟 = 5 个节点；不该有更多（down=1 无子节点）。
	if len(g.Nodes) != 5 {
		t.Fatalf("子图节点数 = %d, want 5", len(g.Nodes))
	}
}

// 跨会话的节点必须拒绝，防止用图谱接口探测其它会话的树。
func TestGraphRejectsForeignSession(t *testing.T) {
	_, _, sessSvc, sessionID, _, root := newTestServices(t, affectionScript(1))

	if _, err := sessSvc.Graph("session_other", root, 2, 2); err == nil {
		t.Fatalf("跨会话查询应被拒绝")
	}
	if _, err := sessSvc.Graph(sessionID, root, 2, 2); err != nil {
		t.Fatalf("本会话查询失败: %v", err)
	}
}

// commitBareTurn 直接经提交事务写入一个无状态变化的回合节点（图谱测试用）。
func commitBareTurn(t *testing.T, st *sqlite.Store, sessionID, branchID, parentID string) string {
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
	turnID := id.New()
	nodeID := id.New()
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
	content, _ := json.Marshal(domain.TurnContent{
		InputText: "推进。", Blocks: []domain.TextBlock{{Kind: "narration", Text: "推进。"}},
	})
	res, err := st.CommitTurn(&ports.CommitPlan{
		TurnID: turnID, ExpectedHeadID: parentID, ExpectedVersion: branch.Version,
		BaseStateHash: state.HashID(), RulesetVersion: RulesetVersion,
		Node: &domain.PlotNode{
			NodeID: nodeID, SessionID: sessionID, ParentID: parentID, Kind: domain.NodeKindTurn,
			Depth: parent.Depth + 1, TurnNumber: parent.TurnNumber + 1, SchemaVersion: 1,
			ContentJSON: string(content),
		},
		NewStateHash: state.HashID(), NewStateJSON: sqlite.MarshalState(state),
	})
	if err != nil || !res.Committed {
		t.Fatalf("commit: %v %+v", err, res)
	}
	return nodeID
}
