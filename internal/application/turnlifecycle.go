package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/protocol"
)

type attemptContextKey struct{}
type recoveryContextKey struct{}
type turnCancel struct{ cancel context.CancelFunc }

func withAttempt(ctx context.Context, attempt string) context.Context {
	return context.WithValue(ctx, attemptContextKey{}, attempt)
}

func attemptFrom(ctx context.Context) string {
	id, _ := ctx.Value(attemptContextKey{}).(string)
	return id
}

// Cleanup only removes its own registration: a continuation can start while
// the previous worker is returning after publishing a truncation event.
func (s *TurnService) registerCancel(turnID string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(s.workCtx)
	handle := &turnCancel{cancel: cancel}
	s.cancelMu.Lock()
	s.cancels[turnID] = handle
	s.cancelMu.Unlock()
	return ctx, func() {
		s.cancelMu.Lock()
		if s.cancels[turnID] == handle {
			delete(s.cancels, turnID)
		}
		s.cancelMu.Unlock()
		cancel()
	}
}

func (s *TurnService) transition(ctx context.Context, turnID string, status domain.TurnStatus, code, message string, retryable bool) bool {
	_, changed, err := s.store.TransitionTurn(ports.TurnTransition{
		TurnID: turnID, AttemptID: attemptFrom(ctx), Status: status,
		FailureCode: code, FailureMessage: message, Retryable: retryable,
	})
	if err != nil {
		slog.Error("turn transition failed", "turnId", turnID, "attemptId", attemptFrom(ctx), "status", status, "error", err)
		return false
	}
	if changed && s.bus != nil {
		// Notifications only wake subscribers; authoritative events are in the
		// same transaction as status and locks, and are replayed from the outbox.
		s.bus.BroadcastOnly(&domain.OutboxEvent{AggregateID: turnID, Type: "turn.state.updated"})
	}
	return changed
}

func (s *TurnService) restoreDraft(turnID string) (*protocol.StreamParser, string, int, int, error) {
	parser := protocol.NewStreamParser()
	attempts, err := s.store.ListAttempts(turnID)
	if err != nil {
		return nil, "", 0, 0, err
	}
	var previous strings.Builder
	lastSeq, nextAttempt := 0, 1
	for _, a := range attempts {
		nextAttempt = max(nextAttempt, a.AttemptNo+1)
		frames, err := s.store.GetDraftFrames(a.AttemptID)
		if err != nil {
			return nil, "", 0, 0, err
		}
		for _, frame := range frames {
			if frame.PayloadHash != "" && hashString(frame.Payload) != frame.PayloadHash {
				return nil, "", 0, 0, errors.New("draft frame checksum mismatch")
			}
			if err := parser.Feed([]byte(frame.Payload + "\n")); err != nil {
				return nil, "", 0, 0, err
			}
			previous.WriteString(frame.Payload + "\n")
			lastSeq = max(lastSeq, frame.FrameSeq)
		}
	}
	return parser, previous.String(), lastSeq, nextAttempt, nil
}

// Recover runs once after acquiring the data-directory lock and before HTTP
// starts. It makes no model calls, never rerolls receipts, and is idempotent.
func (s *TurnService) Recover() error {
	turns, err := s.store.InterruptedTurns()
	if err != nil {
		return err
	}
	for _, turn := range turns {
		ctx := withAttempt(context.WithValue(context.Background(), recoveryContextKey{}, true), turn.ActiveAttemptID)
		cancelled, err := s.store.HasCancelIntent(turn.TurnID)
		if err != nil {
			return err
		}
		if cancelled {
			if _, err := s.Cancel(ctx, turn.TurnID); err != nil {
				return err
			}
			continue
		}
		branch, err := s.store.GetBranch(turn.BranchID)
		if err != nil {
			return fmt.Errorf("recover %s: %w", turn.TurnID, err)
		}
		if branch.HeadNodeID != turn.ExpectedHeadID || branch.Version != turn.ExpectedVersion || (branch.ActiveTurnID != "" && branch.ActiveTurnID != turn.TurnID) {
			if !s.transition(ctx, turn.TurnID, domain.TurnConflicted, "HEAD_CONFLICT", "服务中断期间分支已变化，请基于最新进度重试", true) {
				return fmt.Errorf("recover conflicting turn %s", turn.TurnID)
			}
			continue
		}
		if branch.ActiveTurnID == "" {
			if ok, err := s.store.ClaimActiveTurn(turn.BranchID, turn.TurnID); err != nil || !ok {
				return fmt.Errorf("recover branch ownership for %s: %v", turn.TurnID, err)
			}
		}
		if turn.Status == domain.TurnAwaitingApproval {
			continue
		}
		parser, _, _, _, restoreErr := s.restoreDraft(turn.TurnID)
		if restoreErr != nil || len(parser.Blocks()) == 0 {
			message := "进程退出中断了回合，尚无可恢复正文，请重试"
			if restoreErr != nil {
				message = "草稿校验失败，请重试：" + restoreErr.Error()
			}
			if !s.transition(ctx, turn.TurnID, domain.TurnFailed, "PROCESS_INTERRUPTED", message, true) {
				return fmt.Errorf("recover interrupted turn %s", turn.TurnID)
			}
			continue
		}
		draft, finishErr := parser.Finish()
		if finishErr == nil {
			snap, err := s.store.StateAt(turn.ExpectedHeadID)
			if err != nil {
				return err
			}
			state, err := domain.UnmarshalWorld(snap.StateJSON)
			if err != nil {
				return err
			}
			if _, err := s.commitDraft(ctx, turn, draft, state, parser.Mode(), nil); err != nil {
				current, readErr := s.store.GetTurn(turn.TurnID)
				if readErr != nil || !current.Status.Terminal() {
					return fmt.Errorf("recover complete turn %s: %w", turn.TurnID, err)
				}
			}
			continue
		}
		if !errors.Is(finishErr, protocol.ErrMissingFinal) {
			if !s.transition(ctx, turn.TurnID, domain.TurnFailed, "PROCESS_INTERRUPTED", "中断草稿无法恢复，请重试", true) {
				return fmt.Errorf("recover invalid draft %s", turn.TurnID)
			}
			continue
		}
		if turn.Status == domain.TurnAwaitingContinuation {
			continue // Validated above; repeated startup must not emit another pause.
		}
		if !s.transition(ctx, turn.TurnID, domain.TurnAwaitingContinuation, "PROCESS_INTERRUPTED", "服务已重启，已保存的正文可以继续或放弃", true) {
			return fmt.Errorf("recover draft %s", turn.TurnID)
		}
	}
	return nil
}
