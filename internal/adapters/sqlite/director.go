package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/util/id"
)

const schemaV10 = `
CREATE TABLE director_drafts (
 branch_id TEXT PRIMARY KEY REFERENCES branches(branch_id), session_id TEXT NOT NULL REFERENCES sessions(session_id),
 version INTEGER NOT NULL, base_revision_id TEXT NOT NULL, plan_json TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE director_projections (node_id TEXT PRIMARY KEY REFERENCES plot_nodes(node_id), state_json TEXT NOT NULL);
CREATE INDEX idx_director_events ON domain_events(node_id) WHERE type='director_change';
CREATE TABLE director_commands (
 session_id TEXT NOT NULL, idempotency_key TEXT NOT NULL, branch_id TEXT NOT NULL,
 payload_hash TEXT NOT NULL, node_id TEXT NOT NULL REFERENCES plot_nodes(node_id), version INTEGER NOT NULL,
 PRIMARY KEY(session_id,idempotency_key)
);
CREATE TABLE director_requests (
 request_id TEXT PRIMARY KEY, session_id TEXT NOT NULL REFERENCES sessions(session_id), branch_id TEXT NOT NULL REFERENCES branches(branch_id),
 idempotency_key TEXT NOT NULL, payload_hash TEXT NOT NULL, base_node_id TEXT NOT NULL REFERENCES plot_nodes(node_id),
 draft_version INTEGER NOT NULL, draft_json TEXT NOT NULL, text TEXT NOT NULL, status TEXT NOT NULL,
 reply TEXT NOT NULL DEFAULT '', candidate_json TEXT NOT NULL DEFAULT '', draft_applied INTEGER NOT NULL DEFAULT 0,
 error TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, UNIQUE(session_id,idempotency_key)
);
CREATE INDEX idx_director_requests_branch ON director_requests(branch_id,created_at);
CREATE UNIQUE INDEX idx_director_request_active ON director_requests(branch_id) WHERE status='generating';
`

func (s *Store) DirectorAt(nodeID string) (*domain.DirectorState, error) {
	return directorAtQ(s.rdb(), nodeID)
}

func directorAtQ(q queryer, nodeID string) (*domain.DirectorState, error) {
	// Ordinary stories need no ancestry traversal. The partial event index keeps
	// this probe small even when a library contains many unrelated stories.
	var exists bool
	if err := q.QueryRow(`SELECT EXISTS(SELECT 1 FROM domain_events e JOIN plot_nodes n ON n.node_id=e.node_id WHERE e.type='director_change' AND n.session_id=(SELECT session_id FROM plot_nodes WHERE node_id=?))`, nodeID).Scan(&exists); err != nil || !exists {
		return nil, err
	}
	rows, err := q.Query(`WITH RECURSIVE path(node_id,parent_id,depth) AS (
 SELECT node_id,parent_id,depth FROM plot_nodes WHERE node_id=?
 UNION ALL SELECT n.node_id,n.parent_id,n.depth FROM plot_nodes n JOIN path p ON n.node_id=p.parent_id
 WHERE NOT EXISTS(SELECT 1 FROM director_projections c WHERE c.node_id=p.node_id)
) SELECT p.node_id,c.state_json FROM path p LEFT JOIN director_projections c ON c.node_id=p.node_id ORDER BY p.depth`, nodeID)
	if err != nil {
		return nil, err
	}
	type entry struct {
		node     string
		snapshot sql.NullString
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.node, &e.snapshot); err != nil {
			rows.Close()
			return nil, err
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var state *domain.DirectorState
	for _, e := range entries {
		if e.snapshot.Valid {
			if err := json.Unmarshal([]byte(e.snapshot.String), &state); err != nil {
				return nil, err
			}
			continue
		}
		events, err := eventsQ(q, e.node)
		if err != nil {
			return nil, err
		}
		for _, ev := range events {
			state, err = domain.ApplyDirectorEvent(state, ev)
			if err != nil {
				return nil, err
			}
		}
	}
	return state, nil
}

func cacheDirectorEventsTx(tx *sql.Tx, node *domain.PlotNode, events []*domain.DomainEvent) error {
	found := false
	for _, e := range events {
		found = found || e.Type == domain.EventDirectorChange
	}
	if !found {
		return nil
	}
	state, err := directorAtQ(tx, node.ParentID)
	if err != nil {
		return err
	}
	for _, e := range events {
		if e.Type != domain.EventDirectorChange {
			continue
		}
		var change domain.DirectorChange
		if err := json.Unmarshal([]byte(e.PayloadJSON), &change); err != nil {
			return err
		}
		if change.Action == "report" {
			if node.Kind != domain.NodeKindTurn || change.Report == nil {
				return fmt.Errorf("director reports require a story turn")
			}
			var tc domain.TurnContent
			if err := json.Unmarshal([]byte(node.ContentJSON), &tc); err != nil {
				return err
			}
			if err := domain.ValidateDirectorEvidence(*change.Report, tc.Blocks); err != nil {
				return err
			}
		} else if node.Kind != domain.NodeKindDirectorEvent {
			return fmt.Errorf("director commands require a director node")
		}
		state, err = domain.ApplyDirectorEvent(state, e)
		if err != nil {
			return err
		}
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT OR REPLACE INTO director_projections(node_id,state_json) VALUES(?,?)`, node.NodeID, string(raw))
	return err
}

func (s *Store) GetDirectorDraft(branchID string) (*domain.DirectorDraft, error) {
	return directorDraftQ(s.rdb(), branchID)
}
func directorDraftQ(q queryer, branchID string) (*domain.DirectorDraft, error) {
	d := &domain.DirectorDraft{BranchID: branchID}
	var raw, updated string
	err := q.QueryRow(`SELECT session_id,version,base_revision_id,plan_json,updated_at FROM director_drafts WHERE branch_id=?`, branchID).Scan(&d.SessionID, &d.Version, &d.BaseRevisionID, &raw, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), &d.Plan); err != nil {
		return nil, err
	}
	d.UpdatedAt = (&Store{}).parseTime(updated)
	return d, nil
}

func (s *Store) SaveDirectorDraft(d *domain.DirectorDraft, version int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var session string
	if err = tx.QueryRow(`SELECT session_id FROM branches WHERE branch_id=?`, d.BranchID).Scan(&session); err != nil {
		return mapErr(err)
	}
	if session != d.SessionID {
		return &ports.DirectorConflict{Code: "BRANCH_SESSION_MISMATCH"}
	}
	old, err := directorDraftQ(tx, d.BranchID)
	if err != nil {
		return err
	}
	current := int64(0)
	if old != nil {
		current = old.Version
	}
	if current != version {
		return &ports.DirectorConflict{Code: "DRAFT_CONFLICT"}
	}
	raw, err := json.Marshal(d.Plan)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO director_drafts(branch_id,session_id,version,base_revision_id,plan_json,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(branch_id) DO UPDATE SET version=excluded.version,base_revision_id=excluded.base_revision_id,plan_json=excluded.plan_json,updated_at=excluded.updated_at`, d.BranchID, d.SessionID, version+1, d.BaseRevisionID, string(raw), s.now())
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	d.Version, d.UpdatedAt = version+1, s.clock.Now()
	return nil
}

func directorCommandQ(q queryer, session, branch, key, hash string) (*ports.CommitResult, error) {
	var oldBranch, oldHash, node string
	var version int64
	err := q.QueryRow(`SELECT branch_id,payload_hash,node_id,version FROM director_commands WHERE session_id=? AND idempotency_key=?`, session, key).Scan(&oldBranch, &oldHash, &node, &version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if branch != oldBranch || hash != oldHash {
		return &ports.CommitResult{ConflictCode: "IDEMPOTENCY_CONFLICT"}, nil
	}
	return &ports.CommitResult{AlreadyDone: true, NewHeadID: node, NewVersion: version}, nil
}
func (s *Store) FindDirectorCommand(session, branch, key, hash string) (*ports.CommitResult, error) {
	return directorCommandQ(s.rdb(), session, branch, key, hash)
}

func (s *Store) directorOutboxTx(tx *sql.Tx, aggregate, kind string, payload any) error {
	seq, err := nextOutboxSeqTx(tx, aggregate)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO outbox_events(event_id,aggregate_id,sequence,type,payload_json,created_at) VALUES(?,?,?,?,?,?)`, id.New(), aggregate, seq, kind, string(raw), s.now())
	return err
}

func (s *Store) CommitDirector(ctx context.Context, c *ports.DirectorCommit) (*ports.CommitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if old, err := directorCommandQ(tx, c.SessionID, c.BranchID, c.IdempotencyKey, c.PayloadHash); err != nil || old != nil {
		return old, err
	}
	var session, head, active, ruleset string
	var version int64
	err = tx.QueryRow(`SELECT b.session_id,b.head_node_id,b.active_turn_id,b.version,s.ruleset_version FROM branches b JOIN sessions s ON s.session_id=b.session_id WHERE b.branch_id=?`, c.BranchID).Scan(&session, &head, &active, &version, &ruleset)
	if err != nil {
		return nil, mapErr(err)
	}
	conflict := ""
	if session != c.SessionID {
		conflict = "BRANCH_SESSION_MISMATCH"
	} else if head != c.ExpectedHeadID || version != c.ExpectedVersion {
		conflict = "HEAD_CONFLICT"
	} else if active != "" {
		conflict = "TURN_BUSY"
	}
	if conflict != "" {
		return &ports.CommitResult{ConflictCode: conflict}, nil
	}
	if c.DraftVersion != nil {
		d, err := directorDraftQ(tx, c.BranchID)
		if err != nil {
			return nil, err
		}
		if d == nil || d.Version != *c.DraftVersion {
			return &ports.CommitResult{ConflictCode: "DRAFT_CONFLICT"}, nil
		}
	}
	parent, err := scanNode(tx.QueryRow(`SELECT `+nodeCols+` FROM plot_nodes WHERE node_id=?`, head))
	if err != nil {
		return nil, err
	}
	c.Node.ParentID, c.Node.SessionID, c.Node.Kind = head, session, domain.NodeKindDirectorEvent
	c.Node.Depth, c.Node.TurnNumber, c.Node.SchemaVersion = parent.Depth+1, parent.TurnNumber, 1
	raw, err := json.Marshal(c.Change)
	if err != nil {
		return nil, err
	}
	ev := &domain.DomainEvent{EventID: id.New(), NodeID: c.Node.NodeID, Type: domain.EventDirectorChange, PayloadJSON: string(raw), RulesetVersion: ruleset}
	if err := insertNodeTx(tx, c.Node, s.now()); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`INSERT INTO domain_events(event_id,node_id,event_index,type,payload_json,ruleset_version) VALUES(?,?,0,?,?,?)`, ev.EventID, ev.NodeID, ev.Type, ev.PayloadJSON, ruleset); err != nil {
		return nil, err
	}
	if err := cacheDirectorEventsTx(tx, c.Node, []*domain.DomainEvent{ev}); err != nil {
		return nil, err
	}
	if c.Change.Action == "activate" {
		planJSON, err := json.Marshal(c.Change.Plan)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`UPDATE director_drafts SET version=version+1,base_revision_id=?,plan_json=?,updated_at=? WHERE branch_id=?`, c.Node.NodeID, string(planJSON), s.now(), c.BranchID); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(`UPDATE branches SET head_node_id=?,version=version+1 WHERE branch_id=?`, c.Node.NodeID, c.BranchID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE sessions SET updated_at=? WHERE session_id=?`, s.now(), session); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`INSERT INTO director_commands(session_id,idempotency_key,branch_id,payload_hash,node_id,version) VALUES(?,?,?,?,?,?)`, session, c.IdempotencyKey, c.BranchID, c.PayloadHash, c.Node.NodeID, version+1); err != nil {
		return nil, err
	}
	if err := s.directorOutboxTx(tx, session, "session.updated", map[string]any{"sessionId": session, "branchId": c.BranchID, "nodeId": c.Node.NodeID, "reason": "director"}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &ports.CommitResult{Committed: true, NewHeadID: c.Node.NodeID, NewVersion: version + 1}, nil
}

const directorRequestCols = `request_id,session_id,branch_id,idempotency_key,payload_hash,base_node_id,draft_version,draft_json,text,status,reply,candidate_json,draft_applied,error,created_at`

func scanDirectorRequest(row interface{ Scan(...any) error }) (*domain.DirectorRequest, error) {
	r := &domain.DirectorRequest{}
	var candidate, created string
	err := row.Scan(&r.RequestID, &r.SessionID, &r.BranchID, &r.IdempotencyKey, &r.PayloadHash, &r.BaseNodeID, &r.DraftVersion, &r.DraftJSON, &r.Text, &r.Status, &r.Reply, &candidate, &r.DraftApplied, &r.Error, &created)
	if err == sql.ErrNoRows {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, mapErr(err)
	}
	if candidate != "" {
		if err := json.Unmarshal([]byte(candidate), &r.Candidate); err != nil {
			return nil, err
		}
	}
	r.CreatedAt = (&Store{}).parseTime(created)
	return r, nil
}
func (s *Store) GetDirectorRequest(requestID string) (*domain.DirectorRequest, error) {
	return scanDirectorRequest(s.rdb().QueryRow(`SELECT `+directorRequestCols+` FROM director_requests WHERE request_id=?`, requestID))
}
func (s *Store) ListDirectorRequests(branchID string, limit int) ([]*domain.DirectorRequest, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := s.rdb().Query(`SELECT `+directorRequestCols+` FROM director_requests WHERE branch_id=? ORDER BY created_at DESC,request_id DESC LIMIT ?`, branchID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.DirectorRequest{}
	for rows.Next() {
		r, err := scanDirectorRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}
func (s *Store) CreateDirectorRequest(r *domain.DirectorRequest) (*domain.DirectorRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	old, err := scanDirectorRequest(tx.QueryRow(`SELECT `+directorRequestCols+` FROM director_requests WHERE session_id=? AND idempotency_key=?`, r.SessionID, r.IdempotencyKey))
	if err == nil {
		if old.PayloadHash != r.PayloadHash || old.BranchID != r.BranchID {
			return nil, &ports.DirectorConflict{Code: "IDEMPOTENCY_CONFLICT"}
		}
		return old, nil
	}
	if err != ports.ErrNotFound {
		return nil, err
	}
	var session, head string
	var busy bool
	if err := tx.QueryRow(`SELECT session_id,head_node_id FROM branches WHERE branch_id=?`, r.BranchID).Scan(&session, &head); err != nil {
		return nil, mapErr(err)
	}
	if session != r.SessionID || head != r.BaseNodeID {
		return nil, &ports.DirectorConflict{Code: "HEAD_CONFLICT"}
	}
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM director_requests WHERE branch_id=? AND status='generating')`, r.BranchID).Scan(&busy); err != nil {
		return nil, err
	}
	if busy {
		return nil, &ports.DirectorConflict{Code: "DIRECTOR_BUSY"}
	}
	d, err := directorDraftQ(tx, r.BranchID)
	if err != nil {
		return nil, err
	}
	version := int64(0)
	if d != nil {
		version = d.Version
	}
	if version != r.DraftVersion {
		return nil, &ports.DirectorConflict{Code: "DRAFT_CONFLICT"}
	}
	_, err = tx.Exec(`INSERT INTO director_requests(`+directorRequestCols+`) VALUES(?,?,?,?,?,?,?,?,?,'generating','','',0,'',?)`, r.RequestID, r.SessionID, r.BranchID, r.IdempotencyKey, r.PayloadHash, r.BaseNodeID, r.DraftVersion, r.DraftJSON, r.Text, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.directorOutboxTx(tx, r.RequestID, "director.started", map[string]string{"requestId": r.RequestID}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	r.CreatedAt = s.clock.Now()
	r.Status = "generating"
	return r, nil
}
func (s *Store) FinishDirectorRequest(requestID, status, reply, failure string, candidate *domain.DirectorPlan) (*domain.DirectorRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	r, err := scanDirectorRequest(tx.QueryRow(`SELECT `+directorRequestCols+` FROM director_requests WHERE request_id=?`, requestID))
	if err != nil {
		return nil, err
	}
	if r.Status != "generating" {
		return r, nil
	}
	if status != "completed" && status != "failed" && status != "cancelled" && status != "interrupted" {
		return nil, fmt.Errorf("invalid director request status")
	}
	var raw string
	if candidate != nil && status == "completed" {
		if err := domain.ValidateDirectorPlan(*candidate, true); err != nil {
			return nil, err
		}
		b, err := json.Marshal(candidate)
		if err != nil {
			return nil, err
		}
		raw = string(b)
		var head string
		if err := tx.QueryRow(`SELECT head_node_id FROM branches WHERE branch_id=?`, r.BranchID).Scan(&head); err != nil {
			return nil, err
		}
		d, err := directorDraftQ(tx, r.BranchID)
		if err != nil {
			return nil, err
		}
		version := int64(0)
		if d != nil {
			version = d.Version
		}
		if head == r.BaseNodeID && version == r.DraftVersion {
			var original domain.DirectorDraft
			if err := json.Unmarshal([]byte(r.DraftJSON), &original); err != nil {
				return nil, err
			}
			_, err = tx.Exec(`INSERT INTO director_drafts(branch_id,session_id,version,base_revision_id,plan_json,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(branch_id) DO UPDATE SET version=excluded.version,base_revision_id=excluded.base_revision_id,plan_json=excluded.plan_json,updated_at=excluded.updated_at`, r.BranchID, r.SessionID, version+1, original.BaseRevisionID, raw, s.now())
			if err != nil {
				return nil, err
			}
			r.DraftApplied = true
		}
	}
	_, err = tx.Exec(`UPDATE director_requests SET status=?,reply=?,error=?,candidate_json=?,draft_applied=? WHERE request_id=?`, status, reply, failure, raw, r.DraftApplied, requestID)
	if err != nil {
		return nil, err
	}
	if err := s.directorOutboxTx(tx, requestID, "director."+status, map[string]string{"requestId": requestID}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	r.Status, r.Reply, r.Error, r.Candidate = status, reply, failure, candidate
	return r, nil
}
func (s *Store) RecoverDirectorRequests() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT request_id FROM director_requests WHERE status='generating'`)
	if err != nil {
		return err
	}
	var pending []string
	for rows.Next() {
		var requestID string
		if err := rows.Scan(&requestID); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, requestID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, requestID := range pending {
		if _, err := tx.Exec(`UPDATE director_requests SET status='interrupted',error='服务已重启，讨论已保留，请重试' WHERE request_id=?`, requestID); err != nil {
			return err
		}
		if err := s.directorOutboxTx(tx, requestID, "director.interrupted", map[string]string{"requestId": requestID}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func rebuildDirectorProjectionsTx(tx *sql.Tx, nodes []*domain.PlotNode) error {
	ordered := append([]*domain.PlotNode(nil), nodes...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Depth < ordered[j].Depth })
	for _, node := range ordered {
		events, err := eventsQ(tx, node.NodeID)
		if err != nil {
			return err
		}
		if err := cacheDirectorEventsTx(tx, node, events); err != nil {
			return err
		}
	}
	return nil
}
