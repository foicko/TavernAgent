package sqlite

import (
	"database/sql"
	"encoding/json"
	"strings"

	"tavernagent/internal/domain"
)

// ---- 记忆记录（M3）----

const memoryCols = `memory_id, source_node_id, kind, owner_ids, content, entity_ids, confidence, importance, pinned, supersedes, hidden, metadata_json, subject_key, created_turn, valid_from_turn, valid_until_turn`

// insertMemoryTx 写入记忆记录并同步词法索引（同一事务，避免索引与记录分叉）。
//
// terms 是实体 ID → 全部称呼（正式名 + 别名），用于索引
// （记忆正文常不写名字，见 memoryIndexTokens）。
func (s *Store) insertMemoryTx(ex execer, m *domain.MemoryRecord, terms map[string][]string) error {
	owners, err := json.Marshal(nonNilStrings(m.OwnerIDs))
	if err != nil {
		return err
	}
	entities, err := json.Marshal(nonNilStrings(m.EntityIDs))
	if err != nil {
		return err
	}
	importance := m.Importance
	if importance <= 0 {
		importance = domain.DefaultMemoryImportance
	}
	_, err = ex.Exec(`INSERT INTO memory_records(`+memoryCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.MemoryID, m.SourceNodeID, string(m.Kind), string(owners), m.Content,
		string(entities), m.Confidence, importance, boolToInt(m.Pinned), m.Supersedes, boolToInt(m.Hidden), memoryMetadataJSON(m),
		m.SubjectKey, m.CreatedTurn, m.ValidFromTurn, m.ValidUntilTurn)
	if err != nil {
		return err
	}
	if !s.fts {
		return nil
	}
	return s.indexMemoryTx(ex, m, terms)
}

// entityTermsAt 从某节点的状态投影解析实体称呼（索引侧需要）。
// 读取失败时返回 nil：索引少几个称呼只会降低召回，不会造成错误结果。
func (s *Store) entityTermsAt(nodeID string) map[string][]string {
	snap, err := s.StateAt(nodeID)
	if err != nil || snap == nil {
		return nil
	}
	return entityTermsMap(snap.StateJSON)
}

// turnTextOf 从节点内容里取出可检索的正文（玩家输入 + 模型正文块）。
func turnTextOf(contentJSON string) string {
	var c struct {
		InputText string `json:"inputText"`
		Blocks    []struct {
			Text string `json:"text"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal([]byte(contentJSON), &c); err != nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(c.InputText)
	for _, b := range c.Blocks {
		sb.WriteString(" ")
		sb.WriteString(b.Text)
	}
	return sb.String()
}

func scanMemory(row interface{ Scan(...any) error }) (*domain.MemoryRecord, error) {
	var m domain.MemoryRecord
	var kind, owners, entities, metadata string
	var pinned, hidden int
	if err := row.Scan(&m.MemoryID, &m.SourceNodeID, &kind, &owners, &m.Content,
		&entities, &m.Confidence, &m.Importance, &pinned, &m.Supersedes, &hidden, &metadata,
		&m.SubjectKey, &m.CreatedTurn, &m.ValidFromTurn, &m.ValidUntilTurn); err != nil {
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
	return &m, nil
}

// ListMemories 返回会话内全部记忆（含隐藏与被修订项，供管理视图）。
func (s *Store) ListMemories(sessionID string) ([]*domain.MemoryRecord, error) {
	rows, err := s.rdb().Query(`SELECT `+prefixed(memoryCols, "m")+`
		FROM memory_records m JOIN plot_nodes n ON n.node_id = m.source_node_id
		WHERE n.session_id = ? ORDER BY m.rowid`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.MemoryRecord
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MemoriesInChain 返回挂在给定节点集合上的记忆（分支隔离：调用方传当前节点的祖先链）。
func (s *Store) MemoriesInChain(nodeIDs []string) ([]*domain.MemoryRecord, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	ph := make([]string, len(nodeIDs))
	args := make([]any, len(nodeIDs))
	for i, id := range nodeIDs {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := s.rdb().Query(`SELECT `+memoryCols+` FROM memory_records
		WHERE source_node_id IN (`+strings.Join(ph, ",")+`) ORDER BY rowid`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.MemoryRecord
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CreateMemory 写入单条记忆记录（用户发起的覆盖记录：纠正/置顶/隐藏）。
func (s *Store) CreateMemory(m *domain.MemoryRecord) error {
	// 称呼解析先于加锁：StateAt 走同一条连接，持锁期间读库会自等。
	terms := s.entityTermsAt(m.SourceNodeID)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projectionBuildMu.Lock()
	defer s.projectionBuildMu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.insertMemoryTx(tx, m, terms); err != nil {
		return err
	}
	return s.commitMemoryMutation(tx)
}

// UpdateMemory 更新既有记忆（纠正内容 / 置顶 / 隐藏 / 标记被修订）。
func (s *Store) UpdateMemory(m *domain.MemoryRecord) error {
	owners, err := json.Marshal(nonNilStrings(m.OwnerIDs))
	if err != nil {
		return err
	}
	entities, err := json.Marshal(nonNilStrings(m.EntityIDs))
	if err != nil {
		return err
	}
	terms := s.entityTermsAt(m.SourceNodeID)
	importance := m.Importance
	if importance <= 0 {
		importance = domain.DefaultMemoryImportance
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projectionBuildMu.Lock()
	defer s.projectionBuildMu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE memory_records SET kind=?, owner_ids=?, content=?, entity_ids=?,
		confidence=?, importance=?, pinned=?, supersedes=?, hidden=?, metadata_json=?,
		subject_key=?, created_turn=?, valid_from_turn=?, valid_until_turn=? WHERE memory_id=?`,
		string(m.Kind), string(owners), m.Content, string(entities),
		m.Confidence, importance, boolToInt(m.Pinned), m.Supersedes, boolToInt(m.Hidden), memoryMetadataJSON(m),
		m.SubjectKey, m.CreatedTurn, m.ValidFromTurn, m.ValidUntilTurn, m.MemoryID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	// 内容可能已变，索引必须跟着变：先删后插（同事务，不留旧词元）。
	if s.fts {
		if err := s.unindexMemoryTx(tx, m.MemoryID); err != nil {
			return err
		}
		if err := s.indexMemoryTx(tx, m, terms); err != nil {
			return err
		}
	}
	return s.commitMemoryMutation(tx)
}

// prefixed 给逗号分隔的列名统一加表前缀，便于 JOIN 查询。
func prefixed(cols, prefix string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = prefix + "." + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
