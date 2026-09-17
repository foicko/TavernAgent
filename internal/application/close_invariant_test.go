package application

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// closeWatchStore 给存储加一道"关闭之后不得再写"的探针。
//
// CI 里偶发的 `sql: database is closed`（关闭/用量路径）来源就是某个 worker
// 活过了存储关闭。这个用例把那个窗口变成确定性断言：只要 TurnService.Close
// 没有排空 worker，计数就不为零。
type closeWatchStore struct {
	ports.Store
	shut   atomic.Bool
	writes atomic.Int64
}

func (s *closeWatchStore) watch() {
	if s.shut.Load() {
		s.writes.Add(1)
	}
}

func (s *closeWatchStore) TransitionTurn(change ports.TurnTransition) (*domain.TurnRequest, bool, error) {
	s.watch()
	return s.Store.TransitionTurn(change)
}

func (s *closeWatchStore) RecordTurnUsage(rec ports.TurnUsageRecord) error {
	s.watch()
	return s.Store.RecordTurnUsage(rec)
}

func (s *closeWatchStore) AppendDraftFrame(attemptID string, seq int, payload, payloadHash string) error {
	s.watch()
	return s.Store.AppendDraftFrame(attemptID, seq, payload, payloadHash)
}

func (s *closeWatchStore) AppendOutbox(events []*domain.OutboxEvent) error {
	s.watch()
	return s.Store.AppendOutbox(events)
}

// 取消一个正在流式输出的回合后立刻关停：服务必须先排空 worker，再轮到存储关闭。
// 顺序反了就会在关闭后的存储上写用量与终态——也就是那条 flake 日志。
func TestCancelledTurnWritesFinishBeforeStorageCloses(t *testing.T) {
	dir := t.TempDir()
	inner, err := sqlite.Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	watch := &closeWatchStore{Store: inner}
	prov := &blockingProvider{release: make(chan struct{})}
	turnSvc := NewTurnService(watch, prov, ctxpkg.New(watch, ctxpkg.DefaultOptions()), NewEventBus(watch))

	res, err := NewSessionService(watch).Setup(context.Background(), &SessionSetupRequest{
		Title: "测试", CharacterJSON: testCard, Player: Player{Name: "旅人"}, OpeningText: "开始",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	tr, err := turnSvc.Accept(context.Background(), res.Session.SessionID, res.Branch.BranchID, &TurnAcceptRequest{
		IdempotencyKey: "close-window", ExpectedHeadID: res.RootNode.NodeID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "等等"}, Mode: "structured",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	waitTurn(t, turnSvc, tr.TurnID, domain.TurnGenerating, domain.TurnCommitted)
	if _, err := turnSvc.Cancel(context.Background(), tr.TurnID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	turnSvc.Close() // 关闭顺序的第一半：排空 worker（含用量与终态写入）
	watch.shut.Store(true)
	if err := inner.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	time.Sleep(100 * time.Millisecond) // 给泄漏的 worker 一个真正动手的机会

	if n := watch.writes.Load(); n != 0 {
		t.Fatalf("存储关闭后仍有 %d 次写入：TurnService.Close 必须先排空 worker", n)
	}

	reopened, err := sqlite.Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	current, err := reopened.GetTurn(tr.TurnID)
	if err != nil {
		t.Fatalf("get turn: %v", err)
	}
	if current.Status != domain.TurnCancelled {
		t.Fatalf("终态未落盘：%s", current.Status)
	}
	attempts, err := reopened.ListAttempts(tr.TurnID)
	if err != nil || len(attempts) == 0 {
		t.Fatalf("attempts: %v %v", attempts, err)
	}
	if _, err := reopened.RecentTurnUsage(10); err != nil {
		t.Fatalf("用量台账在关闭后不可读: %v", err)
	}
}

// 未被 Close 的服务不得在测试结束后继续写库：这条不变量是所有 standalone
// 构造 TurnService 的用例必须注册 t.Cleanup(turnSvc.Close) 的原因。
func TestServiceCloseIsIdempotentAndStopsWrites(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	watch := &closeWatchStore{Store: st}
	turnSvc := NewTurnService(watch, mock.New(happyScript()), ctxpkg.New(watch, ctxpkg.DefaultOptions()), NewEventBus(watch))
	turnSvc.Close()
	turnSvc.Close()
	watch.shut.Store(true)
	time.Sleep(50 * time.Millisecond)
	if n := watch.writes.Load(); n != 0 {
		t.Fatalf("关闭后仍写入 %d 次", n)
	}
}
