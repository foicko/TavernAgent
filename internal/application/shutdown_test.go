package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"tavernagent/internal/adapters/providers/mock"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

func TestShutdownSavesDraftAndWaitsForGenerationBeforeClosingStorage(t *testing.T) {
	st, turns, _, sid, bid, root := newTestServices(t, happyScript())
	started := make(chan struct{})
	turns.provider = cognitiveProviderFunc(func(ctx context.Context, _ ports.ChatRequest, sink ports.StreamSink) error {
		if err := sink.Chunk([]byte(mockBlock(1, "narration", "这段已写下的旅程必须保留。") + "\n")); err != nil {
			return err
		}
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	turn, err := turns.Accept(context.Background(), sid, bid, &TurnAcceptRequest{IdempotencyKey: "shutdown", ExpectedHeadID: root, Input: domain.TurnInput{Kind: "text", Text: "继续"}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("generation did not start")
	}
	closed := make(chan struct{})
	go func() { turns.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not cancel generation")
	}
	current, _ := st.GetTurn(turn.TurnID)
	if current.Status != domain.TurnAwaitingContinuation {
		t.Fatalf("draft was not recoverable: %+v", current)
	}
	attempts, _ := st.ListAttempts(turn.TurnID)
	frames, _ := st.GetDraftFrames(attempts[0].AttemptID)
	if len(frames) != 1 {
		t.Fatal("shutdown lost the persisted frame")
	}
	_, err = turns.Accept(context.Background(), sid, bid, &TurnAcceptRequest{IdempotencyKey: "after-close", ExpectedHeadID: root, Input: domain.TurnInput{Kind: "text", Text: "继续"}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "SERVER_SHUTTING_DOWN" {
		t.Fatalf("closed service accepted work: %v", err)
	}
	restarted := NewTurnService(st, mock.New([]mock.Item{mock.Frame(mockFinal(2, "", ""))}), ctxpkg.New(st, ctxpkg.DefaultOptions()), NewEventBus(st))
	defer restarted.Close()
	if _, err := restarted.Continue(context.Background(), turn.TurnID); err != nil {
		t.Fatal(err)
	}
	committed := waitTurn(t, restarted, turn.TurnID, domain.TurnCommitted)
	branch, _ := st.GetBranch(bid)
	if branch.ActiveTurnID != "" || branch.HeadNodeID != committed.ResultNodeID {
		t.Fatal("continuation failed to release the branch")
	}
}
