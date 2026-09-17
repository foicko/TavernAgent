package application

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// Seed the durable boundary left by a terminated process, then use the same
// initialization calls as main. No live user database or external model is used.
func TestReleaseRestartRecoversAbandonedGeneratingTurn(t *testing.T) {
	dir := t.TempDir()
	st, err := sqlite.Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	setup, err := NewSessionService(st).Setup(context.Background(), &SessionSetupRequest{
		CharacterJSON: testCard, OpeningText: "旅人推开了酒馆的门。",
	})
	if err != nil {
		st.Close()
		t.Fatal(err)
	}
	turn := &domain.TurnRequest{
		TurnID: "review-abandoned", SessionID: setup.Session.SessionID,
		BranchID: setup.Branch.BranchID, IdempotencyKey: "review-abandoned",
		PayloadHash: "seed", ExpectedHeadID: setup.RootNode.NodeID,
		Status: domain.TurnGenerating, Mode: "structured",
		InputJSON: "{\"kind\":\"text\",\"text\":\"继续\"}",
	}
	if err := st.CreateTurnRequest(turn); err != nil {
		t.Fatal(err)
	}
	if ok, err := st.ClaimActiveTurn(turn.BranchID, turn.TurnID); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	attempt := &domain.TurnAttempt{
		AttemptID: "review-attempt", TurnID: turn.TurnID, AttemptNo: 1,
		BaseHeadID: turn.ExpectedHeadID,
	}
	if err := st.CreateAttempt(attempt); err != nil {
		t.Fatal(err)
	}
	frame := mockBlock(1, "narration", "这段已经持久化的正文应当能够继续。")
	if err := st.AppendDraftFrame(attempt.AttemptID, 1, frame, hashString(frame)); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	bus := NewEventBus(reopened)
	compiler := ctxpkg.New(reopened, ctxpkg.DefaultOptions())
	manager := NewProviderManager(nil, nil, mock.New(happyScript()))
	turns := NewTurnServiceWithManager(reopened, manager, compiler, bus)
	defer turns.Close()
	director := NewDirectorService(reopened, manager, compiler, bus, nil)
	defer director.Close()
	if err := turns.Recover(); err != nil {
		t.Fatal(err)
	}
	if err := director.Recover(); err != nil {
		t.Fatal(err)
	}
	current, err := turns.Get(turn.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	_, continuationErr := turns.Continue(context.Background(), turn.TurnID)
	_, acceptErr := turns.Accept(context.Background(), turn.SessionID, turn.BranchID, &TurnAcceptRequest{
		IdempotencyKey: "review-next", ExpectedHeadID: turn.ExpectedHeadID,
		Input: domain.TurnInput{Kind: "text", Text: "下一步"},
	})
	t.Logf("after restart: status=%s continue=%v next-turn=%v", current.Status, continuationErr, acceptErr)
	if current.Status == domain.TurnGenerating {
		t.Fatal("restart left a generating turn with no worker; persisted draft is not continuable")
	}
}

type reviewCancelGate struct {
	ports.TurnDeps
	reached chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (g *reviewCancelGate) TransitionTurn(change ports.TurnTransition) (*domain.TurnRequest, bool, error) {
	if change.Status == domain.TurnCancelled {
		g.once.Do(func() {
			close(g.reached)
			<-g.resume
		})
	}
	return g.TurnDeps.TransitionTurn(change)
}

// Exercise a legal ordering: cancel reads "generating", commit completes, then
// cancel persists its intent. A committed turn must stay committed.
func TestReleaseLateCancelPreservesCommittedResult(t *testing.T) {
	st, turns, _, sid, bid, root := newTestServices(t, happyScript())
	started, finish := make(chan struct{}), make(chan struct{})
	var finishOnce, resumeOnce sync.Once
	gate := &reviewCancelGate{TurnDeps: turns.store, reached: make(chan struct{}), resume: make(chan struct{})}
	turns.store = gate
	t.Cleanup(func() {
		finishOnce.Do(func() { close(finish) })
		resumeOnce.Do(func() { close(gate.resume) })
	})
	turns.provider = cognitiveProviderFunc(func(ctx context.Context, _ ports.ChatRequest, sink ports.StreamSink) error {
		close(started)
		select {
		case <-finish:
		case <-ctx.Done():
			return ctx.Err()
		}
		return sink.Chunk([]byte(mockBlock(1, "narration", "旅人完成了这一步行动。") + "\n" + mockFinal(2, "", "") + "\n"))
	})
	turn, err := turns.Accept(context.Background(), sid, bid, &TurnAcceptRequest{
		IdempotencyKey: "review-cancel-race", ExpectedHeadID: root,
		Input: domain.TurnInput{Kind: "text", Text: "前进"},
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("provider not started")
	}
	cancelDone := make(chan error, 1)
	go func() { _, err := turns.Cancel(context.Background(), turn.TurnID); cancelDone <- err }()
	select {
	case <-gate.reached:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not reach the gate")
	}
	finishOnce.Do(func() { close(finish) })
	committed := waitTurn(t, turns, turn.TurnID, domain.TurnCommitted)
	turns.workers.Wait()
	resumeOnce.Do(func() { close(gate.resume) })
	select {
	case err := <-cancelDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not finish")
	}
	current, err := turns.Get(turn.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	branch, err := st.GetBranch(bid)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("committed=%s after-cancel=%s result=%q branch-head=%s", committed.ResultNodeID, current.Status, current.ResultNodeID, branch.HeadNodeID)
	if current.Status != domain.TurnCommitted || current.ResultNodeID != committed.ResultNodeID {
		t.Fatal("late cancel overwrote the committed turn status or result pointer")
	}
}

// compileGate 让"读取基准状态"停在半路，用来观察取消发生在编译期时的行为。
type compileGate struct {
	ports.TurnDeps
	reached chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (g *compileGate) StateAt(nodeID string) (*domain.StateSnapshot, error) {
	g.once.Do(func() { close(g.reached); <-g.resume })
	return g.TurnDeps.StateAt(nodeID)
}

// 取消发生在"受理之后、开始流式输出之前"（编译期是最长的一段）：这个回合
// 不能再调用模型，也不能把分支锁一直占到超时。此前这条窗口零测试覆盖。
func TestCancelDuringCompilationNeverCallsProvider(t *testing.T) {
	st, turns, _, sid, bid, root := newTestServices(t, happyScript())
	gate := &compileGate{TurnDeps: turns.store, reached: make(chan struct{}), resume: make(chan struct{})}
	turns.store = gate
	var resumeOnce sync.Once
	t.Cleanup(func() { resumeOnce.Do(func() { close(gate.resume) }) })
	var calls atomic.Int32
	turns.provider = cognitiveProviderFunc(func(ctx context.Context, _ ports.ChatRequest, sink ports.StreamSink) error {
		calls.Add(1)
		return sink.Chunk([]byte(mockBlock(1, "narration", "不应出现。") + "\n" + mockFinal(2, "", "") + "\n"))
	})

	turn, err := turns.Accept(context.Background(), sid, bid, &TurnAcceptRequest{
		IdempotencyKey: "cancel-during-compile", ExpectedHeadID: root,
		Input: domain.TurnInput{Kind: "text", Text: "前进"},
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-gate.reached:
	case <-time.After(3 * time.Second):
		t.Fatal("回合未进入编译期")
	}
	if _, err := turns.Cancel(context.Background(), turn.TurnID); err != nil {
		t.Fatal(err)
	}
	resumeOnce.Do(func() { close(gate.resume) })
	turns.workers.Wait()

	if calls.Load() != 0 {
		t.Fatalf("取消后的回合仍调用模型 %d 次", calls.Load())
	}
	current, err := st.GetTurn(turn.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.TurnCancelled {
		t.Fatalf("终态 = %s，期望 cancelled", current.Status)
	}
	branch, err := st.GetBranch(bid)
	if err != nil {
		t.Fatal(err)
	}
	if branch.ActiveTurnID != "" {
		t.Fatalf("分支锁未释放：%s", branch.ActiveTurnID)
	}
}
