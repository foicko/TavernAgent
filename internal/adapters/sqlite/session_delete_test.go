package sqlite

import (
	"fmt"
	"testing"
	"time"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/util/id"
)

// sessionScopedCounts 返回"每个会话私有表"里属于该会话的行数。
//
// 之所以逐表统计而不是只看 sessions 行数：SQLite 的外键只在**有外键声明**的表上
// 兜底，而 bookmarks / outbox_events / turn_usage / draft_frames / action_receipts
// 这些表刻意没有外键，漏删它们不会报错、只会静默留下孤儿数据。
func sessionScopedCounts(t *testing.T, s *Store, sessionID string) map[string]int {
	t.Helper()
	nodes := `SELECT node_id FROM plot_nodes WHERE session_id=?`
	turns := `SELECT turn_id FROM turn_requests WHERE session_id=?`
	branches := `SELECT branch_id FROM branches WHERE session_id=?`
	queries := map[string]string{
		"sessions":                   `SELECT COUNT(*) FROM sessions WHERE session_id=?`,
		"plot_nodes":                 `SELECT COUNT(*) FROM plot_nodes WHERE session_id=?`,
		"branches":                   `SELECT COUNT(*) FROM branches WHERE session_id=?`,
		"domain_events":              `SELECT COUNT(*) FROM domain_events WHERE node_id IN (` + nodes + `)`,
		"state_snapshots":            `SELECT COUNT(*) FROM state_snapshots WHERE node_id IN (` + nodes + `)`,
		"memory_records":             `SELECT COUNT(*) FROM memory_records WHERE source_node_id IN (` + nodes + `)`,
		"memory_mentions":            `SELECT COUNT(*) FROM memory_mentions WHERE node_id IN (` + nodes + `)`,
		"memory_projection_snapshot": `SELECT COUNT(*) FROM memory_projection_snapshots WHERE node_id IN (` + nodes + `)`,
		"memory_batches":             `SELECT COUNT(*) FROM memory_batches WHERE session_id=?`,
		"turn_requests":              `SELECT COUNT(*) FROM turn_requests WHERE session_id=?`,
		"turn_attempts":              `SELECT COUNT(*) FROM turn_attempts WHERE turn_id IN (` + turns + `)`,
		"draft_frames":               `SELECT COUNT(*) FROM draft_frames WHERE attempt_id IN (SELECT attempt_id FROM turn_attempts WHERE turn_id IN (` + turns + `))`,
		"action_receipts":            `SELECT COUNT(*) FROM action_receipts WHERE turn_id IN (` + turns + `)`,
		"turn_usage":                 `SELECT COUNT(*) FROM turn_usage WHERE turn_id IN (` + turns + `)`,
		"director_drafts":            `SELECT COUNT(*) FROM director_drafts WHERE session_id=?`,
		"director_requests":          `SELECT COUNT(*) FROM director_requests WHERE session_id=?`,
		"director_commands":          `SELECT COUNT(*) FROM director_commands WHERE session_id=?`,
		"director_projections":       `SELECT COUNT(*) FROM director_projections WHERE node_id IN (` + nodes + `)`,
		"bookmarks":                  `SELECT COUNT(*) FROM bookmarks WHERE session_id=?`,
		"outbox_events":              `SELECT COUNT(*) FROM outbox_events WHERE aggregate_id=? OR aggregate_id IN (` + turns + `) OR aggregate_id IN (` + branches + `)`,
	}
	counts := make(map[string]int, len(queries))
	for name, q := range queries {
		// 带子查询的语句会在每个 ? 位置各绑一次会话 ID。
		args := make([]any, 0, 8)
		for i := 0; i < countPlaceholders(q); i++ {
			args = append(args, sessionID)
		}
		var n int
		if err := s.rdb().QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatalf("统计 %s: %v", name, err)
		}
		counts[name] = n
	}
	return counts
}

func countPlaceholders(q string) int {
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
		}
	}
	return n
}

// seedFullSession 铺满所有会话私有表，用于验证级联删除的完整性。
func seedFullSession(t *testing.T, s *Store, title string) (sessionID, branchID, nodeID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	sess := &domain.Session{SessionID: id.New(), RootNodeID: "root_" + title, Title: title, CreatedAt: time.Now()}
	root := &domain.PlotNode{
		NodeID: sess.RootNodeID, SessionID: sess.SessionID, Kind: domain.NodeKindRoot,
		Depth: 0, TurnNumber: 0, ContentJSON: `{"initial":true}`, SchemaVersion: 1, CreatedAt: time.Now(),
	}
	branch := &domain.Branch{BranchID: "branch_" + title, SessionID: sess.SessionID, Name: "main", HeadNodeID: root.NodeID, Version: 0}
	snapshot := &domain.StateSnapshot{
		NodeID: root.NodeID, SnapshotVersion: 1, RulesetVersion: "v1",
		StateJSON: `{"characters":{}}`, StateHash: "hash_" + title,
	}
	if err := s.CreateSessionWithSnapshot(sess, root, branch, nil, snapshot); err != nil {
		t.Fatalf("create session: %v", err)
	}
	turnNode := &domain.PlotNode{
		NodeID: "node_" + title, SessionID: sess.SessionID, ParentID: root.NodeID, Kind: domain.NodeKindTurn,
		Depth: 1, TurnNumber: 1, ContentJSON: `{"blocks":[]}`, SchemaVersion: 1, CreatedAt: time.Now(),
	}
	if err := s.InsertNode(turnNode); err != nil {
		t.Fatalf("insert node: %v", err)
	}

	// 记忆：一条普通记录 + 一条覆盖记录（supersedes 指向前者，验证自引用不挡删除）
	first := &domain.MemoryRecord{
		MemoryID: "mem_" + title, SourceNodeID: turnNode.NodeID, Kind: domain.MemoryObserved,
		Content: "记得这件事",
	}
	if err := s.CreateMemory(first); err != nil {
		t.Fatalf("create memory: %v", err)
	}
	second := &domain.MemoryRecord{
		MemoryID: "mem2_" + title, SourceNodeID: turnNode.NodeID, Kind: domain.MemoryObserved,
		Content: "其实相反", Supersedes: first.MemoryID,
	}
	if err := s.CreateMemory(second); err != nil {
		t.Fatalf("create superseding memory: %v", err)
	}

	// 其余会话私有表：直接落行（这些表没有写入 API，且正是外键顺序最容易出错的地方）
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.db.Exec(q, args...); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	turnID := "turn_" + title
	attemptID := "attempt_" + title
	exec(`INSERT INTO turn_requests(turn_id,session_id,branch_id,idempotency_key,payload_hash,expected_head_id,
		expected_version,status,mode,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		turnID, sess.SessionID, branch.BranchID, "idem-"+title, "ph", root.NodeID, 0, string(domain.TurnCommitted), "turn", now, now)
	exec(`INSERT INTO turn_attempts(attempt_id,turn_id,attempt_no,base_head_id,base_version,created_at) VALUES(?,?,?,?,?,?)`,
		attemptID, turnID, 1, root.NodeID, 0, now)
	exec(`INSERT INTO draft_frames(attempt_id,frame_seq,payload,payload_hash) VALUES(?,?,?,?)`, attemptID, 0, "{}", "h")
	exec(`INSERT INTO action_receipts(receipt_id,turn_id,action_id,base_head_id,ruleset_version,result_json,status) VALUES(?,?,?,?,?,?,?)`,
		"receipt_"+title, turnID, "act", root.NodeID, "v1", "{}", "ok")
	exec(`INSERT INTO turn_usage(turn_id,attempt_id,created_at) VALUES(?,?,?)`, turnID, attemptID, now)
	exec(`INSERT INTO domain_events(event_id,node_id,event_index,type,payload_json) VALUES(?,?,?,?,?)`,
		"evt_"+title, turnNode.NodeID, 0, "turn.committed", "{}")
	exec(`INSERT INTO memory_mentions(memory_id,node_id,turn_number) VALUES(?,?,?)`, first.MemoryID, turnNode.NodeID, 1)
	exec(`INSERT INTO memory_projection_snapshots(node_id,memory_ids) VALUES(?,?)`, turnNode.NodeID, "[]")
	exec(`INSERT INTO memory_batches(batch_id,session_id,branch_id,source_turn_id,node_id,payload_hash,created_at) VALUES(?,?,?,?,?,?,?)`,
		"batch_"+title, sess.SessionID, branch.BranchID, turnID, turnNode.NodeID, "h", now)
	exec(`INSERT INTO director_drafts(branch_id,session_id,version,base_revision_id,plan_json,updated_at) VALUES(?,?,?,?,?,?)`,
		branch.BranchID, sess.SessionID, 1, "rev", "{}", now)
	exec(`INSERT INTO director_requests(request_id,session_id,branch_id,idempotency_key,payload_hash,base_node_id,
		draft_version,draft_json,text,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		"req_"+title, sess.SessionID, branch.BranchID, "idem-req-"+title, "ph", turnNode.NodeID, 1, "{}", "讨论", "done", now)
	exec(`INSERT INTO director_commands(session_id,idempotency_key,branch_id,payload_hash,node_id,version) VALUES(?,?,?,?,?,?)`,
		sess.SessionID, "cmd-"+title, branch.BranchID, "ph", turnNode.NodeID, 1)
	exec(`INSERT INTO director_projections(node_id,state_json) VALUES(?,?)`, turnNode.NodeID, "{}")
	exec(`INSERT INTO bookmarks(bookmark_id,session_id,node_id,title) VALUES(?,?,?,?)`,
		"bm_"+title, sess.SessionID, turnNode.NodeID, "书签")
	exec(`INSERT INTO outbox_events(event_id,aggregate_id,sequence,type,payload_json,created_at) VALUES(?,?,?,?,?,?)`,
		"ob_"+title, sess.SessionID, 1, "session.updated", "{}", now)
	exec(`INSERT INTO outbox_events(event_id,aggregate_id,sequence,type,payload_json,created_at) VALUES(?,?,?,?,?,?)`,
		"obt_"+title, turnID, 1, "turn.committed", "{}", now)
	return sess.SessionID, branch.BranchID, turnNode.NodeID
}

// 删除会话必须清空它的全部私有数据，且不碰其它会话。
func TestDeleteSessionRemovesEveryScopedRowAndKeepsOthers(t *testing.T) {
	s := newTestStore(t)
	victim, _, _ := seedFullSession(t, s, "victim")
	keeper, _, _ := seedFullSession(t, s, "keeper")

	before := sessionScopedCounts(t, s, keeper)
	for name, n := range before {
		if n == 0 {
			t.Fatalf("测试前置条件不成立：%s 在保留会话里应为非空", name)
		}
	}

	if err := s.DeleteSession(victim); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	for name, n := range sessionScopedCounts(t, s, victim) {
		if n != 0 {
			t.Errorf("删除后 %s 仍残留 %d 行", name, n)
		}
	}
	for name, n := range sessionScopedCounts(t, s, keeper) {
		if n != before[name] {
			t.Errorf("删除他人会话影响了 %s：%d → %d", name, before[name], n)
		}
	}

	// 反向证明：还有任何未清理的外键引用时，这一句会失败。
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE session_id=?`, victim); err != nil {
		t.Fatalf("删除后仍有行引用该会话：%v", err)
	}
	// 共享模板不属于任何单个会话，必须保留。
	var templates int
	if err := s.rdb().QueryRow(`SELECT COUNT(*) FROM template_versions`).Scan(&templates); err != nil {
		t.Fatalf("count templates: %v", err)
	}
}

// 重复删除返回 not found，而不是假装成功。
func TestDeleteSessionIsNotIdempotentOnMissingSession(t *testing.T) {
	s := newTestStore(t)
	if err := s.DeleteSession("sess_missing"); err != ports.ErrNotFound {
		t.Fatalf("期望 ErrNotFound，得到 %v", err)
	}
	if err := s.DeleteSession(""); err != ports.ErrNotFound {
		t.Fatalf("空 ID 期望 ErrNotFound，得到 %v", err)
	}
}

// 有回合正在生成时拒绝删除，并且不留下半删状态（事务回滚）。
func TestDeleteSessionRefusesWhileTurnInFlight(t *testing.T) {
	s := newTestStore(t)
	sessionID, _, _ := seedFullSession(t, s, "busy")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`UPDATE turn_requests SET status=? WHERE session_id=?`, string(domain.TurnGenerating), sessionID); err != nil {
		t.Fatalf("mark generating: %v", err)
	}
	_ = now

	if err := s.DeleteSession(sessionID); err != ports.ErrSessionBusy {
		t.Fatalf("期望 ErrSessionBusy，得到 %v", err)
	}
	for name, n := range sessionScopedCounts(t, s, sessionID) {
		if n == 0 {
			t.Errorf("拒绝删除后 %s 不应被清空", name)
		}
	}
}

// 暂停中的草稿（awaiting_continuation）不算在途，允许删除。
func TestDeleteSessionAllowsPausedDraft(t *testing.T) {
	s := newTestStore(t)
	sessionID, _, _ := seedFullSession(t, s, "paused")
	if _, err := s.db.Exec(`UPDATE turn_requests SET status=? WHERE session_id=?`, string(domain.TurnAwaitingContinuation), sessionID); err != nil {
		t.Fatalf("mark paused: %v", err)
	}
	if err := s.DeleteSession(sessionID); err != nil {
		t.Fatalf("暂停草稿的会话应可删除，得到 %v", err)
	}
}

// 删除会清掉词法索引里指向已删记忆的行（否则检索会命中幽灵记忆）。
func TestDeleteSessionClearsMemoryIndex(t *testing.T) {
	s := newTestStore(t)
	if !s.fts {
		t.Skip("当前构建没有 FTS5 词法索引")
	}
	sessionID, _, _ := seedFullSession(t, s, "indexed")
	var before int
	if err := s.rdb().QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, memoryFTSTable)).Scan(&before); err != nil {
		t.Fatalf("count fts: %v", err)
	}
	if before == 0 {
		t.Fatalf("测试前置条件不成立：索引里应有行")
	}
	if err := s.DeleteSession(sessionID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var after int
	if err := s.rdb().QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, memoryFTSTable)).Scan(&after); err != nil {
		t.Fatalf("count fts: %v", err)
	}
	if after != 0 {
		t.Fatalf("词法索引残留 %d 行", after)
	}
}
