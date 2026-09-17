package sqlite

import (
	"database/sql"
	"time"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

func insertNodeTx(ex execer, n *domain.PlotNode, createdAt string) error {
	_, err := ex.Exec(`INSERT INTO plot_nodes(node_id, session_id, parent_id, kind, depth, turn_number, schema_version, content_json, created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		n.NodeID, n.SessionID, n.ParentID, string(n.Kind), n.Depth, n.TurnNumber, n.SchemaVersion, n.ContentJSON, createdAt)
	return err
}

func scanNode(row interface{ Scan(...any) error }) (*domain.PlotNode, error) {
	var n domain.PlotNode
	var sessionID, kind, at, parent sql.NullString
	if err := row.Scan(&n.NodeID, &sessionID, &parent, &kind, &n.Depth, &n.TurnNumber, &n.SchemaVersion, &n.ContentJSON, &at); err != nil {
		return nil, mapErr(err)
	}
	n.SessionID = sessionID.String
	n.ParentID = parent.String
	n.Kind = domain.NodeKind(kind.String)
	if t, err := time.Parse(time.RFC3339Nano, at.String); err == nil {
		n.CreatedAt = t
	}
	return &n, nil
}

const nodeCols = "node_id, session_id, parent_id, kind, depth, turn_number, schema_version, content_json, created_at"

func (s *Store) InsertNode(n *domain.PlotNode) error {
	return insertNodeTx(s.db, n, s.now())
}

func (s *Store) GetNode(nodeID string) (*domain.PlotNode, error) {
	row := s.rdb().QueryRow(`SELECT `+nodeCols+` FROM plot_nodes WHERE node_id=?`, nodeID)
	return scanNode(row)
}

func (s *Store) ListChildren(sessionID, parentID string) ([]*domain.PlotNode, error) {
	rows, err := s.rdb().Query(`SELECT `+nodeCols+` FROM plot_nodes WHERE session_id=? AND parent_id=? ORDER BY depth, created_at`, sessionID, parentID)
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

// ListChildrenOf 批量取若干父节点的子节点，按父节点分组。
// 一次取全"同父候选组"，避免对链上每个回合各查一次（N+1）。
// 没有子节点的父节点不会出现在结果里。
func (s *Store) ListChildrenOf(sessionID string, parentIDs []string) (map[string][]*domain.PlotNode, error) {
	out := map[string][]*domain.PlotNode{}
	if len(parentIDs) == 0 {
		return out, nil
	}
	placeholders := ""
	args := make([]any, 0, len(parentIDs)+1)
	args = append(args, sessionID)
	for i, pid := range parentIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, pid)
	}
	rows, err := s.rdb().Query(`SELECT `+nodeCols+` FROM plot_nodes WHERE session_id=? AND parent_id IN (`+placeholders+`) ORDER BY depth, created_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out[n.ParentID] = append(out[n.ParentID], n)
	}
	return out, rows.Err()
}

func (s *Store) AncestorChain(nodeID string, fromRoot bool) ([]*domain.PlotNode, error) {
	return ancestorChainQ(s.rdb(), nodeID, fromRoot)
}

func ancestorChainQ(q queryer, nodeID string, fromRoot bool) ([]*domain.PlotNode, error) {
	// 递归 CTE 向父方向攀升：parent_id 无环（导入处校验）。
	const query = `
WITH RECURSIVE chain(node_id) AS (
  SELECT node_id FROM plot_nodes WHERE node_id = ?
  UNION ALL
  SELECT p.parent_id FROM plot_nodes p JOIN chain c ON p.node_id = c.node_id AND p.parent_id IS NOT NULL
)
SELECT ` + nodeCols + ` FROM plot_nodes WHERE node_id IN (SELECT node_id FROM chain) ORDER BY depth ASC, created_at ASC
`
	rows, err := q.Query(query, nodeID)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, sql.ErrNoRows
	}
	if !fromRoot && len(out) > 1 {
		out = out[1:] // 去掉根，返回从根子节点到目标
	}
	return out, nil
}

// RecentTurnNodes 返回向上最近的 limit 个回合节点（按深度正序）。
//
// 为什么限深：正常数据里每个回合占一层，向上 limit*4 层足以覆盖 limit 个回合节点，
// 于是查询代价与链长无关。若返回不足（非回合节点密集，例如导入的配置节点），
// 回退到不限深遍历保证正确性——但那条路径会走到根，所以只在异常形态下付出代价。
func (s *Store) RecentTurnNodes(nodeID string, limit int) ([]*domain.PlotNode, error) {
	if limit <= 0 {
		return nil, nil
	}
	nodes, err := recentTurnsQ(s.rdb(), nodeID, limit*4)
	if err != nil {
		return nil, err
	}
	if len(nodes) >= limit {
		return tailNodes(nodes, limit), nil
	}
	nodes, err = recentTurnsQ(s.rdb(), nodeID, 0)
	if err != nil {
		return nil, err
	}
	return tailNodes(nodes, limit), nil
}

func recentTurnsQ(q queryer, nodeID string, maxDepth int) ([]*domain.PlotNode, error) {
	depthClause := ""
	args := []any{nodeID}
	if maxDepth > 0 {
		depthClause = " AND u.n < ?"
		args = append(args, maxDepth)
	}
	args = append(args, string(domain.NodeKindTurn))
	// 递归项只带节点 ID 与步数：早期版本把"当前节点的 depth"写进父节点的行，
	// 导致排序键错位（父节点拿到子节点的深度），链一长返回顺序就乱。
	// 深度统一从最终 JOIN 的 plot_nodes 取，排序才可靠。
	query := `
WITH RECURSIVE up(node_key, n) AS (
  SELECT node_id, 0 FROM plot_nodes WHERE node_id = ?
  UNION ALL
  SELECT p.parent_id, u.n + 1 FROM plot_nodes p JOIN up u ON p.node_id = u.node_key
   WHERE p.parent_id IS NOT NULL` + depthClause + `
)
SELECT ` + nodeCols + ` FROM plot_nodes p JOIN up u ON u.node_key = p.node_id
 WHERE p.kind = ?
 ORDER BY p.depth ASC`
	rows, err := q.Query(query, args...)
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

func tailNodes(nodes []*domain.PlotNode, limit int) []*domain.PlotNode {
	if len(nodes) <= limit {
		return nodes
	}
	return nodes[len(nodes)-limit:]
}

// MemoriesOnPath 返回从根到该节点这条路径上的全部记忆（词法索引不可用时的兜底）。
// 用递归 CTE 直接在 SQL 里按路径过滤（走 idx_memory_source），
// 避免把整条祖先链读进 Go 再筛——那是"读全链"里代价最高的一处。
func (s *Store) MemoriesOnPath(nodeID string) ([]*ports.MemoryCandidate, error) {
	return memoriesOnPathQ(s.rdb(), nodeID)
}

func memoriesOnPathQ(q queryer, nodeID string) ([]*ports.MemoryCandidate, error) {
	// 兜底路径：这里取**全部**可见记忆（含隐藏与覆盖记录），不设上限。
	// 词法排名留给调用方在 Go 侧算，因此 HasRank 恒为 false。
	query := `
WITH RECURSIVE up(node_key) AS (
  SELECT node_id FROM plot_nodes WHERE node_id = ?
  UNION ALL
  SELECT p.parent_id FROM plot_nodes p JOIN up u ON p.node_id = u.node_key
   WHERE p.parent_id IS NOT NULL
)
SELECT ` + prefixed(memoryCols, "m") + `, sn.turn_number
  FROM memory_records m
  JOIN up u ON u.node_key = m.source_node_id
  JOIN plot_nodes sn ON sn.node_id = m.source_node_id
 ORDER BY m.memory_id`
	rows, err := q.Query(query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ports.MemoryCandidate{}
	for rows.Next() {
		var m domain.MemoryRecord
		var kind, owners, entities, metadata string
		var pinned, hidden int
		var sourceTurn int
		if err := rows.Scan(&m.MemoryID, &m.SourceNodeID, &kind, &owners, &m.Content,
			&entities, &m.Confidence, &m.Importance, &pinned, &m.Supersedes, &hidden, &metadata,
			&m.SubjectKey, &m.CreatedTurn, &m.ValidFromTurn, &m.ValidUntilTurn,
			&sourceTurn); err != nil {
			return nil, err
		}
		m.Kind = domain.MemoryKind(kind)
		m.Pinned = pinned != 0
		m.Hidden = hidden != 0
		decodeIDList(owners, "ownerIds", &m.OwnerIDs)
		decodeIDList(entities, "entityIds", &m.EntityIDs)
		if err := decodeMemoryMetadata(&m, metadata); err != nil {
			return nil, err
		}
		out = append(out, &ports.MemoryCandidate{Memory: &m, SourceTurn: sourceTurn})
	}
	return out, rows.Err()
}

func (s *Store) GetEvents(nodeID string) ([]*domain.DomainEvent, error) {
	return eventsQ(s.rdb(), nodeID)
}

func eventsQ(q queryer, nodeID string) ([]*domain.DomainEvent, error) {
	rows, err := q.Query(`SELECT event_id, node_id, event_index, type, payload_json, ruleset_version FROM domain_events WHERE node_id=? ORDER BY event_index`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.DomainEvent
	for rows.Next() {
		var ev domain.DomainEvent
		var typ string
		var nodeIDVal string
		if err := rows.Scan(&ev.EventID, &nodeIDVal, &ev.EventIndex, &typ, &ev.PayloadJSON, &ev.RulesetVersion); err != nil {
			return nil, err
		}
		ev.NodeID = nodeIDVal
		ev.Type = domain.EventType(typ)
		out = append(out, &ev)
	}
	return out, rows.Err()
}
