package application

import (
	"context"
	"fmt"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

func TestRecoveryMatrixIsIdempotentAndNeverCallsModel(t *testing.T) {
	cases := []struct {
		name    string
		status  domain.TurnStatus
		frames  []string
		corrupt bool
		want    domain.TurnStatus
	}{
		{"accepted", domain.TurnAccepted, nil, false, domain.TurnFailed},
		{"queued", domain.TurnQueued, nil, false, domain.TurnFailed},
		{"preparing", domain.TurnPreparing, nil, false, domain.TurnFailed},
		{"empty stream", domain.TurnGenerating, nil, false, domain.TurnFailed},
		{"complete", domain.TurnGenerating, []string{mockBlock(1, "narration", "可恢复的完整正文。"), mockFinal(2, "", "")}, false, domain.TurnCommitted},
		{"validating", domain.TurnValidating, []string{mockBlock(1, "narration", "校验中断的正文。"), mockFinal(2, "", "")}, false, domain.TurnCommitted},
		{"partial", domain.TurnGenerating, []string{mockBlock(1, "narration", "已经保存的半轮。")}, false, domain.TurnAwaitingContinuation},
		{"checksum", domain.TurnGenerating, []string{mockBlock(1, "narration", "损坏的帧。")}, true, domain.TurnFailed},
		{"invalid payload", domain.TurnGenerating, []string{`{"v":1,"seq":1,"type":"block"`}, false, domain.TurnFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			store, err := sqlite.Open(dir, ports.RealClock{})
			if err != nil {
				t.Fatal(err)
			}
			setup, err := NewSessionService(store).Setup(context.Background(), &SessionSetupRequest{CharacterJSON: testCard, OpeningText: "独立的恢复测试。"})
			if err != nil {
				t.Fatal(err)
			}
			turn := &domain.TurnRequest{TurnID: "recover", SessionID: setup.Session.SessionID, BranchID: setup.Branch.BranchID, IdempotencyKey: "recover", PayloadHash: "seed", ExpectedHeadID: setup.RootNode.NodeID, Status: tc.status, Mode: "structured", InputJSON: `{"kind":"text","text":"继续"}`}
			if err = store.CreateTurnRequest(turn); err != nil {
				t.Fatal(err)
			}
			if ok, err := store.ClaimActiveTurn(turn.BranchID, turn.TurnID); err != nil || !ok {
				t.Fatal(err)
			}
			if err = store.CreateAttempt(&domain.TurnAttempt{AttemptID: "a1", TurnID: turn.TurnID, AttemptNo: 1, BaseHeadID: turn.ExpectedHeadID}); err != nil {
				t.Fatal(err)
			}
			for i, frame := range tc.frames {
				hash := hashString(frame)
				if tc.corrupt {
					hash = "bad"
				}
				if err = store.AppendDraftFrame("a1", i+1, frame, hash); err != nil {
					t.Fatal(err)
				}
			}
			store.Close()
			store, err = sqlite.Open(dir, ports.RealClock{})
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			// No provider is installed: recovery cannot silently regenerate or pay.
			service := NewTurnService(store, nil, ctxpkg.New(store, ctxpkg.DefaultOptions()), NewEventBus(store))
			defer service.Close()
			for pass := 0; pass < 3; pass++ {
				if err = service.Recover(); err != nil {
					t.Fatalf("recovery %d: %v", pass, err)
				}
				got, err := store.GetTurn(turn.TurnID)
				if err != nil || got.Status != tc.want {
					t.Fatalf("status=%+v err=%v", got, err)
				}
				if tc.want == domain.TurnFailed && got.FailureCode != "PROCESS_INTERRUPTED" {
					t.Fatal(got.FailureCode)
				}
				branch, _ := store.GetBranch(turn.BranchID)
				if tc.want.Terminal() && branch.ActiveTurnID != "" {
					t.Fatal("terminal lock survived")
				}
				if tc.want == domain.TurnCommitted {
					if branch.Version != 1 || got.ResultNodeID != branch.HeadNodeID {
						t.Fatal("duplicate or incomplete settlement")
					}
					if _, err = service.Cancel(context.Background(), turn.TurnID); err != nil {
						t.Fatal(err)
					}
				}
			}
			rows, err := store.RecentTurnUsage(100)
			if err != nil || len(rows) != 0 {
				t.Fatalf("recovery made model calls: %+v %v", rows, err)
			}
		})
	}
}

func TestOlderAttemptCannotChangeCurrentAttemptOrTerminal(t *testing.T) {
	f := newMemoryFixture(t)
	turn := &domain.TurnRequest{TurnID: "fenced", SessionID: f.sess.SessionID, BranchID: f.main.BranchID, IdempotencyKey: "fenced", PayloadHash: "seed", ExpectedHeadID: "root", Status: domain.TurnGenerating, Mode: "structured"}
	if err := f.store.CreateTurnRequest(turn); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		if err := f.store.CreateAttempt(&domain.TurnAttempt{TurnID: turn.TurnID, AttemptID: fmt.Sprint(i), AttemptNo: i, BaseHeadID: "root"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, changed, err := f.store.TransitionTurn(ports.TurnTransition{TurnID: turn.TurnID, AttemptID: "1", Status: domain.TurnFailed}); err != nil || changed {
		t.Fatal("stale attempt accepted")
	}
	if _, changed, err := f.store.TransitionTurn(ports.TurnTransition{TurnID: turn.TurnID, AttemptID: "2", Status: domain.TurnCancelled}); err != nil || !changed {
		t.Fatal("current attempt rejected")
	}
	if _, changed, err := f.store.TransitionTurn(ports.TurnTransition{TurnID: turn.TurnID, AttemptID: "2", Status: domain.TurnFailed}); err != nil || changed {
		t.Fatal("terminal overwritten")
	}
}
