package sqlite

import (
	"encoding/json"
	"strings"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/search"
)

// memoryFTSTable 是记忆词法索引的虚拟表名。
//
// 注意：FTS5 的 MATCH 左操作数**必须是表名本身，不能用别名**
// （`FROM memory_fts f WHERE f MATCH ?` 会报 no such column: f）。
// 因此本文件里的查询一律不给这张表起别名。
const memoryFTSTable = "memory_fts"

// memoryFTSTriTable 是双字/三字回退索引表（tokenize='trigram'）。
const memoryFTSTriTable = "memory_fts_tri"

// recallCandidateLimit 是词法命中的候选池上限。
//
// 池只是「进入打分公式的集合」，最终注入条数由 context 的预算决定（默认 5）。
// 200 倍余量是给实体重合、重要度与时序三项留出选择空间，同时避免把整条
// 路径的记忆（万级）搬回内存——那正是 M4 要消掉的代价。
const recallCandidateLimit = 200

// mentionMinBigramOverlap 是判定「正文确实提到该记忆」的重合门槛。
// 与召回相关性门槛同源（context 的 defaultMinMemoryBigramOverlap）。
const mentionMinBigramOverlap = 2

// execQueryer 同时具备写入与查询能力（*sql.DB 与 *sql.Tx 都满足）。
// 提及判定要在提交事务内回读记忆内容，因此需要两者的并集。
type execQueryer interface {
	execer
	queryer
}

// ---- 索引维护 ----

// ensureMemoryIndex 创建词法索引并回填缺失内容。
//
// 索引表在**迁移之外**创建：FTS5 是可选能力（M0 风险项，见 SystemInfo 探测）。
// 若把它写进迁移 SQL，一次创建失败会让整个库打不开——而它只是加速结构，
// 不该有这种权限。失败时只降级（fts=false），检索退回 Go 侧双字重合。
func (s *Store) ensureMemoryIndex() error {
	var definition string
	_ = s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE name='memory_fts'`).Scan(&definition)
	if definition != "" && !strings.Contains(definition, "porter") {
		if _, err := s.db.Exec(`DROP TABLE memory_fts`); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS ` + memoryFTSTable +
		` USING fts5(memory_id UNINDEXED, tokens, tokenize='porter unicode61 remove_diacritics 2')`); err != nil {
		s.fts = false
		return nil
	}
	s.fts = true
	_, err := s.db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts_tri USING fts5(memory_id UNINDEXED, text, tokenize='trigram')`)
	s.trigram = err == nil
	return s.backfillMemoryIndex()
}

// LexicalIndexAvailable 报告词法索引是否可用。
func (s *Store) LexicalIndexAvailable() bool { return s.fts }

// backfillMemoryIndex 在必要时全量重建索引。
//
// 两种需要重建的情况：
//   - 索引为空而记忆非空（首次启用或迁移后）；
//   - 索引行与记忆行的 rowid 没对齐（旧布局）。对齐是查询形态的硬要求，
//     见 indexMemoryTx 的说明——错位不会报错，只会让召回悄悄变得很慢。
//
// 用「整体计数」而不是「逐条 NOT EXISTS」判断：memory_id 是 UNINDEXED 列，
// 逐条比对会让 FTS5 表被整表扫描 N 次（万级记忆就是上亿次比较）。
func (s *Store) backfillMemoryIndex() error {
	tables := []string{memoryFTSTable}
	if s.trigram {
		tables = append(tables, "memory_fts_tri")
	}
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM memory_records`).Scan(&total); err != nil {
		return err
	}
	for _, table := range tables {
		var indexed, aligned int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&indexed); err != nil {
			return err
		}
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table + ` f JOIN memory_records m ON m.rowid=f.rowid AND m.memory_id=f.memory_id`).Scan(&aligned); err != nil {
			return err
		}
		if indexed != total || aligned != total {
			return s.RebuildMemoryIndex()
		}
	}
	return nil
}

// RebuildMemoryIndex 全量重建词法索引（迁移回填、评测与运维入口）。
func (s *Store) RebuildMemoryIndex() error {
	if !s.fts {
		return nil
	}
	// 显示名解析必须在写事务之前完成：连接池只有一条连接（MaxOpenConns(1)），
	// 在事务里再调 StateAt 会等自己持有的连接而死锁。
	terms, err := s.globalEntityTerms()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM ` + memoryFTSTable); err != nil {
		return err
	}
	if s.trigram {
		if _, err := tx.Exec(`DELETE FROM memory_fts_tri`); err != nil {
			return err
		}
	}
	rows, err := tx.Query(`SELECT m.rowid, n.session_id, ` + prefixed(memoryCols, "m") + ` FROM memory_records m JOIN plot_nodes n ON n.node_id=m.source_node_id ORDER BY m.rowid`)
	if err != nil {
		return err
	}
	type row struct {
		rowid     int64
		sessionID string
		rec       *domain.MemoryRecord
	}
	var memories []row
	for rows.Next() {
		var rid int64
		var sessionID string
		var m domain.MemoryRecord
		var kind, owners, entities, metadata string
		var pinned, hidden int
		if serr := rows.Scan(&rid, &sessionID, &m.MemoryID, &m.SourceNodeID, &kind, &owners, &m.Content,
			&entities, &m.Confidence, &m.Importance, &pinned, &m.Supersedes, &hidden, &metadata,
			&m.SubjectKey, &m.CreatedTurn, &m.ValidFromTurn, &m.ValidUntilTurn); serr != nil {
			rows.Close()
			return serr
		}
		m.Kind = domain.MemoryKind(kind)
		m.Pinned = pinned != 0
		m.Hidden = hidden != 0
		decodeIDList(owners, "ownerIds", &m.OwnerIDs)
		decodeIDList(entities, "entityIds", &m.EntityIDs)
		if err := decodeMemoryMetadata(&m, metadata); err != nil {
			rows.Close()
			return err
		}
		memories = append(memories, row{rowid: rid, sessionID: sessionID, rec: &m})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range memories {
		if err := s.indexMemoryTx(tx, r.rec, terms[r.sessionID]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// globalEntityNames 汇总各会话头节点状态里的 id → 显示名。
//
// 角色 ID 全局唯一，因此可以合成一张全局表。这是**迁移期的一次性近似**：
// 用会话当前状态去解析历史记忆的实体名。显示名在会话内基本不变，
// 所以对回填够用；新写入的记忆一律用产生它的那次提交的状态解析。
func (s *Store) globalEntityTerms() (map[string]map[string][]string, error) {
	rows, err := s.db.Query(`SELECT session_id, root_node_id FROM sessions`)
	if err != nil {
		return nil, err
	}
	type source struct{ session, head string }
	var heads []source
	for rows.Next() {
		var h source
		if err := rows.Scan(&h.session, &h.head); err != nil {
			rows.Close()
			return nil, err
		}
		heads = append(heads, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	terms := map[string]map[string][]string{}
	for _, h := range heads {
		snap, serr := s.StateAt(h.head)
		if serr != nil || snap == nil {
			continue
		}
		terms[h.session] = entityTermsMap(snap.StateJSON)
	}
	return terms, nil
}

// entityTermsMap 从状态投影里取出 id → 该实体的全部称呼（正式名 + 别名）。
//
// 只解析 characters 子树：状态投影可能很大，索引只需要这一小块。
// 称呼规则来自 domain.CharacterInfo.SearchTerms，索引侧与查询侧同源。
func entityTermsMap(stateJSON string) map[string][]string {
	if stateJSON == "" {
		return nil
	}
	var st struct {
		Characters map[string]domain.CharacterInfo `json:"characters"`
		Items      map[string]domain.ItemInstance  `json:"items"`
	}
	if err := json.Unmarshal([]byte(stateJSON), &st); err != nil {
		return nil
	}
	out := make(map[string][]string, len(st.Characters))
	for id, c := range st.Characters {
		if terms := c.SearchTerms(); len(terms) > 0 {
			out[id] = terms
		}
	}
	for id, it := range st.Items {
		out[id] = append([]string{it.Name}, it.Aliases...)
	}
	return out
}

// memoryIndexTokens 计算一条记忆的检索词元串。
//
// 组成：正文 + 实体/归属 ID + **实体显示名**。
// 显示名是必需的：记忆正文经常不写名字（"她习惯在清晨磨刀"），只索引正文时
// 按角色名的查询会把这类记忆整条漏掉，而它们恰恰是最需要被召回的关系性认知。
func memoryIndexTokens(m *domain.MemoryRecord, terms map[string][]string) string {
	var sb strings.Builder
	sb.WriteString(strings.Join(search.Tokenize(m.Content), " "))
	ids := make([]string, 0, len(m.EntityIDs)+len(m.OwnerIDs))
	ids = append(ids, m.EntityIDs...)
	ids = append(ids, m.OwnerIDs...)
	for _, id := range ids {
		if id == "" {
			continue
		}
		sb.WriteString(" ")
		sb.WriteString(strings.Join(search.Tokenize(id), " "))
		for _, t := range terms[id] {
			sb.WriteString(" ")
			sb.WriteString(strings.Join(search.Tokenize(t), " "))
		}
	}
	return strings.TrimSpace(sb.String())
}

// indexMemoryTx 把一条记忆写入词法索引（与记忆记录同事务）。
//
// 索引行的 rowid **必须等于 memory_records 的 rowid**。原因不是省空间，
// 而是查询形态：召回要「先按 BM25 取前 N，再回表验可见性」，这要求两个方向
// 的连接都走 rowid。memory_id 在 FTS 里是 UNINDEXED 列——用它在两表之间连接
// 会让 FTS 表被整表扫描，实测把这段从 35ms 推到 120ms 以上。
//
// 调用方必须紧接在 memory_records 插入之后调用本函数：这里用
// last_insert_rowid() 取刚才那条记忆的行号。
func (s *Store) indexMemoryTx(ex execer, m *domain.MemoryRecord, terms map[string][]string) error {
	tokens := memoryIndexTokens(m, terms)
	_, err := ex.Exec(`INSERT INTO `+memoryFTSTable+`(rowid, memory_id, tokens)
		VALUES((SELECT rowid FROM memory_records WHERE memory_id = ?), ?, ?)`,
		m.MemoryID, m.MemoryID, tokens)
	if err != nil || !s.trigram {
		return err
	}
	var raw strings.Builder
	raw.WriteString(m.Content)
	for _, eid := range append(append([]string{}, m.EntityIDs...), m.OwnerIDs...) {
		raw.WriteString("\n" + eid)
		for _, term := range terms[eid] {
			raw.WriteString("\n" + term)
		}
	}
	_, err = ex.Exec(`INSERT INTO memory_fts_tri(rowid,memory_id,text) VALUES((SELECT rowid FROM memory_records WHERE memory_id=?),?,?)`, m.MemoryID, m.MemoryID, raw.String())
	return err
}

// unindexMemoryTx 移除一条记忆的词法索引（更新时先删后插）。
func (s *Store) unindexMemoryTx(ex execer, memoryID string) error {
	_, err := ex.Exec(`DELETE FROM `+memoryFTSTable+
		` WHERE rowid = (SELECT rowid FROM memory_records WHERE memory_id = ?)`, memoryID)
	if err != nil || !s.trigram {
		return err
	}
	_, err = ex.Exec(`DELETE FROM memory_fts_tri WHERE rowid=(SELECT rowid FROM memory_records WHERE memory_id=?)`, memoryID)
	return err
}

// ---- 可见候选池 ----

// recallCTE 是从当前节点向上收集祖先集合。
//
// 记忆可见性 = 它的来源节点在这个集合里（契约 §8，T16/T17）。
// 这步是 O(链深) 的递归遍历，无法用索引规避；实测万级链约 13ms，
// 因此它保留在查询里，但**不能让下游为它多物化一万行**。
const recallCTE = `
WITH RECURSIVE up(node_key) AS (
  SELECT node_id FROM plot_nodes WHERE node_id = ?
  UNION ALL
  SELECT p.parent_id FROM plot_nodes p JOIN up u ON p.node_id = u.node_key
   WHERE p.parent_id IS NOT NULL
),
mention AS (
  SELECT mm.memory_id AS memory_id, MAX(mm.turn_number) AS last_turn
    FROM memory_mentions mm
    JOIN up mu ON mu.node_key = mm.node_id
   GROUP BY mm.memory_id
)
`

// RecallMemories 返回当前路径上的可见候选池（技术契约 §8.1）。
//
// 池 = {词法命中前 N} ∪ {三元命中前 N} ∪ {短文本匹配} ∪ {置顶} ∪ {重要性保底}，全部先经过路径可见性过滤。
// 隐藏记录（hidden=1）不入候选池，隐藏记忆由管理探针（probe.Hidden=false 路径）单独处理（T2.3）；
// 被覆盖的旧记录（superseded）从可见集中排除；
// 重要性保底路按 importance DESC, rowid DESC 截取，保证语义改写或无直接字面重合的记忆也能进入候选池（T2.2）。
//
// 查询形态是**先取词法前 N 再回表**，而不是「先物化全部可见记忆再排名」：
// 后者要为一万条记忆建宽行（含 content 与两个 JSON 列），实测 56ms，
// 加上后续连接与排序整段超过 120ms；前者只把 200 行带回表，实测 35ms。
// 这个形态依赖索引行与记忆行的 rowid 对齐（见 indexMemoryTx）。
//
// 另一半契约要求不能让步：**先在可见集合内排名**，而不是先全局排名再过滤——
// 否则别的分支的高分记忆会挤掉本分支的名额。
func (s *Store) RecallMemories(nodeID string, q ports.MemoryRecallQuery) ([]*ports.MemoryCandidate, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = recallCandidateLimit
	}
	if limit > 1000 {
		limit = 1000
	}
	// Resolve immutable revisions once through the checkpointed projection.
	// Ranking every historical revision made compile cost grow with edit count.
	memories, err := s.ProjectedMemories(nodeID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(memories))
	for _, memory := range memories {
		if !memory.Hidden {
			ids = append(ids, memory.MemoryID)
		}
	}
	if len(ids) == 0 {
		return []*ports.MemoryCandidate{}, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	args := []any{nodeID, string(encoded)}
	query := recallCTE + `, visible AS MATERIALIZED (
 SELECT m.rowid AS rid FROM json_each(?) id JOIN memory_records m ON m.memory_id=id.value WHERE m.hidden=0
`
	if q.OwnerIDs != nil {
		owners, _ := json.Marshal(q.OwnerIDs)
		query += ` AND (json_array_length(m.owner_ids)=0 OR EXISTS(SELECT 1 FROM json_each(m.owner_ids) o WHERE o.value IN (SELECT value FROM json_each(?))))`
		args = append(args, string(owners))
	}
	secrets, _ := json.Marshal(nonNilStrings(q.SecretIDs))
	query += ` AND (m.kind<>'secret' AND COALESCE(json_extract(m.metadata_json,'$.secretId'),'')='' OR json_extract(m.metadata_json,'$.secretId') IN (SELECT value FROM json_each(?))))`
	args = append(args, string(secrets))
	channels := []string{}
	addChannel := func(name, table, expr string) {
		if expr == "" {
			return
		}
		// Drive from MATCH once. A rowid IN list makes FTS5 restart its
		// MATCH cursor for every visible memory (quadratic on broad queries).
		// CROSS JOIN fixes this order while retaining visibility before LIMIT.
		query += `, ` + name + ` AS MATERIALIZED (
 SELECT ` + table + `.rowid AS rid,bm25(` + table + `) AS rnk FROM ` + table + `
 CROSS JOIN visible v ON v.rid=` + table + `.rowid
 WHERE ` + table + ` MATCH ?
 ORDER BY bm25(` + table + `),` + table + `.rowid LIMIT ?)`
		args = append(args, expr, limit)
		channels = append(channels, `SELECT rid, -1.0/(60+ROW_NUMBER() OVER(ORDER BY rnk,rid)) AS rnk FROM `+name)
	}
	if s.fts {
		addChannel("lexical", "memory_fts", q.MatchExpr)
	}
	triExpr := search.TrigramMatchExpr(q.RawText)
	if s.trigram {
		addChannel("trigrams", "memory_fts_tri", triExpr)
	}
	if strings.TrimSpace(q.RawText) != "" && q.MatchExpr == "" && triExpr == "" {
		query += `, short_hits AS MATERIALIZED (SELECT m.rowid AS rid FROM memory_records m
 WHERE m.rowid IN(SELECT rid FROM visible) AND instr(lower(m.content),lower(?))>0 ORDER BY m.memory_id LIMIT ?)`
		args = append(args, strings.TrimSpace(q.RawText), limit)
		channels = append(channels, `SELECT rid,-1.0/61 AS rnk FROM short_hits`)
	}
	channels = append(channels, `SELECT m.rowid AS rid,NULL AS rnk FROM memory_records m
 WHERE m.rowid IN(SELECT rid FROM visible) AND m.pinned=1`)
	if len(q.EntityIDs) > 0 {
		entities, _ := json.Marshal(q.EntityIDs)
		query += `, entity_hits AS MATERIALIZED (SELECT m.rowid AS rid,m.created_turn FROM memory_records m
 WHERE m.rowid IN(SELECT rid FROM visible) AND EXISTS(SELECT 1 FROM json_each(m.entity_ids) e WHERE e.value IN(SELECT value FROM json_each(?)))
 ORDER BY m.created_turn DESC,m.memory_id LIMIT ?)`
		args = append(args, string(entities), limit)
		channels = append(channels, `SELECT rid,-0.25/(60+ROW_NUMBER() OVER(ORDER BY created_turn DESC,rid)) AS rnk FROM entity_hits`)
	}

	// 第五路：重要性保底路（T2.2）。按 importance DESC, rowid DESC 保留 Top K，
	// 保证语义改写或无直接字面重合的记忆也能以候选进入 Go 侧打分。
	if q.FallbackLimit > 0 {
		query += `, fallback AS MATERIALIZED (SELECT m.rowid AS rid FROM memory_records m
 WHERE m.rowid IN(SELECT rid FROM visible) ORDER BY m.importance DESC, m.rowid DESC LIMIT ?)`
		args = append(args, q.FallbackLimit)
		channels = append(channels, `SELECT rid,NULL AS rnk FROM fallback`)
	}

	query += `, matches AS (` + strings.Join(channels, " UNION ALL ") + `), pool AS (
 SELECT rid,SUM(rnk) AS rnk FROM matches GROUP BY rid
)
SELECT ` + prefixed(memoryCols, "m") + `, n.turn_number, p.rnk, mt.last_turn
  FROM pool p
  JOIN memory_records m ON m.rowid = p.rid
  JOIN plot_nodes n ON n.node_id = m.source_node_id
  LEFT JOIN mention mt ON mt.memory_id = m.memory_id
 ORDER BY (p.rnk IS NULL), p.rnk, m.memory_id`

	rows, err := s.rdb().Query(query, args...)
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
		var rank *float64
		var lastTurn *int
		if err := rows.Scan(
			&m.MemoryID, &m.SourceNodeID, &kind, &owners, &m.Content, &entities,
			&m.Confidence, &m.Importance, &pinned, &m.Supersedes, &hidden, &metadata,
			&m.SubjectKey, &m.CreatedTurn, &m.ValidFromTurn, &m.ValidUntilTurn,
			&sourceTurn, &rank, &lastTurn,
		); err != nil {
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
		c := &ports.MemoryCandidate{Memory: &m, SourceTurn: sourceTurn}
		if rank != nil {
			c.Rank = *rank
			c.HasRank = true
		}
		if lastTurn != nil {
			c.LastMentionTurn = *lastTurn
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- 提及记录 ----

// insertMentionsTx 在提交事务内记录「本轮正文确实提到」的记忆。
//
// 只有被注入过（candidates）**且**提交正文里确有提及依据的才落库。
// 注入本身不算：否则一次召回就把 recency 永久抬高，而正文可能只字未提
// （契约 §8.1 明确要求召回动作本身不更新）。
func (s *Store) insertMentionsTx(ex execQueryer, node *domain.PlotNode, candidates []string, turnText, stateJSON string) error {
	if len(candidates) == 0 || strings.TrimSpace(turnText) == "" {
		return nil
	}
	ph := make([]string, len(candidates))
	args := make([]any, 0, len(candidates))
	for i, id := range candidates {
		ph[i] = "?"
		args = append(args, id)
	}
	rows, err := ex.Query(`SELECT `+memoryCols+` FROM memory_records WHERE memory_id IN (`+strings.Join(ph, ",")+`)`, args...)
	if err != nil {
		return err
	}
	var memories []*domain.MemoryRecord
	for rows.Next() {
		m, serr := scanMemory(rows)
		if serr != nil {
			rows.Close()
			return serr
		}
		memories = append(memories, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	terms := entityTermsMap(stateJSON)
	for _, m := range memories {
		if !mentionsMemory(turnText, m, terms) {
			continue
		}
		if _, err := ex.Exec(`INSERT OR IGNORE INTO memory_mentions(memory_id, node_id, turn_number) VALUES(?,?,?)`,
			m.MemoryID, node.NodeID, node.TurnNumber); err != nil {
			return err
		}
	}
	return nil
}

// mentionsMemory 判断提交的交互是否确实提到这条记忆。
//
// 依据（满足其一即可）：实体显示名出现在正文里，或正文与记忆内容有
// mentionMinBigramOverlap 个相邻双字重合。
func mentionsMemory(turnText string, m *domain.MemoryRecord, terms map[string][]string) bool {
	low := strings.ToLower(turnText)
	for _, id := range m.EntityIDs {
		for _, t := range terms[id] {
			if strings.Contains(low, strings.ToLower(t)) {
				return true
			}
		}
	}
	turn := map[string]bool{}
	for _, t := range search.Tokenize(turnText) {
		turn[t] = true
	}
	seen := map[string]bool{}
	overlap := 0
	for _, t := range search.Tokenize(m.Content) {
		if seen[t] || !turn[t] {
			continue
		}
		seen[t] = true
		overlap++
		if overlap >= mentionMinBigramOverlap {
			return true
		}
	}
	return false
}
