package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

func (s *Store) IsAncestor(ancestorID, nodeID string) (bool, error) {
	return isAncestorQ(s.rdb(), ancestorID, nodeID)
}

func (s *Store) ActiveMemoryCount(nodeID string) (int, error) {
	var count int
	err := s.rdb().QueryRow(recallCTE+` SELECT COUNT(*) FROM memory_records m JOIN up ON up.node_key=m.source_node_id
 WHERE m.hidden=0 AND NOT EXISTS(SELECT 1 FROM memory_records c JOIN up u ON u.node_key=c.source_node_id WHERE c.supersedes=m.memory_id)`, nodeID).Scan(&count)
	return count, err
}

func isAncestorQ(q queryer, ancestorID, nodeID string) (bool, error) {
	var found bool
	err := q.QueryRow(`WITH RECURSIVE up(id,parent_id) AS (
 SELECT node_id,parent_id FROM plot_nodes WHERE node_id=?
 UNION ALL SELECT p.node_id,p.parent_id FROM plot_nodes p JOIN up ON p.node_id=up.parent_id WHERE up.id<>?
) SELECT EXISTS(SELECT 1 FROM up WHERE id=?)`, nodeID, ancestorID, ancestorID).Scan(&found)
	return found, err
}

func (s *Store) FindCognitiveBatch(branchID, sourceTurnID string) (*ports.CommitResult, error) {
	var node string
	err := s.rdb().QueryRow(`SELECT node_id FROM memory_batches WHERE branch_id=? AND source_turn_id=?`, branchID, sourceTurnID).Scan(&node)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ports.CommitResult{AlreadyDone: true, NewHeadID: node}, nil
}

func (s *Store) FindMemoryBatch(batchID, sessionID, branchID, payloadHash string) (*ports.CommitResult, error) {
	var node, session, branch, hash string
	err := s.rdb().QueryRow(`SELECT node_id,session_id,branch_id,payload_hash FROM memory_batches WHERE batch_id=?`, batchID).Scan(&node, &session, &branch, &hash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if hash != payloadHash || session != sessionID || branch != branchID {
		return &ports.CommitResult{ConflictCode: "IDEMPOTENCY_CONFLICT"}, nil
	}
	return &ports.CommitResult{AlreadyDone: true, NewHeadID: node}, nil
}

func (s *Store) CommitMemoryBatch(ctx context.Context, b *ports.MemoryBatch) (*ports.CommitResult, error) {
	if b == nil || b.BatchID == "" || b.NodeID == "" || b.PayloadHash == "" {
		return nil, fmt.Errorf("incomplete memory batch")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var existing, hash, existingSession, existingBranch string
	err = tx.QueryRow(`SELECT node_id,payload_hash,session_id,branch_id FROM memory_batches WHERE batch_id=?`, b.BatchID).Scan(&existing, &hash, &existingSession, &existingBranch)
	if err == nil {
		if hash != b.PayloadHash || existingSession != b.SessionID || existingBranch != b.BranchID {
			return &ports.CommitResult{ConflictCode: "IDEMPOTENCY_CONFLICT"}, nil
		}
		return &ports.CommitResult{AlreadyDone: true, NewHeadID: existing}, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	var session, head, active, ruleset string
	var version int64
	err = tx.QueryRow(`SELECT b.session_id,b.head_node_id,b.active_turn_id,b.version,s.ruleset_version
 FROM branches b JOIN sessions s ON s.session_id=b.session_id WHERE b.branch_id=?`, b.BranchID).Scan(&session, &head, &active, &version, &ruleset)
	if err != nil {
		return nil, mapErr(err)
	}
	if session != b.SessionID {
		return &ports.CommitResult{ConflictCode: "BRANCH_SESSION_MISMATCH"}, nil
	}
	if head != b.ExpectedHeadID || version != b.ExpectedVersion {
		return &ports.CommitResult{ConflictCode: "HEAD_CONFLICT"}, nil
	}
	if active != "" {
		return &ports.CommitResult{ConflictCode: "QUEUE_FULL"}, nil
	}
	sourceNode := head
	if b.SourceTurnID != "" {
		var sourceSession, sourceBranch, status string
		err = tx.QueryRow(`SELECT result_node_id,session_id,branch_id,status FROM turn_requests WHERE turn_id=?`, b.SourceTurnID).Scan(&sourceNode, &sourceSession, &sourceBranch, &status)
		if err != nil {
			return nil, mapErr(err)
		}
		visible, err := isAncestorQ(tx, sourceNode, head)
		if err != nil {
			return nil, err
		}
		if sourceSession != session || sourceBranch != b.BranchID || status != string(domain.TurnCommitted) || !visible {
			return &ports.CommitResult{ConflictCode: "COGNITIVE_SOURCE_CONFLICT"}, nil
		}
	}
	nodeID := head
	changed := len(b.Memories) > 0 || len(b.Events) > 0
	if changed {
		snap, err := stateAtQ(tx, head)
		if err != nil {
			return nil, err
		}
		state, err := domain.UnmarshalWorld(snap.StateJSON)
		if err != nil {
			return nil, err
		}
		var depth, turn int
		if err := tx.QueryRow(`SELECT depth,turn_number FROM plot_nodes WHERE node_id=?`, head).Scan(&depth, &turn); err != nil {
			return nil, err
		}
		nodeID = b.NodeID
		content, _ := json.Marshal(map[string]any{"reason": b.Reason, "sourceTurnId": b.SourceTurnID, "sourceNodeId": sourceNode, "memoryChanges": b.Memories})
		node := &domain.PlotNode{NodeID: nodeID, SessionID: session, ParentID: head, Kind: domain.NodeKindMemoryChange,
			Depth: depth + 1, TurnNumber: turn, SchemaVersion: 1, ContentJSON: string(content)}
		if err := insertNodeTx(tx, node, s.now()); err != nil {
			return nil, err
		}
		// All references must point backwards along this path; this also rules out
		// cycles, revisions of sibling records and duplicate replacements.
		seen := map[string]bool{}
		for _, m := range b.Memories {
			if m.SourceNodeID != nodeID {
				return nil, fmt.Errorf("memory source does not match batch node")
			}
			if m.Supersedes != "" {
				if seen[m.Supersedes] {
					return nil, fmt.Errorf("duplicate replacement")
				}
				seen[m.Supersedes] = true
				var source string
				if err := tx.QueryRow(`SELECT source_node_id FROM memory_records WHERE memory_id=?`, m.Supersedes).Scan(&source); err != nil {
					return nil, err
				}
				ok, err := isAncestorQ(tx, source, head)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, fmt.Errorf("replacement crosses branch")
				}
			}
		}
		for i, ev := range b.Events {
			ev.NodeID = nodeID
			ev.EventID = fmt.Sprintf("%s_event_%d", b.BatchID, i)
			ev.EventIndex = i
			ev.RulesetVersion = ruleset
			if _, err := domain.ApplyEvent(state, ev); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(`INSERT INTO domain_events(event_id,node_id,event_index,type,payload_json,ruleset_version) VALUES(?,?,?,?,?,?)`, ev.EventID, nodeID, i, ev.Type, ev.PayloadJSON, ruleset); err != nil {
				return nil, err
			}
		}
		stateJSON, _ := state.Marshal()
		terms := entityTermsMap(stateJSON)
		// Inserts precede hidden overlays. Original rows remain available to old
		// branches, exports and audits; no irreversible memory deletion occurs.
		for _, m := range b.Memories {
			if err := s.insertMemoryTx(tx, m, terms); err != nil {
				return nil, err
			}
		}
		if _, err := tx.Exec(`INSERT INTO state_snapshots(node_id,snapshot_version,ruleset_version,state_json,state_hash) VALUES(?,1,?,?,?)`, nodeID, ruleset, stateJSON, state.HashID()); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`UPDATE branches SET head_node_id=?,version=version+1 WHERE branch_id=? AND head_node_id=? AND version=? AND active_turn_id=''`, nodeID, b.BranchID, head, version); err != nil {
			return nil, err
		}
		version++
		if _, err := tx.Exec(`UPDATE sessions SET updated_at=? WHERE session_id=?`, s.now(), session); err != nil {
			return nil, err
		}
		seq, err := nextOutboxSeqTx(tx, session)
		if err != nil {
			return nil, err
		}
		payload, _ := json.Marshal(map[string]any{"sessionId": session, "branchId": b.BranchID, "nodeId": nodeID, "branchVersion": version, "reason": b.Reason})
		if _, err := tx.Exec(`INSERT INTO outbox_events(event_id,aggregate_id,sequence,type,payload_json,created_at) VALUES(?,?,?,'session.updated',?,?)`, b.BatchID+"_updated", session, seq, string(payload), s.now()); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(`INSERT INTO memory_batches(batch_id,session_id,branch_id,source_turn_id,node_id,payload_hash,created_at) VALUES(?,?,?,?,?,?,?)`, b.BatchID, session, b.BranchID, b.SourceTurnID, nodeID, b.PayloadHash, s.now()); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &ports.CommitResult{Committed: true, NewHeadID: nodeID, NewVersion: version}, nil
}
