package sqlite

import (
	"database/sql"
	"errors"
	"strings"

	"tavernagent/internal/domain"
	"tavernagent/internal/pack"
	"tavernagent/internal/ports"
)

// ---- 剧情包：导出读取（M3 · T21）----

// ExportSession 以一致读取出会话的完整可携带快照。
//
// 一致性：多张表的读取放在同一个读事务里，导出期间不会看到"提交到一半"的组合。
// branchID 非空时只导出该分支的头节点祖先链（仍含全部必要祖先），
// 因为「仅导出一个分支时仍包含它的全部必要祖先和相关模板」（契约 §12.2）。
func (s *Store) ExportSession(sessionID, branchID string) (*domain.SessionBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	sess, err := s.sessionQ(tx, sessionID)
	if err != nil {
		if errors.Is(err, domain.ErrItemNotFound) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	branches, err := branchesQ(tx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(branches) == 0 {
		return nil, ports.ErrNotFound
	}

	scope := domain.ExportScope{Full: branchID == ""}
	var nodes []*domain.PlotNode
	if branchID == "" {
		nodes, err = nodesQ(tx, sessionID)
	} else {
		var br *domain.Branch
		for _, b := range branches {
			if b.BranchID == branchID {
				br = b
				break
			}
		}
		if br == nil {
			return nil, ports.ErrNotFound
		}
		scope.BranchID, scope.BranchName = br.BranchID, br.Name
		nodes, err = ancestorChainQ(tx, br.HeadNodeID, true)
		branches = []*domain.Branch{br}
	}
	if err != nil {
		return nil, err
	}

	nodeIDs := make([]string, 0, len(nodes))
	rootTemplateIDs := []string{}
	for _, n := range nodes {
		nodeIDs = append(nodeIDs, n.NodeID)
		if n.Kind == domain.NodeKindRoot {
			rootTemplateIDs = append(rootTemplateIDs, pack.TemplateRefsOfRoot(n.ContentJSON)...)
		}
	}

	events, err := eventsForNodesQ(tx, nodeIDs)
	if err != nil {
		return nil, err
	}
	memories, err := memoriesForNodesQ(tx, nodeIDs)
	if err != nil {
		return nil, err
	}
	receipts, err := receiptsForNodesQ(tx, nodeIDs)
	if err != nil {
		return nil, err
	}
	snapshots, err := snapshotsForNodesQ(tx, nodeIDs)
	if err != nil {
		return nil, err
	}
	templates, err := templatesQ(tx, rootTemplateIDs)
	if err != nil {
		return nil, err
	}
	bookmarks, err := bookmarksQ(tx, sessionID)
	if err != nil {
		return nil, err
	}

	return &domain.SessionBundle{
		FormatVersion:  domain.PackVersionForEvents(events),
		RulesetVersion: sess.RulesetVersion,
		Scope:          scope,
		Session:        sess,
		RootNodeID:     sess.RootNodeID,
		Templates:      templates,
		Nodes:          nodes,
		Events:         events,
		Branches:       branches,
		Memories:       memories,
		Receipts:       receipts,
		Bookmarks:      bookmarks,
		Snapshots:      snapshots,
	}, nil
}

// ---- 读事务内的查询 ----

func (s *Store) sessionQ(q queryer, sessionID string) (*domain.Session, error) {
	row := q.QueryRow(`SELECT session_id, root_node_id, title, created_at, ruleset_version, character_id, updated_at FROM sessions WHERE session_id=?`, sessionID)
	var sess domain.Session
	var at, updated string
	var ruleset sql.NullString
	if err := row.Scan(&sess.SessionID, &sess.RootNodeID, &sess.Title, &at, &ruleset, &sess.CharacterID, &updated); err != nil {
		return nil, mapErr(err)
	}
	sess.CreatedAt = s.parseTime(at)
	sess.UpdatedAt = s.parseTime(updated)
	sess.RulesetVersion = ruleset.String
	return &sess, nil
}

func branchesQ(q queryer, sessionID string) ([]*domain.Branch, error) {
	rows, err := q.Query(`SELECT branch_id, session_id, name, head_node_id, version FROM branches WHERE session_id=? ORDER BY created_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Branch
	for rows.Next() {
		var b domain.Branch
		if err := rows.Scan(&b.BranchID, &b.SessionID, &b.Name, &b.HeadNodeID, &b.Version); err != nil {
			return nil, err
		}
		out = append(out, &b)
	}
	return out, rows.Err()
}

func nodesQ(q queryer, sessionID string) ([]*domain.PlotNode, error) {
	rows, err := q.Query(`SELECT `+nodeCols+` FROM plot_nodes WHERE session_id=? ORDER BY depth, created_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.PlotNode
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// inBatches 按固定批大小执行 IN 查询，避免超出 SQLite 的变量数量上限。
func inBatches(ids []string, size int, fn func(batch []string) error) error {
	if size <= 0 {
		size = 400
	}
	for start := 0; start < len(ids); start += size {
		end := start + size
		if end > len(ids) {
			end = len(ids)
		}
		if err := fn(ids[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func inClause(ids []string) (string, []any) {
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	return strings.Repeat("?,", len(ids)-1) + "?", args
}

func eventsForNodesQ(q queryer, nodeIDs []string) ([]*domain.DomainEvent, error) {
	out := []*domain.DomainEvent{}
	err := inBatches(nodeIDs, 400, func(batch []string) error {
		ph, args := inClause(batch)
		rows, err := q.Query(`SELECT event_id, node_id, event_index, type, payload_json, ruleset_version FROM domain_events WHERE node_id IN (`+ph+`) ORDER BY node_id, event_index`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ev domain.DomainEvent
			var typ string
			if err := rows.Scan(&ev.EventID, &ev.NodeID, &ev.EventIndex, &typ, &ev.PayloadJSON, &ev.RulesetVersion); err != nil {
				return err
			}
			ev.Type = domain.EventType(typ)
			out = append(out, &ev)
		}
		return rows.Err()
	})
	return out, err
}

func memoriesForNodesQ(q queryer, nodeIDs []string) ([]*domain.MemoryRecord, error) {
	out := []*domain.MemoryRecord{}
	err := inBatches(nodeIDs, 400, func(batch []string) error {
		ph, args := inClause(batch)
		rows, err := q.Query(`SELECT `+memoryCols+` FROM memory_records WHERE source_node_id IN (`+ph+`) ORDER BY memory_id`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			m, err := scanMemory(rows)
			if err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

func snapshotsForNodesQ(q queryer, nodeIDs []string) ([]*domain.StateSnapshot, error) {
	out := []*domain.StateSnapshot{}
	err := inBatches(nodeIDs, 400, func(batch []string) error {
		ph, args := inClause(batch)
		rows, err := q.Query(`SELECT node_id, snapshot_version, ruleset_version, state_json, state_hash FROM state_snapshots WHERE node_id IN (`+ph+`)`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sn domain.StateSnapshot
			var ruleset sql.NullString
			if err := rows.Scan(&sn.NodeID, &sn.SnapshotVersion, &ruleset, &sn.StateJSON, &sn.StateHash); err != nil {
				return err
			}
			sn.RulesetVersion = ruleset.String
			out = append(out, &sn)
		}
		return rows.Err()
	})
	return out, err
}

func templatesQ(q queryer, ids []string) ([]*domain.TemplateVersion, error) {
	out := []*domain.TemplateVersion{}
	if len(ids) == 0 {
		return out, nil
	}
	err := inBatches(ids, 400, func(batch []string) error {
		ph, args := inClause(batch)
		rows, err := q.Query(`SELECT template_version_id, kind, schema_version, content, content_hash FROM template_versions WHERE template_version_id IN (`+ph+`)`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t domain.TemplateVersion
			var kind string
			if err := rows.Scan(&t.TemplateVersionID, &kind, &t.SchemaVersion, &t.Content, &t.ContentHash); err != nil {
				return err
			}
			t.Kind = domain.TemplateKind(kind)
			out = append(out, &t)
		}
		return rows.Err()
	})
	return out, err
}

func bookmarksQ(q queryer, sessionID string) ([]domain.Bookmark, error) {
	rows, err := q.Query(`SELECT bookmark_id, session_id, node_id, title FROM bookmarks WHERE session_id=? ORDER BY bookmark_id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Bookmark{}
	for rows.Next() {
		var b domain.Bookmark
		if err := rows.Scan(&b.BookmarkID, &b.SessionID, &b.NodeID, &b.Title); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
