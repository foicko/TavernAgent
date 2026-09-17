package sqlite

import (
	"database/sql"
	"fmt"
	"slices"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

const schemaV13 = `
ALTER TABLE turn_requests ADD COLUMN active_attempt_id TEXT NOT NULL DEFAULT '';
UPDATE turn_requests SET active_attempt_id=COALESCE((SELECT attempt_id FROM turn_attempts a WHERE a.turn_id=turn_requests.turn_id ORDER BY attempt_no DESC LIMIT 1),'');
CREATE INDEX idx_turns_status ON turn_requests(status);
CREATE INDEX idx_attempts_turn_number ON turn_attempts(turn_id,attempt_no);
`

func (s *Store) TransitionTurn(change ports.TurnTransition) (*domain.TurnRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	turn, err := s.scanTurn(tx.QueryRow(`SELECT `+turnCols+` FROM turn_requests WHERE turn_id=?`, change.TurnID))
	if err != nil {
		return nil, false, err
	}
	if !domain.CanTransitionTurn(turn.Status, change.Status) ||
		(change.AttemptID != "" && turn.ActiveAttemptID != change.AttemptID) ||
		(len(change.Expected) > 0 && !slices.Contains(change.Expected, turn.Status)) {
		return turn, false, nil
	}
	if _, err := tx.Exec(`UPDATE turn_requests SET status=?,result_node_id=?,failure_code=?,failure_message=?,cancel_requested=CASE WHEN ?='cancelled' THEN 1 ELSE cancel_requested END,updated_at=? WHERE turn_id=?`,
		change.Status, change.ResultNodeID, change.FailureCode, change.FailureMessage, change.Status, s.now(), change.TurnID); err != nil {
		return nil, false, err
	}
	if change.Status.Terminal() {
		if _, err := tx.Exec(`UPDATE branches SET active_turn_id='' WHERE branch_id=? AND active_turn_id=?`, turn.BranchID, turn.TurnID); err != nil {
			return nil, false, err
		}
		if change.Status != domain.TurnCommitted {
			if _, err := tx.Exec(`UPDATE action_receipts SET status=? WHERE turn_id=? AND status=?`, domain.ReceiptAbandoned, turn.TurnID, domain.ReceiptPrepared); err != nil {
				return nil, false, err
			}
		}
	}
	kind := ""
	switch change.Status {
	case domain.TurnCancelled:
		kind = "turn.cancelled"
	case domain.TurnFailed:
		kind = "turn.failed"
	case domain.TurnConflicted:
		kind = "turn.conflicted"
	case domain.TurnAwaitingContinuation:
		kind = "turn.truncated"
	}
	if kind != "" {
		payload := map[string]any{"turnId": turn.TurnID, "attemptId": turn.ActiveAttemptID, "code": change.FailureCode, "message": change.FailureMessage, "reason": change.FailureMessage, "retryable": change.Retryable}
		if err := s.directorOutboxTx(tx, turn.TurnID, kind, payload); err != nil {
			return nil, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	turn.Status, turn.ResultNodeID = change.Status, change.ResultNodeID
	turn.FailureCode, turn.FailureMessage = change.FailureCode, change.FailureMessage
	return turn, true, nil
}

// Called only during startup, before accepting requests. Terminal requests from
// older versions may still own a branch; clear those abandoned locks first.
func (s *Store) InterruptedTurns() ([]*domain.TurnRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`UPDATE branches SET active_turn_id='' WHERE active_turn_id<>'' AND NOT EXISTS (SELECT 1 FROM turn_requests t WHERE t.turn_id=branches.active_turn_id AND t.status NOT IN ('committed','cancelled','failed','conflicted'))`); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT ` + turnCols + ` FROM turn_requests WHERE status NOT IN ('committed','cancelled','failed','conflicted') ORDER BY created_at,turn_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var turns []*domain.TurnRequest
	for rows.Next() {
		t, err := s.scanTurn(rows)
		if err != nil {
			return nil, err
		}
		turns = append(turns, t)
	}
	return turns, rows.Err()
}

func (s *Store) createAttempt(a *domain.TurnAttempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status domain.TurnStatus
	var latest sql.NullInt64
	if err := tx.QueryRow(`SELECT status FROM turn_requests WHERE turn_id=?`, a.TurnID).Scan(&status); err != nil {
		return err
	}
	if status.Terminal() {
		return fmt.Errorf("turn %s is terminal", a.TurnID)
	}
	if err := tx.QueryRow(`SELECT MAX(attempt_no) FROM turn_attempts WHERE turn_id=?`, a.TurnID).Scan(&latest); err != nil {
		return err
	}
	if latest.Valid && int64(a.AttemptNo) <= latest.Int64 {
		return fmt.Errorf("stale attempt for turn %s", a.TurnID)
	}
	if _, err := tx.Exec(`INSERT INTO turn_attempts(attempt_id,turn_id,attempt_no,base_head_id,base_version,config_version,created_at) VALUES(?,?,?,?,?,?,?)`, a.AttemptID, a.TurnID, a.AttemptNo, a.BaseHeadID, a.BaseVersion, a.ConfigVersion, s.now()); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE turn_requests SET active_attempt_id=? WHERE turn_id=?`, a.AttemptID, a.TurnID); err != nil {
		return err
	}
	return tx.Commit()
}
