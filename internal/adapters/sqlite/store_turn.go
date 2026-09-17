package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// ---- turns ----

func (s *Store) scanTurn(row interface{ Scan(...any) error }) (*domain.TurnRequest, error) {
	var t domain.TurnRequest
	var status, mode, createdAt, updatedAt, after, inputJSON, resultNode, failCode, failMsg, cancel, ruleset sql.NullString
	if err := row.Scan(&t.TurnID, &t.SessionID, &t.BranchID, &t.IdempotencyKey, &t.PayloadHash, &t.ExpectedHeadID, &t.ExpectedVersion,
		&after, &status, &mode, &ruleset, &inputJSON, &resultNode, &failCode, &failMsg, &cancel, &createdAt, &updatedAt, &t.ActiveAttemptID); err != nil {
		return nil, mapErr(err)
	}
	t.AfterTurnID = after.String
	t.Status = domain.TurnStatus(status.String)
	t.Mode = mode.String
	t.RulesetVersion = ruleset.String
	t.InputJSON = inputJSON.String
	t.ResultNodeID = resultNode.String
	t.FailureCode = failCode.String
	t.FailureMessage = failMsg.String
	t.CreatedAt = s.parseTime(createdAt.String)
	t.UpdatedAt = s.parseTime(updatedAt.String)
	return &t, nil
}

const turnCols = "turn_id, session_id, branch_id, idempotency_key, payload_hash, expected_head_id, expected_version, after_turn_id, status, mode, ruleset_version, input_json, result_node_id, failure_code, failure_message, cancel_requested, created_at, updated_at, active_attempt_id"

func (s *Store) CreateTurnRequest(req *domain.TurnRequest) error {
	_, err := s.db.Exec(`INSERT INTO turn_requests(turn_id, session_id, branch_id, idempotency_key, payload_hash, expected_head_id, expected_version, after_turn_id, status, mode, ruleset_version, input_json, result_node_id, failure_code, failure_message, cancel_requested, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		req.TurnID, req.SessionID, req.BranchID, req.IdempotencyKey, req.PayloadHash, req.ExpectedHeadID, req.ExpectedVersion,
		req.AfterTurnID, string(req.Status), req.Mode, req.RulesetVersion, req.InputJSON, req.ResultNodeID, req.FailureCode, req.FailureMessage, 0, s.now(), s.now())
	return err
}

func (s *Store) GetTurn(turnID string) (*domain.TurnRequest, error) {
	return s.scanTurn(s.rdb().QueryRow(`SELECT `+turnCols+` FROM turn_requests WHERE turn_id=?`, turnID))
}

func (s *Store) FindTurnByIdempotency(sessionID, key string) (*domain.TurnRequest, error) {
	turn, err := s.scanTurn(s.rdb().QueryRow(`SELECT `+turnCols+` FROM turn_requests WHERE session_id=? AND idempotency_key=?`, sessionID, key))
	if errors.Is(err, domain.ErrItemNotFound) {
		return nil, nil
	}
	return turn, err
}

func (s *Store) UpdateTurnResult(turnID string, status domain.TurnStatus, resultNodeID, failureCode, failureMessage string) error {
	_, _, err := s.TransitionTurn(ports.TurnTransition{TurnID: turnID, Status: status, ResultNodeID: resultNodeID, FailureCode: failureCode, FailureMessage: failureMessage})
	return err
}

func (s *Store) SetCancelIntent(turnID string, cancel bool) error {
	v := 0
	if cancel {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE turn_requests SET cancel_requested=?, updated_at=? WHERE turn_id=? AND status NOT IN ('committed','cancelled','failed','conflicted')`, v, s.now(), turnID)
	return err
}

func (s *Store) HasCancelIntent(turnID string) (bool, error) {
	var v int
	if err := s.rdb().QueryRow(`SELECT cancel_requested FROM turn_requests WHERE turn_id=?`, turnID).Scan(&v); err != nil {
		return false, mapErr(err)
	}
	return v != 0, nil
}

func (s *Store) CreateAttempt(a *domain.TurnAttempt) error {
	return s.createAttempt(a)
}

func (s *Store) ListAttempts(turnID string) ([]*domain.TurnAttempt, error) {
	rows, err := s.rdb().Query(`SELECT attempt_id,turn_id,attempt_no,base_head_id,base_version,config_version FROM turn_attempts WHERE turn_id=? ORDER BY attempt_no`, turnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.TurnAttempt
	for rows.Next() {
		a := &domain.TurnAttempt{}
		if err := rows.Scan(&a.AttemptID, &a.TurnID, &a.AttemptNo, &a.BaseHeadID, &a.BaseVersion, &a.ConfigVersion); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) ResumeTurn(turnID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE turn_requests SET status=?,updated_at=? WHERE turn_id=? AND status=? AND cancel_requested=0 AND EXISTS (SELECT 1 FROM branches b WHERE b.branch_id=turn_requests.branch_id AND b.active_turn_id=turn_requests.turn_id AND b.head_node_id=turn_requests.expected_head_id AND b.version=turn_requests.expected_version)`, string(domain.TurnPreparing), s.now(), turnID, string(domain.TurnAwaitingContinuation))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (s *Store) AppendDraftFrame(attemptID string, seq int, payload, payloadHash string) error {
	_, err := s.db.Exec(`INSERT INTO draft_frames(attempt_id, frame_seq, payload, payload_hash) VALUES(?,?,?,?)`,
		attemptID, seq, payload, payloadHash)
	return err
}

func (s *Store) GetDraftFrames(attemptID string) ([]*domain.DraftFrame, error) {
	rows, err := s.rdb().Query(`SELECT attempt_id, frame_seq, payload, payload_hash FROM draft_frames WHERE attempt_id=? ORDER BY frame_seq`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.DraftFrame
	for rows.Next() {
		var f domain.DraftFrame
		if err := rows.Scan(&f.AttemptID, &f.FrameSeq, &f.Payload, &f.PayloadHash); err != nil {
			return nil, err
		}
		out = append(out, &f)
	}
	return out, rows.Err()
}

const receiptCols = `receipt_id, turn_id, action_id, roll_id, base_head_id, ruleset_version, result_json, status`

func (s *Store) SaveReceipt(r *domain.ActionReceipt) error {
	_, err := s.db.Exec(`INSERT INTO action_receipts(`+receiptCols+`) VALUES(?,?,?,?,?,?,?,?)`,
		r.ReceiptID, r.TurnID, r.ActionID, r.RollID, r.BaseHeadID, r.RulesetVersion, r.ResultJSON, string(r.Status))
	return err
}

// FindReceiptByRoll 查找同一行动实例在相同基准父节点上的既有收据。
// 找到即复用其结果，不再掷骰（契约 §5.2）。
func (s *Store) FindReceiptByRoll(rollID, baseHeadID string) (*domain.ActionReceipt, error) {
	row := s.rdb().QueryRow(`SELECT `+receiptCols+` FROM action_receipts
		WHERE roll_id=? AND base_head_id=? ORDER BY rowid DESC LIMIT 1`, rollID, baseHeadID)
	r, err := scanReceipt(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return r, nil
}

// SettleReceipts 把收据置为 committed。
//
// 只改状态、不改结果：结果在准备阶段就定了，结算是让它生效。
// 幂等：已经 committed 的再次结算不报错也不重复计数。
func (s *Store) SettleReceipts(receiptIDs []string) error {
	if len(receiptIDs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range receiptIDs {
		if _, err := tx.Exec(`UPDATE action_receipts SET status=? WHERE receipt_id=? AND status<>?`,
			string(domain.ReceiptCommitted), id, string(domain.ReceiptCommitted)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func scanReceipt(row interface{ Scan(...any) error }) (*domain.ActionReceipt, error) {
	var r domain.ActionReceipt
	var status string
	if err := row.Scan(&r.ReceiptID, &r.TurnID, &r.ActionID, &r.RollID, &r.BaseHeadID,
		&r.RulesetVersion, &r.ResultJSON, &status); err != nil {
		return nil, err
	}
	r.Status = domain.ReceiptStatus(status)
	return &r, nil
}

func (s *Store) GetReceipts(turnID string) ([]*domain.ActionReceipt, error) {
	rows, err := s.rdb().Query(`SELECT `+receiptCols+` FROM action_receipts WHERE turn_id=?`, turnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.ActionReceipt
	for rows.Next() {
		r, serr := scanReceipt(rows)
		if serr != nil {
			return nil, serr
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReceiptsAtResultNodes 按结果剧情节点 ID 批量取已结算(committed)收据，并带出
// 该节点 ID 与回合序号（供账本 "Turn #N" 标注）。
//
// 收据挂在 turn_id 上，剧情节点通过 turn_requests.result_node_id 反查，因此经
// JOIN 一步取回，避免编译层维护 节点→回合 映射。只返回已结算收据——被放弃或
// 未提交回合的判定不算权威历史事实。
func (s *Store) ReceiptsAtResultNodes(resultNodeIDs []string) ([]ports.ReceiptAtNode, error) {
	if len(resultNodeIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(resultNodeIDs)), ",")
	args := make([]any, 0, len(resultNodeIDs))
	for _, id := range resultNodeIDs {
		args = append(args, id)
	}
	q := `SELECT a.` + strings.ReplaceAll(receiptCols, ",", ",a.") +
		`, n.node_id, n.turn_number
		 FROM action_receipts a
		 JOIN turn_requests t ON t.turn_id = a.turn_id
		 JOIN plot_nodes n ON n.node_id = t.result_node_id
		 WHERE t.result_node_id IN (` + placeholders + `) AND a.status=?`
	args = append(args, string(domain.ReceiptCommitted))
	rows, err := s.rdb().Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ports.ReceiptAtNode
	for rows.Next() {
		var (
			r        domain.ActionReceipt
			status   string
			nodeID   string
			turnNumb int
		)
		if err := rows.Scan(&r.ReceiptID, &r.TurnID, &r.ActionID, &r.RollID, &r.BaseHeadID,
			&r.RulesetVersion, &r.ResultJSON, &status, &nodeID, &turnNumb); err != nil {
			return nil, err
		}
		r.Status = domain.ReceiptStatus(status)
		out = append(out, ports.ReceiptAtNode{NodeID: nodeID, TurnNumber: turnNumb, Receipt: &r})
	}
	return out, rows.Err()
}

func (s *Store) ClaimActiveTurn(branchID, turnID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var active sql.NullString
	if err := s.db.QueryRow(`SELECT active_turn_id FROM branches WHERE branch_id=?`, branchID).Scan(&active); err != nil {
		return false, err
	}
	if active.String != "" && active.String != turnID {
		return false, nil // 已有其他活动回合
	}
	res, err := s.db.Exec(`UPDATE branches SET active_turn_id=? WHERE branch_id=?`, turnID, branchID)
	if err != nil {
		return false, err
	}
	return checkRows(res) == nil, nil
}

// ReleaseActiveTurn 仅在 active_turn_id == turnID 时清空，以便分支锁定释放。
func (s *Store) ReleaseActiveTurn(branchID, turnID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var active sql.NullString
	if err := s.db.QueryRow(`SELECT active_turn_id FROM branches WHERE branch_id=?`, branchID).Scan(&active); err != nil {
		return err
	}
	if !active.Valid || active.String != turnID {
		return nil // 不做变更，不报错
	}
	_, err := s.db.Exec(`UPDATE branches SET active_turn_id='' WHERE branch_id=?`, branchID)
	return err
}

// ---- CommitTurn：提交事务 8 步（技术契约 §6.1）----

// commitBase 是提交时的分支 CAS 基准。
type commitBase struct {
	branchID string
	head     string
	version  int64
}

// CommitTurn 权威提交一个回合：单个事务里完成守卫校验 → 写节点/事件/记忆/提及 →
// 稀疏检查点 → 分支 CAS → 终态落库。任一步失败整体回滚。
func (s *Store) CommitTurn(plan *ports.CommitPlan) (*ports.CommitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("commit step [unknown]: %w", err)
	}
	defer tx.Rollback()

	base, early, err := s.commitPrecheckTx(tx, plan)
	if err != nil || early != nil {
		return early, err
	}
	conflict, err := s.commitWriteTx(tx, plan, base)
	if err != nil {
		return nil, err
	}
	if conflict != "" {
		return &ports.CommitResult{ConflictCode: conflict}, nil
	}
	if err := s.finalizeCommitTx(tx, plan, base); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit step [finalize turn]: %w", err)
	}
	return &ports.CommitResult{Committed: true, NewHeadID: plan.Node.NodeID, NewVersion: base.version + 1}, nil
}

// commitPrecheckTx 执行提交前的全部守卫（步骤 1-5）：幂等重放、终态、活动尝试、
// 取消意图、活动回合、分支头版本 CAS 与基准状态哈希。
// early 非 nil 表示应当直接把该结果返回给调用方（幂等重放或冲突）。
func (s *Store) commitPrecheckTx(tx *sql.Tx, plan *ports.CommitPlan) (commitBase, *ports.CommitResult, error) {
	var base commitBase
	res := &ports.CommitResult{}
	// 1. 再读请求终态；已提交则返回原结果（幂等，不重复生成/结算）。
	var status, resultNode string
	if err := tx.QueryRow(`SELECT status, result_node_id, branch_id FROM turn_requests WHERE turn_id=?`, plan.TurnID).Scan(&status, &resultNode, &base.branchID); err != nil {
		return base, nil, fmt.Errorf("commit step [unknown]: %w", err)
	}
	if domain.TurnStatus(status) == domain.TurnCommitted {
		res.AlreadyDone = true
		res.NewHeadID = resultNode
		return base, res, nil
	}
	if domain.TurnStatus(status).Terminal() {
		if status == string(domain.TurnCancelled) {
			res.ConflictCode = "CANCELLED"
		} else {
			res.ConflictCode = "TURN_TERMINAL"
		}
		return base, res, nil
	}
	if plan.AttemptID != "" {
		var currentAttempt string
		if err := tx.QueryRow(`SELECT active_attempt_id FROM turn_requests WHERE turn_id=?`, plan.TurnID).Scan(&currentAttempt); err != nil {
			return base, nil, err
		}
		if currentAttempt != plan.AttemptID {
			res.ConflictCode = "STALE_ATTEMPT"
			return base, res, nil
		}
	}

	// 2. 检查取消意图与提交竞争：取消意图已持久生效则禁止提交。
	var cancel int
	if err := tx.QueryRow(`SELECT cancel_requested FROM turn_requests WHERE turn_id=?`, plan.TurnID).Scan(&cancel); err != nil {
		return base, nil, fmt.Errorf("commit step [unknown]: %w", err)
	}
	if cancel != 0 {
		res.ConflictCode = "CANCELLED"
		return base, res, nil
	}

	// 3. 与活动回合校验：只有本次回合仍是该分支的活动回合才允许推进。
	var active sql.NullString
	if err := tx.QueryRow(`SELECT active_turn_id FROM branches WHERE branch_id=?`, base.branchID).Scan(&active); err != nil {
		return base, nil, fmt.Errorf("commit step [unknown]: %w", err)
	}
	if active.String != "" && active.String != plan.TurnID {
		res.ConflictCode = "ACTIVE_TURN"
		return base, res, nil
	}

	// 4. 检查分支头与版本（CAS）。
	if err := tx.QueryRow(`SELECT head_node_id, version FROM branches WHERE branch_id=?`, base.branchID).Scan(&base.head, &base.version); err != nil {
		return base, nil, fmt.Errorf("commit step [unknown]: %w", err)
	}
	if base.head != plan.ExpectedHeadID || base.version != plan.ExpectedVersion {
		res.ConflictCode = "HEAD_CONFLICT"
		return base, res, nil
	}

	// 5. 校验 baseStateHash 与已存快照一致（防止基于错误基准提交）。
	if plan.BaseStateHash != "" {
		if err := s.checkBaseHashTx(tx, plan.ExpectedHeadID, plan.BaseStateHash); err != nil {
			if errors.Is(err, errStateMismatch) {
				res.ConflictCode = "STATE_MISMATCH"
				return base, res, nil
			}
			return base, nil, fmt.Errorf("commit step [unknown]: %w", err)
		}
	}
	return base, nil, nil
}

// commitWriteTx 写入节点、事件、导演进度、记忆、提及与稀疏检查点，
// 并用 CAS 推进分支头（步骤 6-8）。返回非空 conflict 表示 CAS 失败。
func (s *Store) commitWriteTx(tx *sql.Tx, plan *ports.CommitPlan, base commitBase) (string, error) {
	if err := insertNodeTx(tx, plan.Node, s.now()); err != nil {
		return "", fmt.Errorf("commit step [insert node]: %w", err)
	}
	for _, ev := range plan.Events {
		if _, err := tx.Exec(`INSERT INTO domain_events(event_id, node_id, event_index, type, payload_json, ruleset_version) VALUES(?,?,?,?,?,?)`,
			ev.EventID, ev.NodeID, ev.EventIndex, string(ev.Type), ev.PayloadJSON, ev.RulesetVersion); err != nil {
			return "", fmt.Errorf("commit step [insert node]: %w", err)
		}
	}
	// Director progress is validated against this turn's body and committed with it.
	if err := cacheDirectorEventsTx(tx, plan.Node, plan.Events); err != nil {
		return "", fmt.Errorf("commit director progress: %w", err)
	}
	// 记忆记录与产生它的节点同事务落库：记忆必须可回溯到来源节点，
	// 且不能出现「节点已提交但记忆丢失」的半提交。
	// 实体称呼从**本节点提交后的状态**解析（与记忆的产生时刻一致）。
	commitTerms := entityTermsMap(plan.NewStateJSON)
	for _, m := range plan.Memories {
		if err := s.insertMemoryTx(tx, m, commitTerms); err != nil {
			return "", fmt.Errorf("commit step [insert memories]: %w", err)
		}
	}
	// 提及记录：本次注入过、且提交正文确实提到的记忆（契约 §8.1）。
	// 记在节点上而不是回写记忆记录，避免跨分支抬高 recency（T16/T17）。
	if err := s.insertMentionsTx(tx, plan.Node, plan.MentionedMemoryIDs, turnTextOf(plan.Node.ContentJSON), plan.NewStateJSON); err != nil {
		return "", fmt.Errorf("commit step [insert mentions]: %w", err)
	}

	// 稀疏检查点：只在间隔节点落完整状态投影（技术契约 §7）。
	// 非检查点节点不写快照不算丢数据——本节点的事件已在上一步落库，
	// StateAt 会用「最近快照 + 事件重放」还原出同样的状态与哈希。
	if shouldCheckpoint(plan.Node.Depth) {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO state_snapshots(node_id, snapshot_version, ruleset_version, state_json, state_hash) VALUES(?,?,?,?,?)`,
			plan.Node.NodeID, time.Now().UnixNano(), plan.RulesetVersion, plan.NewStateJSON, plan.NewStateHash); err != nil {
			return "", fmt.Errorf("commit step [insert snapshot]: %w", err)
		}
	}

	// CAS 更新 Branch 头节点与版本，并清空活动回合。
	resUpd, err := tx.Exec(`UPDATE branches SET head_node_id=?, version=version+1, active_turn_id='' WHERE branch_id=? AND head_node_id=? AND version=?`,
		plan.Node.NodeID, base.branchID, base.head, base.version)
	if err != nil {
		return "", fmt.Errorf("commit step [update branch]: %w", err)
	}
	if n, _ := resUpd.RowsAffected(); n == 0 {
		return "HEAD_CONFLICT", nil
	}
	return "", nil
}

// finalizeCommitTx 落库终态（步骤 9）：会话更新时间、回合 committed、
// 收据结算与 turn.committed OutboxEvent。
func (s *Store) finalizeCommitTx(tx *sql.Tx, plan *ports.CommitPlan, base commitBase) error {
	if _, err := tx.Exec(`UPDATE sessions SET updated_at=? WHERE session_id=?`, s.now(), plan.Node.SessionID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE turn_requests SET status=?, result_node_id=?, updated_at=? WHERE turn_id=?`,
		string(domain.TurnCommitted), plan.Node.NodeID, s.now(), plan.TurnID); err != nil {
		return fmt.Errorf("commit step [finalize turn]: %w", err)
	}
	for _, rid := range plan.SettleReceipts {
		if _, err := tx.Exec(`UPDATE action_receipts SET status=? WHERE receipt_id=?`, string(domain.ReceiptCommitted), rid); err != nil {
			return fmt.Errorf("commit step [finalize turn]: %w", err)
		}
	}
	seq, err := nextOutboxSeqTx(tx, plan.TurnID)
	if err != nil {
		return fmt.Errorf("commit step [finalize turn]: %w", err)
	}
	payload, _ := json.Marshal(map[string]any{
		"nodeId":        plan.Node.NodeID,
		"branchId":      base.branchID,
		"branchVersion": base.version + 1,
	})
	if _, err := tx.Exec(`INSERT INTO outbox_events(event_id, aggregate_id, sequence, type, payload_json, created_at) VALUES(?,?,?,?,?,?)`,
		plan.TurnID+"_"+fmt.Sprint(seq), plan.TurnID, seq, "turn.committed", string(payload), s.now()); err != nil {
		return fmt.Errorf("commit step [finalize turn]: %w", err)
	}
	return nil
}

var errStateMismatch = errors.New("base state hash mismatch")

// checkBaseHashTx 校验 given 头节点的快照哈希。
// 注：M0 状态下每节点都有快照；M2 起改为祖先最近检查点+事件重放并维护等价校验。
// checkBaseHashTx 校验基准状态哈希。
// 稀疏检查点下基准节点可能没有直接快照，此时用 StateAt 的
// 「最近快照 + 事件重放」结果比对——不能因为查不到快照就跳过校验，
// 否则大多数回合的基准校验会静默失效。
func (s *Store) checkBaseHashTx(tx *sql.Tx, headID, wantHash string) error {
	snap, err := stateAtQ(tx, headID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // 尚无状态投影（例如根节点快照与节点写入之间有窗口）
	}
	if err != nil {
		return err
	}
	if snap.StateHash != wantHash {
		return errStateMismatch
	}
	return nil
}

func nextOutboxSeqTx(tx *sql.Tx, aggregateID string) (int64, error) {
	var seq sql.NullInt64
	err := tx.QueryRow(`SELECT MAX(sequence) FROM outbox_events WHERE aggregate_id=?`, aggregateID).Scan(&seq)
	if err != nil {
		return 0, err
	}
	if !seq.Valid {
		return 1, nil
	}
	return seq.Int64 + 1, nil
}

// ---- outbox ----

func (s *Store) AppendOutbox(events []*domain.OutboxEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, ev := range events {
		seq, err := nextOutboxSeqTx(tx, ev.AggregateID)
		if err != nil {
			return err
		}
		ev.Sequence = seq
		if _, err := tx.Exec(`INSERT INTO outbox_events(event_id, aggregate_id, sequence, type, payload_json, created_at) VALUES(?,?,?,?,?,?)`,
			ev.EventID, ev.AggregateID, seq, ev.Type, ev.PayloadJSON, s.now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) PollOutbox(aggregateID string, afterSequence int64, limit int) ([]*domain.OutboxEvent, error) {
	rows, err := s.rdb().Query(`SELECT event_id, aggregate_id, sequence, type, payload_json, created_at FROM outbox_events WHERE aggregate_id=? AND sequence>? ORDER BY sequence LIMIT ?`,
		aggregateID, afterSequence, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.OutboxEvent
	for rows.Next() {
		var ev domain.OutboxEvent
		var at string
		if err := rows.Scan(&ev.EventID, &ev.AggregateID, &ev.Sequence, &ev.Type, &ev.PayloadJSON, &at); err != nil {
			return nil, err
		}
		ev.CreatedAt = s.parseTime(at)
		out = append(out, &ev)
	}
	return out, rows.Err()
}

// ---- helpers ----

func checkRows(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
