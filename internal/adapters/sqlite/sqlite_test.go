package sqlite

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/util/id"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seedSession(t *testing.T, s *Store) (*domain.Session, *domain.PlotNode, *domain.Branch) {
	t.Helper()
	sess := &domain.Session{SessionID: id.New(), RootNodeID: "root", Title: "测试会话", CreatedAt: time.Now()}
	root := &domain.PlotNode{
		NodeID: "root", SessionID: sess.SessionID, Kind: domain.NodeKindRoot,
		ContentJSON: `{"initial":true}`, SchemaVersion: 1,
	}
	branch := &domain.Branch{BranchID: "branch_main", SessionID: sess.SessionID, Name: "main", HeadNodeID: "root", Version: 0}
	if err := s.CreateSession(sess, root, branch, nil); err != nil {
		t.Fatalf("create session: %v", err)
	}
	sess.RootNodeID = "root"
	return sess, root, branch
}

// 迁移与 FTS5 探测。
func TestOpenMigratesAndProbe(t *testing.T) {
	s := newTestStore(t)
	info, err := s.SystemInfo()
	if err != nil {
		t.Fatalf("system info: %v", err)
	}
	if info.DriverVersion == "" {
		t.Fatalf("driver version missing")
	}
	// 记录探测结果到日志（M0 验证报告依赖此能力）
	t.Logf("sqlite driver=%s fts5=%v err=%q", info.DriverVersion, info.FTS5Available, info.AttributeError)
}

// 会话创建与读取。
func TestCreateAndGetSession(t *testing.T) {
	s := newTestStore(t)
	sess, root, branch := seedSession(t, s)
	got, err := s.GetSession(sess.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.RootNodeID != root.NodeID {
		t.Fatalf("root = %q, want %q", got.RootNodeID, root.NodeID)
	}
	gotBranch, err := s.GetBranch(branch.BranchID)
	if err != nil {
		t.Fatalf("get branch: %v", err)
	}
	if gotBranch.HeadNodeID != "root" || gotBranch.Version != 0 {
		t.Fatalf("branch = %+v", gotBranch)
	}
}

func makeCommitPlan(sessionID, turnID, branchID, newHead, expectedHead string, expectedVersion int64, baseHash string, evs []*domain.DomainEvent, state *domain.WorldState) *ports.CommitPlan {
	plan := &ports.CommitPlan{
		TurnID:          turnID,
		ExpectedHeadID:  expectedHead,
		ExpectedVersion: expectedVersion,
		BaseStateHash:   baseHash,
		Node: &domain.PlotNode{
			NodeID: newHead, SessionID: sessionID, ParentID: expectedHead, Kind: domain.NodeKindTurn,
			TurnNumber: 1, SchemaVersion: 1, ContentJSON: `{"input":"hi","blocks":[]}`,
		},
		NewStateHash: HashState(state),
		NewStateJSON: MarshalState(state),
		Events:       evs,
	}
	for _, ev := range evs {
		if ev.NodeID == "" {
			ev.NodeID = plan.Node.NodeID
		}
	}
	return plan
}

// seedTurn 在分支上准备一个生成中的回合（等价于应用层受理状态）。
func seedTurn(t *testing.T, s *Store, sessionID, branchID, turnID, head string, version int64) {
	t.Helper()
	req := &domain.TurnRequest{
		TurnID: turnID, SessionID: sessionID, BranchID: branchID, IdempotencyKey: "k-" + turnID,
		PayloadHash: "h", ExpectedHeadID: head, ExpectedVersion: version,
		Status: domain.TurnGenerating, Mode: "structured",
	}
	if err := s.CreateTurnRequest(req); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if ok, err := s.ClaimActiveTurn(branchID, turnID); err != nil || !ok {
		t.Fatalf("claim active: ok=%v err=%v", ok, err)
	}
	if err := s.CreateAttempt(&domain.TurnAttempt{AttemptID: "a-" + turnID, TurnID: turnID, AttemptNo: 1, BaseHeadID: head, BaseVersion: version}); err != nil {
		t.Fatalf("attempt: %v", err)
	}
}

// T11a 提交原子性：写入后某一步失败（HEAD_CONFLICT）整笔回滚，无半提交。
func TestCommitAtomicRollback(t *testing.T) {
	s := newTestStore(t)
	sess, _, branch := seedSession(t, s)
	seedTurn(t, s, sess.SessionID, branch.BranchID, "turn1", "root", 0)

	evs := []*domain.DomainEvent{{
		EventID: "ev1", EventIndex: 0, Type: domain.EventRelationshipDelta,
		PayloadJSON: `{"characterId":"npc","field":"trust","applied":5}`,
	}}
	// 预期版本不符 → CAS 失败 → 整笔回滚
	plan := makeCommitPlan(sess.SessionID, "turn1", branch.BranchID, "node1", "root", 99, "", evs, domain.NewWorldState())
	res, err := s.CommitTurn(plan)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if res.ConflictCode != "HEAD_CONFLICT" {
		t.Fatalf("conflict = %q, want HEAD_CONFLICT", res.ConflictCode)
	}
	if _, err := s.GetNode("node1"); err == nil {
		t.Fatalf("node must not exist after rollback")
	}
	if evts, err := s.GetEvents("node1"); err != nil || len(evts) != 0 {
		t.Fatalf("events must be rolled back: %v %d", err, len(evts))
	}
	// 分支未被推进
	gotBranch, _ := s.GetBranch(branch.BranchID)
	if gotBranch.HeadNodeID != "root" {
		t.Fatalf("branch head changed: %+v", gotBranch)
	}
}

// T01 幂等提交：同一 turnId 只产生一个节点、一次状态效果。
func TestCommitOnceIdempotent(t *testing.T) {
	s := newTestStore(t)
	sess, _, branch := seedSession(t, s)
	seedTurn(t, s, sess.SessionID, branch.BranchID, "turn1", "root", 0)

	base := domain.NewWorldState()
	base.Characters["npc"] = domain.CharacterInfo{CharacterID: "npc", Name: "NPC"}
	state := base.Clone()
	state.Relationships["npc"] = domain.RelationValue{Trust: 5}
	evs := []*domain.DomainEvent{{
		EventID: "ev1", EventIndex: 0, Type: domain.EventRelationshipDelta,
		PayloadJSON: `{"characterId":"npc","field":"trust","applied":5}`,
	}}
	plan := makeCommitPlan(sess.SessionID, "turn1", branch.BranchID, "node1", "root", 0, HashState(base), evs, state)

	res, err := s.CommitTurn(plan)
	if err != nil || !res.Committed {
		t.Fatalf("commit: %v res=%+v", err, res)
	}
	// 再次提交同一 turnId → AlreadyDone，不重复效果
	res2, err := s.CommitTurn(plan)
	if err != nil {
		t.Fatalf("commit2: %v", err)
	}
	if !res2.AlreadyDone {
		t.Fatalf("expected AlreadyDone, got %+v", res2)
	}
	// 只存在一个结果节点；分支头正确
	got, err := s.GetNode("node1")
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if got.ParentID != "root" {
		t.Fatalf("parent = %s", got.ParentID)
	}
	evts, _ := s.GetEvents("node1")
	if len(evts) != 1 {
		t.Fatalf("events = %d, want 1", len(evts))
	}
	branch2, _ := s.GetBranch(branch.BranchID)
	if branch2.HeadNodeID != "node1" || branch2.Version != 1 {
		t.Fatalf("branch = %+v", branch2)
	}
	// 快照可读且哈希一致
	sn, err := s.GetSnapshot("node1")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if sn.StateHash != HashState(state) {
		t.Fatalf("snapshot hash mismatch")
	}
	// outbox 完成事件存在
	evs2, err := s.PollOutbox("turn1", 0, 10)
	if err != nil || len(evs2) == 0 {
		t.Fatalf("outbox: %v %v", err, evs2)
	}
	if evs2[len(evs2)-1].Type != "turn.committed" {
		t.Fatalf("outbox last type = %s", evs2[len(evs2)-1].Type)
	}
}

// T11b 崩溃近似模拟：独立事务写入后不提交直接丢弃连接，重启后无半状态。
func TestCrashLeavesNoPartialState(t *testing.T) {
	dir := t.TempDir()
	s1, err := Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sess := &domain.Session{SessionID: "s1", RootNodeID: "root"}
	if err := s1.CreateSession(sess, &domain.PlotNode{NodeID: "root", SessionID: "s1", Kind: domain.NodeKindRoot}, &domain.Branch{BranchID: "b1", SessionID: "s1", HeadNodeID: "root"}, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// 模拟进程死亡：开启事务、写入部分节点、不提交、丢弃。
	tx, err := s1.db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO plot_nodes(node_id, session_id, parent_id, kind, depth, turn_number, schema_version, content_json, created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		"ghost", "s1", "root", "turn", 1, 1, 1, `{}`, time.Now().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert: %v", err)
	}
	_ = tx.Rollback() // 未 commit → SQLite 回滚（等价于崩溃后重启）
	// 先关闭首个连接（Windows 文件锁），再以新进程视角重开。
	if err := s1.Close(); err != nil {
		t.Fatalf("close s1: %v", err)
	}

	s2, err := Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	if _, err := s2.GetNode("ghost"); err == nil {
		t.Fatalf("ghost node must not survive")
	}
}

// 祖先链查询：A 的馈赠/修订不出现在 B 的链上（C04 的存储侧验证）。
func TestAncestorChainIsolation(t *testing.T) {
	s := newTestStore(t)
	sess, _, _ := seedSession(t, s)
	// 链 root -> n1 -> n2 -> n3
	parent := "root"
	for i := 1; i <= 3; i++ {
		n := &domain.PlotNode{NodeID: fmt.Sprintf("n%d", i), SessionID: sess.SessionID, ParentID: parent,
			Kind: domain.NodeKindTurn, Depth: i, TurnNumber: 1, SchemaVersion: 1, ContentJSON: `{}`}
		if err := s.InsertNode(n); err != nil {
			t.Fatalf("insert: %v", err)
		}
		parent = n.NodeID
	}
	chain, err := s.AncestorChain("n3", true)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if len(chain) != 4 { // root..n3
		t.Fatalf("chain len = %d, want 4", len(chain))
	}
}

// ClaimActiveTurn：同一分支只允许一个活动回合。
func TestClaimActiveTurnExclusive(t *testing.T) {
	s := newTestStore(t)
	_, _, branch := seedSession(t, s)
	ok, err := s.ClaimActiveTurn(branch.BranchID, "turn1")
	if err != nil || !ok {
		t.Fatalf("claim1: %v %v", ok, err)
	}
	ok, err = s.ClaimActiveTurn(branch.BranchID, "turn2")
	if err != nil {
		t.Fatalf("claim2: %v", err)
	}
	if ok {
		t.Fatalf("second claim must fail while active turn exists")
	}
	// 同 turn 再次 claim 允许
	ok, err = s.ClaimActiveTurn(branch.BranchID, "turn1")
	if err != nil || !ok {
		t.Fatalf("reclaim same: %v %v", ok, err)
	}
}

// JSON 状态往返。
func TestWorldStateJSONRoundTrip(t *testing.T) {
	ws := domain.NewWorldState()
	ws.Relationships["npc"] = domain.RelationValue{Affection: 10, Trust: 5}
	ws.Items["i1"] = domain.ItemInstance{InstanceID: "i1", Name: "怀表", OwnerID: "player", Quantity: 1}
	data := MarshalState(ws)
	got, err := UnmarshalState(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Relationships["npc"].Trust != 5 || got.Items["i1"].Quantity != 1 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

// ---- 记忆记录（M3）----

// commitMemories 通过真实提交事务写入记忆，覆盖「记忆与节点同事务」这条路径。
func commitMemories(t *testing.T, s *Store, sess *domain.Session, parentID string, mems ...*domain.MemoryRecord) string {
	t.Helper()
	turnID := id.New()
	if err := s.CreateTurnRequest(&domain.TurnRequest{
		TurnID: turnID, SessionID: sess.SessionID, BranchID: "branch_main",
		IdempotencyKey: id.New(), PayloadHash: "h", ExpectedHeadID: parentID,
		Status: domain.TurnAccepted, Mode: "structured",
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if _, err := s.ClaimActiveTurn("branch_main", turnID); err != nil {
		t.Fatalf("claim: %v", err)
	}
	parent, err := s.GetNode(parentID)
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	nodeID := id.New()
	for _, m := range mems {
		m.SourceNodeID = nodeID
	}
	sn := &domain.StateSnapshot{NodeID: parentID, SnapshotVersion: 1, StateJSON: `{}`, StateHash: domain.NewWorldState().HashID()}
	_ = s.SaveSnapshot(sn)
	res, err := s.CommitTurn(&ports.CommitPlan{
		TurnID: turnID, ExpectedHeadID: parentID, ExpectedVersion: 0,
		BaseStateHash: domain.NewWorldState().HashID(), RulesetVersion: "test",
		Node: &domain.PlotNode{
			NodeID: nodeID, SessionID: sess.SessionID, ParentID: parentID,
			Kind: domain.NodeKindTurn, Depth: parent.Depth + 1, TurnNumber: parent.TurnNumber + 1,
			SchemaVersion: 1, ContentJSON: `{}`,
		},
		NewStateHash: domain.NewWorldState().HashID(), NewStateJSON: `{}`,
		Memories: mems,
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !res.Committed {
		t.Fatalf("commit not applied: %+v", res)
	}
	return nodeID
}

func mem(id, content string) *domain.MemoryRecord {
	return &domain.MemoryRecord{MemoryID: id, Kind: domain.MemoryObserved, Content: content}
}

// 记忆随节点同事务落库，可按会话列出。
func TestMemoryCommitAndList(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	nodeID := commitMemories(t, s, sess, root.NodeID,
		mem("m1", "她来自北方。"), mem("m2", "她有个妹妹。"))

	got, err := s.ListMemories(sess.SessionID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("memories = %d, want 2", len(got))
	}
	for _, m := range got {
		if m.SourceNodeID != nodeID {
			t.Fatalf("sourceNodeId = %q, want %q", m.SourceNodeID, nodeID)
		}
		if len(m.OwnerIDs) != 0 || len(m.EntityIDs) != 0 {
			t.Fatalf("空切片应稳定序列化为 []: %+v", m)
		}
	}
}

// T16/T17：记忆按来源节点隔离——分叉出去的分支看不到另一条路径上的记忆。
func TestMemoriesInChainIsolatesBranches(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)

	// 共同祖先之后分叉：A 分支与 B 分支。
	aNode := commitMemories(t, s, sess, root.NodeID, mem("mA", "A 的秘密记忆。"))

	// B 分支：新建分支指向共同祖先 root，再提交一个节点。
	if err := s.CreateBranch(&domain.Branch{
		BranchID: "branch_b", SessionID: sess.SessionID, Name: "b", HeadNodeID: root.NodeID, Version: 0,
	}); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	bTurn := id.New()
	if err := s.CreateTurnRequest(&domain.TurnRequest{
		TurnID: bTurn, SessionID: sess.SessionID, BranchID: "branch_b",
		IdempotencyKey: id.New(), PayloadHash: "h", ExpectedHeadID: root.NodeID,
		Status: domain.TurnAccepted, Mode: "structured",
	}); err != nil {
		t.Fatalf("create turn b: %v", err)
	}
	if _, err := s.ClaimActiveTurn("branch_b", bTurn); err != nil {
		t.Fatalf("claim b: %v", err)
	}
	_ = s.SaveSnapshot(&domain.StateSnapshot{NodeID: root.NodeID, SnapshotVersion: 1, StateJSON: `{}`, StateHash: domain.NewWorldState().HashID()})
	bNode := id.New()
	// 记忆必须指向真实节点：SourceNodeID 缺失会被外键约束拦截（悬空引用不允许）。
	bMem := mem("mB", "B 的记忆。")
	bMem.SourceNodeID = bNode
	res, err := s.CommitTurn(&ports.CommitPlan{
		TurnID: bTurn, ExpectedHeadID: root.NodeID, ExpectedVersion: 0,
		BaseStateHash: domain.NewWorldState().HashID(), RulesetVersion: "test",
		Node: &domain.PlotNode{
			NodeID: bNode, SessionID: sess.SessionID, ParentID: root.NodeID,
			Kind: domain.NodeKindTurn, Depth: 1, TurnNumber: 1, SchemaVersion: 1, ContentJSON: `{}`,
		},
		NewStateHash: domain.NewWorldState().HashID(), NewStateJSON: `{}`,
		Memories: []*domain.MemoryRecord{bMem},
	})
	if err != nil || !res.Committed {
		t.Fatalf("commit b: %v %+v", err, res)
	}

	chainOf := func(nodeID string) []string {
		nodes, err := s.AncestorChain(nodeID, true)
		if err != nil {
			t.Fatalf("chain %s: %v", nodeID, err)
		}
		ids := make([]string, 0, len(nodes))
		for _, n := range nodes {
			ids = append(ids, n.NodeID)
		}
		return ids
	}

	// A 路径：只看得到 A 的记忆。
	aMems, err := s.MemoriesInChain(chainOf(aNode))
	if err != nil {
		t.Fatalf("a chain memories: %v", err)
	}
	if len(aMems) != 1 || aMems[0].MemoryID != "mA" {
		t.Fatalf("A 路径记忆 = %+v", aMems)
	}

	// B 路径：共同祖先之后的事件不会串线（T16/T17）。
	bMems, err := s.MemoriesInChain(chainOf(bNode))
	if err != nil {
		t.Fatalf("b chain memories: %v", err)
	}
	if len(bMems) != 1 || bMems[0].MemoryID != "mB" {
		t.Fatalf("B 路径不应看到 A 的记忆: %+v", bMems)
	}

	// 会话级列表仍能看到全部（管理视图需要）。
	all, err := s.ListMemories(sess.SessionID)
	if err != nil || len(all) != 2 {
		t.Fatalf("list = %d err=%v", len(all), err)
	}
}

// 空链不返回任何记忆（避免退化成"返回全部"）。
func TestMemoriesInChainEmptyInput(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	commitMemories(t, s, sess, root.NodeID, mem("m1", "x"))
	got, err := s.MemoriesInChain(nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("空链应返回空: %d err=%v", len(got), err)
	}
}

// 用户干预：纠正内容 / 置顶 / 隐藏 / 标记被修订。
func TestUpdateMemory(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	commitMemories(t, s, sess, root.NodeID, mem("m1", "她来自北方。"))

	got, _ := s.ListMemories(sess.SessionID)
	m := got[0]

	m2 := *m
	m2.Content = "她来自南方。"
	m2.Pinned = true
	if err := s.UpdateMemory(&m2); err != nil {
		t.Fatalf("update: %v", err)
	}
	after, _ := s.ListMemories(sess.SessionID)
	if after[0].Content != "她来自南方。" || !after[0].Pinned {
		t.Fatalf("更新未生效: %+v", after[0])
	}

	m3 := after[0]
	m3.Hidden = true
	m3.Supersedes = "m0"
	if err := s.UpdateMemory(m3); err != nil {
		t.Fatalf("hide: %v", err)
	}
	final, _ := s.ListMemories(sess.SessionID)
	if !final[0].Hidden || final[0].Supersedes != "m0" {
		t.Fatalf("隐藏/覆盖链接未生效: %+v", final[0])
	}
	// 记录仍在库里（隐藏是可见性，不是删除）。
	if len(final) != 1 {
		t.Fatalf("隐藏不应删除记录: %d", len(final))
	}

	// 不存在的记忆 → 明确报错，而不是静默成功。
	missing := mem("nope", "x")
	if err := s.UpdateMemory(missing); err == nil {
		t.Fatalf("更新不存在的记忆应报错")
	}
}

// ---- 检查点与状态重放（T15 / E5）----

// commitChainTurn 从 parentID 提交一个回合节点，附带一条 relationship_delta 事件。
// 基准状态必须经 StateAt 取得（稀疏检查点下父节点多半没有直接快照）。
func commitChainTurn(t *testing.T, s *Store, sess *domain.Session, parentID string, delta int) (string, string) {
	t.Helper()
	parent, err := s.GetNode(parentID)
	if err != nil {
		t.Fatalf("parent %s: %v", parentID, err)
	}
	bs, err := s.StateAt(parentID)
	if err != nil {
		t.Fatalf("state at %s: %v", parentID, err)
	}
	state, err := domain.UnmarshalWorld(bs.StateJSON)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	nodeID := id.New()
	payload, _ := json.Marshal(domain.RelationshipDeltaPayload{
		CharacterID: "npc_x", Field: "affection", Delta: delta, Applied: delta,
	})
	ev := &domain.DomainEvent{
		EventID: id.New(), NodeID: nodeID, EventIndex: 0,
		Type: domain.EventRelationshipDelta, PayloadJSON: string(payload), RulesetVersion: "test",
	}
	next := state.Clone()
	if _, err := domain.ApplyEvent(next, ev); err != nil {
		t.Fatalf("apply: %v", err)
	}
	nextJSON, _ := next.Marshal()

	turnID := id.New()
	if err := s.CreateTurnRequest(&domain.TurnRequest{
		TurnID: turnID, SessionID: sess.SessionID, BranchID: "branch_main",
		IdempotencyKey: turnID, PayloadHash: "h", ExpectedHeadID: parentID,
		Status: domain.TurnQueued, Mode: "structured",
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if ok, err := s.ClaimActiveTurn("branch_main", turnID); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	branch, err := s.GetBranch("branch_main")
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	res, err := s.CommitTurn(&ports.CommitPlan{
		TurnID: turnID, ExpectedHeadID: parentID, ExpectedVersion: branch.Version,
		BaseStateHash: bs.StateHash, RulesetVersion: "test",
		Node: &domain.PlotNode{
			NodeID: nodeID, SessionID: sess.SessionID, ParentID: parentID,
			Kind: domain.NodeKindTurn, Depth: parent.Depth + 1, TurnNumber: parent.TurnNumber + 1,
			SchemaVersion: 1, ContentJSON: `{}`,
		},
		Events:       []*domain.DomainEvent{ev},
		NewStateHash: next.HashID(), NewStateJSON: nextJSON,
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !res.Committed {
		t.Fatalf("commit rejected: %+v", res)
	}
	return nodeID, next.HashID()
}

// seedRootState 写入根节点状态投影（生产由 Setup 负责，测试需手动补）。
func seedRootState(t *testing.T, s *Store, rootID string) {
	t.Helper()
	st := domain.NewWorldState()
	st.Relationships["npc_x"] = domain.RelationValue{}
	js, _ := st.Marshal()
	if err := s.SaveSnapshot(&domain.StateSnapshot{
		NodeID: rootID, SnapshotVersion: 1, StateJSON: js, StateHash: st.HashID(),
	}); err != nil {
		t.Fatalf("save root snapshot: %v", err)
	}
}

// snapshotDepths 返回已落盘快照对应的节点深度集合。
func snapshotDepths(t *testing.T, s *Store) []int {
	t.Helper()
	rows, err := s.db.Query(`SELECT n.depth FROM state_snapshots s JOIN plot_nodes n ON n.node_id = s.node_id ORDER BY n.depth`)
	if err != nil {
		t.Fatalf("query snapshots: %v", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var d int
		if err := rows.Scan(&d); err != nil {
			t.Fatalf("scan depth: %v", err)
		}
		out = append(out, d)
	}
	return out
}

// 检查点是稀疏的：长链只落间隔节点，中间节点靠重放还原。
func TestCheckpointIsSparse(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)

	head := root.NodeID
	for i := 0; i < 25; i++ {
		head, _ = commitChainTurn(t, s, sess, head, 1)
	}

	depths := snapshotDepths(t, s)
	want := []int{0, 1, 20}
	if len(depths) != len(want) {
		t.Fatalf("快照深度集合 = %v, want %v（检查点未生效）", depths, want)
	}
	for i := range want {
		if depths[i] != want[i] {
			t.Fatalf("快照深度集合 = %v, want %v", depths, want)
		}
	}

	// 中间节点确实没有直接快照，但 StateAt 必须能还原。
	if _, err := s.GetSnapshot(head); err == nil {
		t.Fatalf("深度 25 的节点不应有直接快照")
	}
	sn, err := s.StateAt(head)
	if err != nil {
		t.Fatalf("StateAt(深度25): %v", err)
	}
	if sn.StateJSON == "" {
		t.Fatalf("重放结果为空")
	}
}

// T15：不同检查点与全量事件重放同一节点，状态哈希必须一致。
func TestStateAtMatchesFullReplay(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)

	head := root.NodeID
	var lastHash string
	for i := 0; i < 25; i++ {
		head, lastHash = commitChainTurn(t, s, sess, head, 1)
	}

	// 稀疏检查点下：从最近的 depth 20 检查点重放 5 步。
	fromCheckpoint, err := s.StateAt(head)
	if err != nil {
		t.Fatalf("StateAt: %v", err)
	}
	if fromCheckpoint.StateHash != lastHash {
		t.Fatalf("重放哈希 ≠ 提交时哈希\n got=%s\nwant=%s", fromCheckpoint.StateHash, lastHash)
	}

	// 删掉中间检查点：退化为从根全量重放 25 步，结果必须完全相同。
	if _, err := s.db.Exec(`DELETE FROM state_snapshots WHERE node_id IN (SELECT node_id FROM plot_nodes WHERE depth=20)`); err != nil {
		t.Fatalf("delete checkpoint: %v", err)
	}
	fromRoot, err := s.StateAt(head)
	if err != nil {
		t.Fatalf("StateAt(全量重放): %v", err)
	}
	if fromRoot.StateHash != fromCheckpoint.StateHash {
		t.Fatalf("不同检查点得到不同状态哈希（T15 失败）\n 检查点起点=%s\n 根起点  =%s",
			fromCheckpoint.StateHash, fromRoot.StateHash)
	}
}

// 重放结果不落库：StateAt 是只读计算。
func TestStateAtIsReadOnly(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)

	head := root.NodeID
	for i := 0; i < 22; i++ {
		head, _ = commitChainTurn(t, s, sess, head, 1)
	}
	before := snapshotDepths(t, s)
	for i := 0; i < 3; i++ {
		if _, err := s.StateAt(head); err != nil {
			t.Fatalf("StateAt: %v", err)
		}
	}
	after := snapshotDepths(t, s)
	if len(before) != len(after) {
		t.Fatalf("StateAt 不应写库: before=%v after=%v", before, after)
	}
}

// 第二十层落检查点后，它自身就是重放起点（不重复重放）。
func TestCheckpointNodeServesAsBase(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)

	head := root.NodeID
	var hashAt20 string
	for i := 1; i <= 20; i++ {
		var h string
		head, h = commitChainTurn(t, s, sess, head, 1)
		if i == 20 {
			hashAt20 = h
		}
	}
	sn, err := s.StateAt(head)
	if err != nil {
		t.Fatalf("StateAt: %v", err)
	}
	if sn.StateHash != hashAt20 {
		t.Fatalf("检查点节点状态不符: got=%s want=%s", sn.StateHash, hashAt20)
	}
	if sn.NodeID != head {
		t.Fatalf("StateAt 应返回目标节点: %s", sn.NodeID)
	}
}

// RecentTurnNodes 必须按深度升序返回。
//
// 这条用例是补上的：早期实现的递归项把"当前节点的 depth"写进了父节点的行，
// 排序键整体错位，链一长返回顺序就乱（实测出现过 4,6,5 这种次序）。
// 顺序错乱会让正文历史颠倒，而这不会让任何计数类断言失败。
func TestRecentTurnNodesOrdering(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)

	head := root.NodeID
	// State checkpoints accelerate replay; they must never truncate narrative.
	// Cross two checkpoints and query both an exact checkpoint and its children.
	for i := 1; i <= 42; i++ {
		head, _ = commitChainTurn(t, s, sess, head, 1)
		if i%20 == 0 {
			atCheckpoint, err := s.RecentTurnNodes(head, 20)
			if err != nil || len(atCheckpoint) != 20 {
				t.Fatalf("checkpoint at turn %d truncated history: got=%d err=%v", i, len(atCheckpoint), err)
			}
		}
	}

	got, err := s.RecentTurnNodes(head, 5)
	if err != nil {
		t.Fatalf("recent turns: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("返回 %d 个，want 5", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Depth <= got[i-1].Depth {
			var depths []int
			for _, n := range got {
				depths = append(depths, n.Depth)
			}
			t.Fatalf("返回顺序不是深度升序: %v", depths)
		}
	}
	if got[len(got)-1].NodeID != head {
		t.Fatalf("最后一个应是最新节点（窗口末端对齐当前头）")
	}

	// 限深路径与兜底路径都要正确：用超过窗口长度的请求逼出回退分支。
	all, err := s.RecentTurnNodes(head, 100)
	if err != nil {
		t.Fatalf("recent turns (all): %v", err)
	}
	if len(all) != 42 {
		t.Fatalf("全部回合 = %d, want 42", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].Depth <= all[i-1].Depth {
			t.Fatalf("回退路径顺序不是深度升序（第 %d 个）", i)
		}
	}
}

// M4h 历史大检定账本：提交后 committed 收据可按结果节点反查（带回合序号），
// prepared（未结算）收据不算权威历史事实，不返回。
func TestReceiptsAtResultNodes(t *testing.T) {
	s := newTestStore(t)
	sess, _, branch := seedSession(t, s)
	seedTurn(t, s, sess.SessionID, branch.BranchID, "turn1", "root", 0)

	// 两张收据：一张将随提交结算，一张保持 prepared。
	if err := s.SaveReceipt(&domain.ActionReceipt{
		ReceiptID: "r1", TurnID: "turn1", ActionID: "stealth", RollID: "roll1",
		BaseHeadID: "root", RulesetVersion: "v1",
		ResultJSON: `{"rollId":"roll1","actionId":"stealth","attribute":"敏捷","attributeValue":12,"attributeModifier":1,"natural":8,"total":9,"dc":15,"outcome":"failure","rulesetVersion":"v1"}`,
		Status:     domain.ReceiptPrepared,
	}); err != nil {
		t.Fatalf("save receipt: %v", err)
	}
	if err := s.SaveReceipt(&domain.ActionReceipt{
		ReceiptID: "r2", TurnID: "turn1", ActionID: "insight", RollID: "roll2",
		BaseHeadID: "root", RulesetVersion: "v1",
		ResultJSON: `{"rollId":"roll2","actionId":"insight","attribute":"感知","attributeValue":14,"attributeModifier":2,"natural":20,"total":22,"dc":18,"outcome":"critical_success","rulesetVersion":"v1"}`,
		Status:     domain.ReceiptPrepared,
	}); err != nil {
		t.Fatalf("save receipt2: %v", err)
	}

	base := domain.NewWorldState()
	state := base.Clone()
	plan := makeCommitPlan(sess.SessionID, "turn1", branch.BranchID, "node1", "root", 0, HashState(base), nil, state)
	plan.SettleReceipts = []string{"r1"}
	if res, err := s.CommitTurn(plan); err != nil || !res.Committed {
		t.Fatalf("commit: %v res=%+v", err, res)
	}

	got, err := s.ReceiptsAtResultNodes([]string{"node1"})
	if err != nil {
		t.Fatalf("receipts at nodes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("committed 收据应恰好 1 条（prepared 不算），得到 %d", len(got))
	}
	if got[0].NodeID != "node1" || got[0].TurnNumber != 1 || got[0].Receipt.ReceiptID != "r1" {
		t.Fatalf("收据视图不正确: %+v", got[0])
	}
	cr, cerr := got[0].Receipt.CheckResultOf()
	if cerr != nil || cr.ActionID != "stealth" {
		t.Fatalf("收据结果解析失败: %v %+v", cerr, cr)
	}

	// 不相干节点不返回；空入参直接返回空。
	if got2, err := s.ReceiptsAtResultNodes([]string{"root"}); err != nil || len(got2) != 0 {
		t.Fatalf("root 无收据: %v %d", err, len(got2))
	}
	if got3, err := s.ReceiptsAtResultNodes(nil); err != nil || got3 != nil {
		t.Fatalf("空入参应返回空: %v", err)
	}
}

// 验证 PERF-01：读写连接池分离，reader 开启 query_only 且支持多协程并发查询。
func TestReaderPoolQueryOnlyAndConcurrentReads(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)

	if s.reader == nil {
		t.Fatalf("reader connection pool must be initialized")
	}

	// 1. 验证 reader 具备 query_only 约束：任何写操作必须被 SQLite 内核直接拦截
	_, err := s.reader.Exec(`INSERT INTO sessions(session_id, root_node_id, title, created_at) VALUES('bad', 'root', 'test', '')`)
	if err == nil {
		t.Fatalf("reader 必须开启 query_only(1)，写操作应返回错误，但实际成功执行")
	}

	// 2. 验证多协程并发读取不会被阻塞且数据一致
	var wg sync.WaitGroup
	const concurrency = 20
	errChan := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := s.GetSession(sess.SessionID)
			if err != nil {
				errChan <- err
				return
			}
			if got.SessionID != sess.SessionID {
				errChan <- fmt.Errorf("sessionID mismatch: got %s, want %s", got.SessionID, sess.SessionID)
				return
			}
			_, err = s.GetNode(root.NodeID)
			if err != nil {
				errChan <- err
				return
			}
		}()
	}
	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			t.Fatalf("concurrent read failed: %v", err)
		}
	}
}

func TestEventsOnPath(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)

	// Turn 1: relationship_delta
	n1, _ := commitChainTurn(t, s, sess, root.NodeID, 5)
	// Turn 2: secret_unlock
	parent, err := s.GetNode(n1)
	if err != nil {
		t.Fatalf("get n1: %v", err)
	}
	bs, err := s.StateAt(n1)
	if err != nil {
		t.Fatalf("state at n1: %v", err)
	}
	st2, _ := domain.UnmarshalWorld(bs.StateJSON)
	st2.UnlockSecret("sec_locket")
	st2JSON, _ := st2.Marshal()

	secTurnID := id.New()
	secNodeID := id.New()
	if err := s.CreateTurnRequest(&domain.TurnRequest{
		TurnID: secTurnID, SessionID: sess.SessionID, BranchID: "branch_main",
		IdempotencyKey: secTurnID, PayloadHash: "h", ExpectedHeadID: n1,
		Status: domain.TurnQueued, Mode: "structured",
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if ok, err := s.ClaimActiveTurn("branch_main", secTurnID); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	br, _ := s.GetBranch("branch_main")
	secPayload, _ := json.Marshal(domain.SecretUnlockPayload{SecretID: "sec_locket", Title: "银质挂坠盒"})
	res, err := s.CommitTurn(&ports.CommitPlan{
		TurnID: secTurnID, ExpectedHeadID: n1, ExpectedVersion: br.Version,
		BaseStateHash: bs.StateHash, RulesetVersion: "test",
		Node: &domain.PlotNode{
			NodeID: secNodeID, SessionID: sess.SessionID, ParentID: n1,
			Kind: domain.NodeKindTurn, Depth: parent.Depth + 1, TurnNumber: parent.TurnNumber + 1,
			SchemaVersion: 1, ContentJSON: `{}`,
		},
		Events: []*domain.DomainEvent{
			{EventID: id.New(), NodeID: secNodeID, EventIndex: 0, Type: domain.EventSecretUnlock, PayloadJSON: string(secPayload), RulesetVersion: "test"},
		},
		NewStateHash: st2.HashID(), NewStateJSON: st2JSON,
	})
	if err != nil || !res.Committed {
		t.Fatalf("commit sec: %v %+v", err, res)
	}

	// Turn 3: relationship_delta
	n3, _ := commitChainTurn(t, s, sess, secNodeID, 10)

	// Query both types
	events, err := s.EventsOnPath(n3, domain.EventRelationshipDelta, domain.EventSecretUnlock)
	if err != nil {
		t.Fatalf("EventsOnPath: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].TurnNumber != 1 || events[0].Event.Type != domain.EventRelationshipDelta {
		t.Errorf("ev[0]: turn=%d type=%s", events[0].TurnNumber, events[0].Event.Type)
	}
	if events[1].TurnNumber != 2 || events[1].Event.Type != domain.EventSecretUnlock {
		t.Errorf("ev[1]: turn=%d type=%s", events[1].TurnNumber, events[1].Event.Type)
	}
	if events[2].TurnNumber != 3 || events[2].Event.Type != domain.EventRelationshipDelta {
		t.Errorf("ev[2]: turn=%d type=%s", events[2].TurnNumber, events[2].Event.Type)
	}

	// Query only secret_unlock
	secEvents, err := s.EventsOnPath(n3, domain.EventSecretUnlock)
	if err != nil {
		t.Fatalf("EventsOnPath sec only: %v", err)
	}
	if len(secEvents) != 1 {
		t.Fatalf("expected 1 secret event, got %d", len(secEvents))
	}
	if secEvents[0].TurnNumber != 2 {
		t.Errorf("expected turn 2, got %d", secEvents[0].TurnNumber)
	}
}
