package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"tavernagent/internal/domain"
)

// BEGIN IMMEDIATE serializes the lookup and random draw across Store instances,
// including two local processes opening the same story database.
func (s *Store) PrepareReceipt(ctx context.Context, receipt *domain.ActionReceipt, draw func() (string, error)) (*domain.ActionReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return nil, err
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `ROLLBACK`) }()
	current, err := scanReceipt(conn.QueryRowContext(ctx, `SELECT `+receiptCols+` FROM action_receipts WHERE turn_id=? AND action_id=? LIMIT 1`, receipt.TurnID, receipt.ActionID))
	if err == nil {
		if current.RollID != receipt.RollID || current.BaseHeadID != receipt.BaseHeadID || current.RulesetVersion != receipt.RulesetVersion {
			return nil, fmt.Errorf("turn already has a different action instance")
		}
		return current, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	prior, err := scanReceipt(conn.QueryRowContext(ctx, `SELECT `+receiptCols+` FROM action_receipts WHERE roll_id=? AND base_head_id=? ORDER BY rowid LIMIT 1`, receipt.RollID, receipt.BaseHeadID))
	if err == nil {
		if prior.ActionID != receipt.ActionID || prior.RulesetVersion != receipt.RulesetVersion {
			return nil, fmt.Errorf("incompatible prior receipt")
		}
		receipt.ResultJSON = prior.ResultJSON
	} else if err == sql.ErrNoRows {
		receipt.ResultJSON, err = draw()
		if err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO action_receipts(`+receiptCols+`) VALUES(?,?,?,?,?,?,?,?)`, receipt.ReceiptID, receipt.TurnID, receipt.ActionID, receipt.RollID, receipt.BaseHeadID, receipt.RulesetVersion, receipt.ResultJSON, string(receipt.Status)); err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return nil, err
	}
	return receipt, nil
}
