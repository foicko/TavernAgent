package application

// 在途正文快照的测试替身：blockingProvider 卡在取消上，partialProvider 只写半行。
import (
	"context"
	"strings"
	"testing"
	"time"

	"tavernagent/internal/adapters/sqlite"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// blockingProvider 阻塞在 release/ctx 上，用于测试取消（T09 相关路径）。
type blockingProvider struct {
	release chan struct{}
}

func (b *blockingProvider) Capabilities(ctx context.Context) (ports.ProviderCapabilities, error) {
	return ports.ProviderCapabilities{ID: "blocking", Streaming: true}, nil
}

func (b *blockingProvider) Stream(ctx context.Context, req ports.ChatRequest, sink ports.StreamSink) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.release:
		return nil
	}
}

// partialProvider 只写出半行块帧就停住，用来固定"正在写这一段"的服务端状态。
type partialProvider struct {
	started chan struct{}
}

func (p *partialProvider) Capabilities(ctx context.Context) (ports.ProviderCapabilities, error) {
	return ports.ProviderCapabilities{ID: "partial", Streaming: true}, nil
}

func (p *partialProvider) Stream(ctx context.Context, req ports.ChatRequest, sink ports.StreamSink) error {
	if err := sink.Chunk([]byte(`{"v":1,"seq":1,"type":"block","kind":"narration","text":"潮水漫过甲板，远处`)); err != nil {
		return err
	}
	close(p.started)
	<-ctx.Done()
	return ctx.Err()
}

// 断线/刷新后的客户端靠 PartialDraft 补齐当前块：已完成的块有 durable 回放，
// 未完成块的行内增量（Sequence 0）永不重放，所以这里必须给出服务端权威文本。
// 回合结束后必须立刻收回，不能把上一回合的残文留在内存里。
func TestPartialDraftExposesInFlightBlock(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	prov := &partialProvider{started: make(chan struct{})}
	turnSvc := NewTurnService(st, prov, ctxpkg.New(st, ctxpkg.DefaultOptions()), NewEventBus(st))
	t.Cleanup(turnSvc.Close)

	res, err := NewSessionService(st).Setup(context.Background(), &SessionSetupRequest{
		Title: "测试", CharacterJSON: testCard, Player: Player{Name: "旅人"}, OpeningText: "开始",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	tr, err := turnSvc.Accept(context.Background(), res.Session.SessionID, res.Branch.BranchID, &TurnAcceptRequest{
		IdempotencyKey: "partial", ExpectedHeadID: res.RootNode.NodeID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "继续"}, Mode: "structured",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	select {
	case <-prov.started:
	case <-time.After(asyncWaitBudget):
		t.Fatal("provider 未开始输出")
	}

	var partial *PartialTurn
	deadline := time.Now().Add(asyncWaitBudget)
	for time.Now().Before(deadline) {
		if p, ok := turnSvc.PartialDraft(tr.TurnID); ok {
			partial = p
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if partial == nil {
		t.Fatal("在途回合应能取到部分正文")
	}
	if partial.AttemptID == "" {
		t.Fatal("部分正文必须带尝试 ID")
	}
	if partial.InFlight == nil || partial.InFlight.Seq != 1 || partial.InFlight.Kind != "narration" {
		t.Fatalf("在途块 = %+v", partial.InFlight)
	}
	if !strings.Contains(partial.InFlight.Text, "潮水漫过甲板") {
		t.Fatalf("在途正文 = %q", partial.InFlight.Text)
	}

	if _, err := turnSvc.Cancel(context.Background(), tr.TurnID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	waitTurn(t, turnSvc, tr.TurnID, domain.TurnCancelled)
	deadline = time.Now().Add(asyncWaitBudget)
	for time.Now().Before(deadline) {
		if _, ok := turnSvc.PartialDraft(tr.TurnID); !ok {
			if _, stale := turnSvc.PartialDraft("turn_never_existed"); stale {
				t.Fatal("未知回合不应返回部分正文")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("回合结束后不应再暴露在途正文")
}
