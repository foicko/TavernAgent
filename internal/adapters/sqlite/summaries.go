package sqlite

import (
	"database/sql"

	"tavernagent/internal/domain"
)

// 本文件是历史区间摘要（M4b）的存取。
//
// 摘要只是**派生材料**：删了可以重建，丢了不影响正确性。它之所以还要落库，
// 是因为生成一次要花模型调用——存下来才能在多个分支间复用相同不可变
// 祖先区间的摘要（契约 §9.2），而不是每个分支各生成一遍。

const summaryCols = `summary_id, from_node_id, to_node_id, source_hash, visibility_scope, text, model_config_version, summary_version`

// SaveSummary 写入一条摘要产物。
//
// 校验在写入侧完成：来源节点必须存在且同会话、区间方向正确（from 在 to 之上）、
// 文本非空。摘要来源是节点区间，节点不可变，因此 source_hash 一旦写入就恒定——
// 这正是"同一不可变祖先区间的摘要可在多个分支复用"的前提。
func (s *Store) SaveSummary(a *domain.SummaryArtifact) error {
	if a == nil {
		return errSummaryNil
	}
	if a.SummaryID == "" || a.FromNodeID == "" || a.ToNodeID == "" || a.Text == "" {
		return errSummaryIncomplete
	}
	if a.SummaryVersion <= 0 {
		a.SummaryVersion = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var fromDepth, toDepth int
	var fromSession, toSession string
	if err := tx.QueryRow(`SELECT session_id, depth FROM plot_nodes WHERE node_id=?`, a.FromNodeID).
		Scan(&fromSession, &fromDepth); err != nil {
		return errSummarySourceMissing
	}
	if err := tx.QueryRow(`SELECT session_id, depth FROM plot_nodes WHERE node_id=?`, a.ToNodeID).
		Scan(&toSession, &toDepth); err != nil {
		return errSummarySourceMissing
	}
	if fromSession != toSession {
		return errSummaryCrossSession
	}
	// 区间方向：from 必须不深于 to（摘要是"从早到晚"的一段）。
	if fromDepth > toDepth {
		return errSummaryWrongDirection
	}
	if ok, err := isAncestorQ(tx, a.FromNodeID, a.ToNodeID); err != nil {
		return err
	} else if !ok {
		return errSummaryWrongDirection
	}
	chain, err := ancestorChainQ(tx, a.ToNodeID, true)
	if err != nil {
		return err
	}
	var sources []*domain.PlotNode
	for _, n := range chain {
		if n.Depth >= fromDepth {
			sources = append(sources, n)
		}
	}
	actual := domain.SummarySourceHash(sources)
	if a.SourceHash != "" && a.SourceHash != actual {
		return errSummary("摘要来源指纹与真实区间不一致")
	}
	a.SourceHash = actual
	if _, err := tx.Exec(`INSERT INTO summary_artifacts(`+summaryCols+`) VALUES(?,?,?,?,?,?,?,?)`,
		a.SummaryID, a.FromNodeID, a.ToNodeID, a.SourceHash, a.VisibilityScope, a.Text,
		a.ModelConfigVersion, a.SummaryVersion); err != nil {
		return err
	}
	return tx.Commit()
}

// SummariesOnPath 返回来源区间完全落在当前路径上的摘要，每个区间取版本最高的一条。
//
// 采用条件（技术契约 §9.2）在此下推到 SQL：
//   - 来源区间仍在请求所用的祖先路径中：from 与 to 都必须是当前节点的祖先。
//     跨分支的摘要其 to_node 不在路径上，自然被排除——T24（老分支摘要晚到、
//     当前已切换分支 → 不应用到不相容路径）。
//   - 区间方向在写入侧已校验，这里不再重复。
//
// 可见范围（visibility_scope）非空的条目当前不采用：作用域语义（按分支/按
// 视角）还没有定义，先不猜——只采用空作用域（对所有分支有效）的摘要。
func (s *Store) SummariesOnPath(nodeID string) ([]*domain.SummaryArtifact, error) {
	rows, err := s.rdb().Query(`
WITH RECURSIVE up(node_key) AS (
  SELECT node_id FROM plot_nodes WHERE node_id = ?
  UNION ALL
  SELECT p.parent_id FROM plot_nodes p JOIN up u ON p.node_id = u.node_key
   WHERE p.parent_id IS NOT NULL
)
SELECT `+prefixed(summaryCols, "s")+` FROM summary_artifacts s
  JOIN up f ON f.node_key = s.from_node_id
  JOIN up t ON t.node_key = s.to_node_id
  JOIN plot_nodes endpoint ON endpoint.node_id=s.to_node_id
  JOIN plot_nodes startpoint ON startpoint.node_id=s.from_node_id
 WHERE s.visibility_scope = ''
 AND NOT EXISTS (
  SELECT 1 FROM memory_records revision
  JOIN up ru ON ru.node_key=revision.source_node_id
  JOIN memory_records original ON original.memory_id=revision.supersedes
  JOIN plot_nodes rn ON rn.node_id=revision.source_node_id
  JOIN plot_nodes origin ON origin.node_id=original.source_node_id
  WHERE rn.depth>endpoint.depth AND origin.depth BETWEEN startpoint.depth AND endpoint.depth
   AND (revision.hidden=1 OR revision.content<>original.content)
 )
 ORDER BY endpoint.depth,s.from_node_id,s.summary_version DESC,s.summary_id DESC
`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// 同一区间可能有多条（不同版本/模型配置）：按 (from,to) 分组取版本最高的，
	// 平局按 summary_id 取最大，保证同一输入得到同一结果。
	seen := map[string]bool{}
	out := []*domain.SummaryArtifact{}
	for rows.Next() {
		a, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		key := a.FromNodeID + "->" + a.ToNodeID
		if !seen[key] {
			out = append(out, a)
			seen[key] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanSummary(row interface{ Scan(...any) error }) (*domain.SummaryArtifact, error) {
	var a domain.SummaryArtifact
	var version sql.NullInt64
	if err := row.Scan(&a.SummaryID, &a.FromNodeID, &a.ToNodeID, &a.SourceHash,
		&a.VisibilityScope, &a.Text, &a.ModelConfigVersion, &version); err != nil {
		return nil, err
	}
	a.SummaryVersion = int(version.Int64)
	return &a, nil
}

// 摘要写入侧的错误。全部是显式错误而不是 panic/静默：
// 摘要来源错了却照常落库，会让"摘要说了什么"变成无法解释的东西。
var (
	errSummaryNil            = errSummary("摘要为空")
	errSummaryIncomplete     = errSummary("摘要缺少必要字段（id/区间/文本）")
	errSummarySourceMissing  = errSummary("摘要的来源节点不存在")
	errSummaryCrossSession   = errSummary("摘要的来源节点属于不同会话")
	errSummaryWrongDirection = errSummary("摘要区间方向错误（from 比 to 更深）")
)

func errSummary(msg string) error { return &summaryError{msg} }

type summaryError struct{ msg string }

func (e *summaryError) Error() string { return e.msg }
