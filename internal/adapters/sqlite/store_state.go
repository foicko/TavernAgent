package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// ---- 状态投影：检查点 + 事件重放 ----

// StateAt 返回指定节点的状态投影（技术契约 §7）：
// StateAt(N) = 祖先路径上最近的兼容快照 + 该快照之后到 N 的有序事件。
// 若 N 自身就有检查点，直接返回；否则重放，结果不落库（只读计算）。
func (s *Store) StateAt(nodeID string) (*domain.StateSnapshot, error) {
	snap, err := stateAtQ(s.rdb(), nodeID)
	if errors.Is(err, sql.ErrNoRows) {
		// 对外只暴露端口层语义，应用层不必依赖 database/sql。
		return nil, ports.ErrNoState
	}
	return snap, err
}

// stateAtQ 计算节点 N 的状态投影：最近检查点 + 该检查点之后的事件重放。
//
// 性能约束：提交路径（checkBaseHashTx）每次都要走这里，所以**不能读整条祖先链**。
// 早期实现在这里先取完整祖先链再从中找检查点，导致每次提交的代价随节点深度
// 线性增长——长链上整体退化成 O(n²)（1000 节点时单次提交约 190ms）。
// 现在先用"限深向上遍历"找最近检查点（深度上限 = 检查点间隔），命中即只重放
// 那条短路径；只有在数据没有规律检查点时（例如外部导入的稀疏数据）才回退到
// 不限深度的完整向上遍历，保证正确性不依赖"每个节点都接近检查点"这一假设。
func stateAtQ(q queryer, nodeID string) (*domain.StateSnapshot, error) {
	path, snap, err := checkpointPathQ(q, nodeID, checkpointInterval)
	if err != nil {
		return nil, err
	}
	if snap == nil {
		// 回退：不限深度地向上找（正确性兜底，不依赖检查点密度）。
		path, snap, err = checkpointPathQ(q, nodeID, 0)
		if err != nil {
			return nil, err
		}
	}
	if snap == nil {
		return nil, sql.ErrNoRows
	}
	state, err := domain.UnmarshalWorld(snap.StateJSON)
	if err != nil {
		return nil, fmt.Errorf("state replay base %s: %w", snap.NodeID, err)
	}
	if snap.StateHash != "" && !state.MatchesHash(snap.StateHash) {
		return nil, fmt.Errorf("state checkpoint %s fingerprint mismatch", snap.NodeID)
	}
	// Upgrade the returned fingerprint without mutating old immutable snapshots.
	snap.StateHash = state.HashID()
	if len(path) == 0 {
		return snap, nil
	}
	// path 是"检查点之后到目标节点"的节点，按深度正序。
	for _, n := range path {
		evs, err := eventsQ(q, n.NodeID)
		if err != nil {
			return nil, err
		}
		if err := domain.ReplayEvents(state, evs); err != nil {
			return nil, fmt.Errorf("state replay at node %s: %w", n.NodeID, err)
		}
	}
	out, err := state.Marshal()
	if err != nil {
		return nil, err
	}
	return &domain.StateSnapshot{
		NodeID:          nodeID,
		SnapshotVersion: snap.SnapshotVersion,
		RulesetVersion:  snap.RulesetVersion,
		StateJSON:       out,
		StateHash:       state.HashID(),
	}, nil
}

// checkpointPathQ 从 nodeID 向上找最近的有快照的祖先。
//
// maxDepth > 0 时把向上级数限制在该值（快路径）；为 0 表示不限深度（兜底路径）。
// 返回「检查点之后到 nodeID」的节点（正序，不含检查点本身）与该检查点。
// 目标节点自身就是检查点时返回空路径。找不到任何快照时返回 (nil, nil, nil)。
func checkpointPathQ(q queryer, nodeID string, maxDepth int) ([]*domain.PlotNode, *domain.StateSnapshot, error) {
	// Maintenance heads and roots already carry a checkpoint. Read it with one
	// indexed query; stateAtQ still validates its payload and fingerprint.
	var exact domain.StateSnapshot
	var exactRuleset sql.NullString
	err := q.QueryRow(`SELECT s.snapshot_version,s.ruleset_version,s.state_json,s.state_hash
		FROM state_snapshots s JOIN plot_nodes n ON n.node_id=s.node_id WHERE s.node_id=?`, nodeID).
		Scan(&exact.SnapshotVersion, &exactRuleset, &exact.StateJSON, &exact.StateHash)
	if err == nil {
		exact.NodeID, exact.RulesetVersion = nodeID, exactRuleset.String
		return nil, &exact, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}
	depthClause := ""
	args := []any{nodeID}
	if maxDepth > 0 {
		depthClause = " AND u.n < ?"
		args = append(args, maxDepth)
	}
	// 与 RecentTurnNodes 同源的问题：深度必须取自 JOIN 后的节点本身，
	// 否则"最近检查点"可能选成更早的那个（结果仍正确，但重放步数变多）。
	findQuery := `
WITH RECURSIVE up(node_key, n) AS (
  SELECT node_id, 0 FROM plot_nodes WHERE node_id = ?
  UNION ALL
  SELECT p.parent_id, u.n + 1 FROM plot_nodes p JOIN up u ON p.node_id = u.node_key
   WHERE p.parent_id IS NOT NULL
     AND NOT EXISTS (SELECT 1 FROM state_snapshots checkpoint WHERE checkpoint.node_id=p.node_id)` + depthClause + `
)
SELECT n.node_id FROM up u
  JOIN plot_nodes n ON n.node_id = u.node_key
  JOIN state_snapshots s ON s.node_id = u.node_key
 ORDER BY n.depth DESC LIMIT 1`

	var snapNode string
	if err := q.QueryRow(findQuery, args...).Scan(&snapNode); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	var snap domain.StateSnapshot
	var ruleset sql.NullString
	if err := q.QueryRow(`SELECT snapshot_version, ruleset_version, state_json, state_hash FROM state_snapshots WHERE node_id=?`, snapNode).
		Scan(&snap.SnapshotVersion, &ruleset, &snap.StateJSON, &snap.StateHash); err != nil {
		return nil, nil, err
	}
	snap.NodeID = snapNode
	snap.RulesetVersion = ruleset.String

	if snapNode == nodeID {
		return nil, &snap, nil
	}

	// 取检查点之后到目标节点的路径。递归在"当前节点等于检查点"时停止，
	// 因此行数 = 目标深度 − 检查点深度（正常不超过检查点间隔）。
	//
	// 用 JOIN 而不是 `node_id IN (SELECT ...)`：后者会让规划器退化成对
	// plot_nodes 的全表扫描，代价随库增长——在提交路径上就是每次提交都
	// 多扫一遍整张表。
	// 递归列名刻意避开 node_id/depth：它们与 nodeCols 重名会让 SELECT 报歧义。
	const replayQuery = `
WITH RECURSIVE down(node_key) AS (
  SELECT node_id FROM plot_nodes WHERE node_id = ?
  UNION ALL
  SELECT p.parent_id FROM plot_nodes p JOIN down d ON p.node_id = d.node_key
   WHERE p.parent_id IS NOT NULL AND p.node_id <> ?
)
SELECT ` + nodeCols + ` FROM plot_nodes p JOIN down d ON d.node_key = p.node_id
 WHERE p.node_id <> ?
 ORDER BY p.depth ASC`

	rows, err := q.Query(replayQuery, nodeID, snapNode, snapNode)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var path []*domain.PlotNode
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, nil, err
		}
		path = append(path, n)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return path, &snap, nil
}
