package sqlite

import (
	"database/sql"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// DeleteSession 永久删除一个会话的全部数据。
//
// 为什么不是一句 DELETE：schema 里除了 lorebook 索引之外没有任何 ON DELETE
// CASCADE，而外键是开启的（writer DSN 带 foreign_keys(1)）。所以必须显式清掉
// 所有引用该会话的行，并且**在同一个事务里**完成，避免中途失败留下半删状态。
//
// 顺序仍然按"先引用方"排列（可读性 + 便于排查），但事务内额外打开
// defer_foreign_keys，把外键校验推迟到 COMMIT：这样即使某条语句的顺序被后人
// 调整，也不会因为声明顺序而误报外键错误——真正漏删的行会在提交时暴露。
//
// 不删除 template_versions：模板按内容寻址、可能被多个会话共享，任何单个会话
// 都不拥有它。共享模板的回收是独立议题（需要跨会话引用计数）。
func (s *Store) DeleteSession(sessionID string) error {
	if sessionID == "" {
		return ports.ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`PRAGMA defer_foreign_keys=ON`); err != nil {
		return err
	}

	// 会话不存在时直接返回，避免把 delete 当成"删了 0 行也算成功"。
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM sessions WHERE session_id=?`, sessionID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ports.ErrNotFound
	}

	// 在途回合会与删除竞争（生成线程随后要写事件/节点），拒绝而不是强行删除。
	var busy int
	if err := tx.QueryRow(`
		SELECT COUNT(1) FROM turn_requests
		WHERE session_id=? AND status NOT IN ('committed','cancelled','failed','conflicted','awaiting_continuation')`,
		sessionID).Scan(&busy); err != nil {
		return err
	}
	if busy > 0 {
		return ports.ErrSessionBusy
	}

	// 按"会话 → 节点/分支/回合"三个维度派生的子表，逐层清空。
	steps := []struct {
		sql string
		arg string
	}{
		// 回合系：草稿帧 → 尝试 → 回执 → 用量（turn_usage 无 session 列，经 turn_requests 连接）
		{`DELETE FROM draft_frames WHERE attempt_id IN (
			SELECT a.attempt_id FROM turn_attempts a JOIN turn_requests t ON t.turn_id=a.turn_id WHERE t.session_id=?)`, sessionID},
		{`DELETE FROM turn_attempts WHERE turn_id IN (SELECT turn_id FROM turn_requests WHERE session_id=?)`, sessionID},
		{`DELETE FROM action_receipts WHERE turn_id IN (SELECT turn_id FROM turn_requests WHERE session_id=?)`, sessionID},
		{`DELETE FROM turn_usage WHERE turn_id IN (SELECT turn_id FROM turn_requests WHERE session_id=?)`, sessionID},

		// 记忆系：提及 → 投影快照 → 记录本体（FTS 行在上面单独处理）
		{`DELETE FROM memory_mentions WHERE node_id IN (SELECT node_id FROM plot_nodes WHERE session_id=?)`, sessionID},
		{`DELETE FROM memory_projection_snapshots WHERE node_id IN (SELECT node_id FROM plot_nodes WHERE session_id=?)`, sessionID},
		{`DELETE FROM memory_records WHERE source_node_id IN (SELECT node_id FROM plot_nodes WHERE session_id=?)`, sessionID},
		{`DELETE FROM memory_batches WHERE session_id=?`, sessionID},

		// 导演系：草稿 / 请求 / 命令 / 投影
		{`DELETE FROM director_drafts WHERE session_id=?`, sessionID},
		{`DELETE FROM director_requests WHERE session_id=?`, sessionID},
		{`DELETE FROM director_commands WHERE session_id=?`, sessionID},
		{`DELETE FROM director_projections WHERE node_id IN (SELECT node_id FROM plot_nodes WHERE session_id=?)`, sessionID},

		// 剧情系：事件、快照、书签、outbox
		{`DELETE FROM domain_events WHERE node_id IN (SELECT node_id FROM plot_nodes WHERE session_id=?)`, sessionID},
		{`DELETE FROM state_snapshots WHERE node_id IN (SELECT node_id FROM plot_nodes WHERE session_id=?)`, sessionID},
		{`DELETE FROM bookmarks WHERE session_id=?`, sessionID},
		{`DELETE FROM outbox_events WHERE aggregate_id=?`, sessionID},
		{`DELETE FROM outbox_events WHERE aggregate_id IN (SELECT turn_id FROM turn_requests WHERE session_id=?)`, sessionID},
		{`DELETE FROM outbox_events WHERE aggregate_id IN (SELECT branch_id FROM branches WHERE session_id=?)`, sessionID},

		// 主体：分支（引用 head_node）→ 节点 → 回合请求 → 会话
		{`DELETE FROM branches WHERE session_id=?`, sessionID},
		{`DELETE FROM plot_nodes WHERE session_id=?`, sessionID},
		{`DELETE FROM turn_requests WHERE session_id=?`, sessionID},
		{`DELETE FROM sessions WHERE session_id=?`, sessionID},
	}

	// FTS 是虚拟表、无外键，且行号对齐 memory_records.rowid：必须在删除记录本体前清掉，
	// 否则索引里会留下指向已删除记忆的行。索引不可用（探测失败）时表不存在，跳过。
	if s.fts {
		if err := deleteMemoryIndexRows(tx, sessionID, memoryFTSTable); err != nil {
			return err
		}
	}
	if s.trigram {
		if err := deleteMemoryIndexRows(tx, sessionID, memoryFTSTriTable); err != nil {
			return err
		}
	}
	for _, step := range steps {
		if _, err := tx.Exec(step.sql, step.arg); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// 投影缓存按节点键持有记忆，被删会话的节点必须清出缓存，否则同进程后续读取
	// 可能命中已删除的记忆。
	s.dropProjectionCache()

	return nil
}

// deleteMemoryIndexRows 清掉某会话下所有记忆的索引行。
func deleteMemoryIndexRows(tx *sql.Tx, sessionID, table string) error {
	_, err := tx.Exec(`DELETE FROM `+table+` WHERE rowid IN (
		SELECT rowid FROM memory_records WHERE source_node_id IN (
			SELECT node_id FROM plot_nodes WHERE session_id=?))`, sessionID)
	return err
}

// dropProjectionCache 清空记忆投影缓存（被删会话的节点已不存在，缓存内容失效）。
func (s *Store) dropProjectionCache() {
	s.projectionMu.Lock()
	defer s.projectionMu.Unlock()
	s.projectionCache = map[string][]*domain.MemoryRecord{}
	s.projectionOrder = nil
}
