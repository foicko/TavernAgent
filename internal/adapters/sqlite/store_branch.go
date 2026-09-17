package sqlite

import (
	"database/sql"

	"tavernagent/internal/domain"
)

// ---- branches ----

func scanBranch(row interface{ Scan(...any) error }) (*domain.Branch, error) {
	var b domain.Branch
	var active sql.NullString
	if err := row.Scan(&b.BranchID, &b.SessionID, &b.Name, &b.HeadNodeID, &b.Version, &active); err != nil {
		return nil, mapErr(err)
	}
	b.ActiveTurnID = active.String
	return &b, nil
}

const branchCols = "branch_id, session_id, name, head_node_id, version, active_turn_id"

func (s *Store) ListBranches(sessionID string) ([]*domain.Branch, error) {
	rows, err := s.rdb().Query(`SELECT `+branchCols+` FROM branches WHERE session_id=? ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Branch
	for rows.Next() {
		b, err := scanBranch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) GetBranch(branchID string) (*domain.Branch, error) {
	return scanBranch(s.rdb().QueryRow(`SELECT `+branchCols+` FROM branches WHERE branch_id=?`, branchID))
}

func (s *Store) CreateBranch(b *domain.Branch) error {
	_, err := s.db.Exec(`INSERT INTO branches(branch_id, session_id, name, head_node_id, version, active_turn_id, created_at) VALUES(?,?,?,?,?,?,?)`,
		b.BranchID, b.SessionID, b.Name, b.HeadNodeID, b.Version, "", s.now())
	return err
}

// ---- snapshots ----

func (s *Store) GetSnapshot(nodeID string) (*domain.StateSnapshot, error) {
	row := s.rdb().QueryRow(`SELECT node_id, snapshot_version, ruleset_version, state_json, state_hash FROM state_snapshots WHERE node_id=?`, nodeID)
	var sn domain.StateSnapshot
	var ruleset sql.NullString
	if err := row.Scan(&sn.NodeID, &sn.SnapshotVersion, &ruleset, &sn.StateJSON, &sn.StateHash); err != nil {
		return nil, mapErr(err)
	}
	sn.RulesetVersion = ruleset.String
	return &sn, nil
}

func (s *Store) SaveSnapshot(sn *domain.StateSnapshot) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO state_snapshots(node_id, snapshot_version, ruleset_version, state_json, state_hash) VALUES(?,?,?,?,?)`,
		sn.NodeID, sn.SnapshotVersion, sn.RulesetVersion, sn.StateJSON, sn.StateHash)
	return err
}
