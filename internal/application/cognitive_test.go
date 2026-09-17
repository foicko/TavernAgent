package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"testing"
	"time"
)

type cognitiveProviderFunc func(context.Context, ports.ChatRequest, ports.StreamSink) error

func (f cognitiveProviderFunc) Capabilities(context.Context) (ports.ProviderCapabilities, error) {
	return ports.ProviderCapabilities{}, nil
}
func (f cognitiveProviderFunc) Stream(ctx context.Context, r ports.ChatRequest, sink ports.StreamSink) error {
	return f(ctx, r, sink)
}

func TestCognitiveCommitEvidenceReplayAndIsolation(t *testing.T) {
	st, turns, _, sid, bid, root := newTestServices(t, happyScript())
	tr, err := turns.Accept(context.Background(), sid, bid, &TurnAcceptRequest{IdempotencyKey: "cog-source", ExpectedHeadID: root, Input: domain.TurnInput{Kind: "text", Text: "玩家拔出了腰间的佩剑"}})
	if err != nil {
		t.Fatal(err)
	}
	tr = waitTurn(t, turns, tr.TurnID, domain.TurnCommitted)
	before, _ := st.StateAt(tr.ResultNodeID)
	if err := st.CreateBranch(&domain.Branch{BranchID: "sibling", SessionID: sid, HeadNodeID: tr.ResultNodeID, Name: "sibling"}); err != nil {
		t.Fatal(err)
	}
	p := cognitiveProviderFunc(func(_ context.Context, req ports.ChatRequest, sink ports.StreamSink) error {
		if !strings.Contains(req.Messages[1].Content, "玩家拔出了腰间的佩剑") {
			t.Error("missing verbatim source")
		}
		raw, _ := json.Marshal(domain.CognitivePlan{PlanID: "test", TurnID: tr.TurnID,
			WriteObserved:      []domain.CognitiveMemory{{Content: "玩家在酒馆出鞘佩剑", EntityIDs: []string{"player"}, SourceQuote: "拔出了腰间的佩剑", Confidence: "high"}},
			InferBelief:        []domain.CognitiveMemory{{Content: "玩家可能有戒心", Reasoning: "保留随时战斗的准备", Confidence: "high"}},
			AdjustRelationship: []domain.CognitiveRelationship{{CharacterID: "npc_elena", Dimension: domain.FieldAlertness, Delta: 2, Reason: "看见玩家亮剑", SourceQuote: "拔出了腰间的佩剑"}},
		})
		return sink.Chunk(raw)
	})
	svc := NewCognitiveService(st, func() (ports.ModelProvider, error) { return p, nil })
	defer svc.Close()
	result, err := svc.RunOnce(context.Background(), sid, bid, tr.ResultNodeID, tr.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Written != 2 || result.NodeID == tr.ResultNodeID {
		t.Fatalf("result=%+v", result)
	}
	mems, _ := pathMemories(st, result.NodeID)
	if len(mems) != 2 {
		t.Fatalf("memories=%+v", mems)
	}
	for _, m := range mems {
		if m.Evidence == nil {
			t.Fatal("missing evidence")
		}
		if m.Kind == domain.MemoryInferred && (m.Evidence.Confidence != "low" || !m.Evidence.AutoDowngraded) {
			t.Fatal("unquoted belief not downgraded")
		}
	}
	after, _ := st.StateAt(result.NodeID)
	a, _ := domain.UnmarshalWorld(after.StateJSON)
	b, _ := domain.UnmarshalWorld(before.StateJSON)
	if a.Relationships["npc_elena"].Alertness != b.Relationships["npc_elena"].Alertness+2 {
		t.Fatal("relationship not applied")
	}
	original, _ := st.StateAt(tr.ResultNodeID)
	if original.StateHash != before.StateHash {
		t.Fatal("mutated old world state")
	}
	if old, _ := pathMemories(st, tr.ResultNodeID); len(old) != 0 {
		t.Fatal("future memory leaked to shared ancestor")
	}
	again, err := svc.RunOnce(context.Background(), sid, bid, tr.ResultNodeID, tr.TurnID)
	if err != nil || !again.AlreadyDone {
		t.Fatalf("duplicate=%+v err=%v", again, err)
	}
	events, _ := st.GetEvents(result.NodeID)
	replay, _ := domain.UnmarshalWorld(before.StateJSON)
	if _, err := domain.ApplyEvents(replay, events); err != nil || replay.HashID() != after.StateHash {
		t.Fatalf("replay diverged: %v", err)
	}
}

func TestCognitiveRejectsUnprovenAndCancelledPlans(t *testing.T) {
	for _, scenario := range []string{"fabricated", "no reasoning", "cancelled", "wrong turn", "unknown fields"} {
		t.Run(scenario, func(t *testing.T) {
			st, turns, _, sid, bid, root := newTestServices(t, happyScript())
			tr, err := turns.Accept(context.Background(), sid, bid, &TurnAcceptRequest{IdempotencyKey: "source", ExpectedHeadID: root, Input: domain.TurnInput{Kind: "text", Text: "继续故事"}})
			if err != nil {
				t.Fatal(err)
			}
			tr = waitTurn(t, turns, tr.TurnID, domain.TurnCommitted)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := cognitiveProviderFunc(func(_ context.Context, _ ports.ChatRequest, sink ports.StreamSink) error {
				plan := domain.CognitivePlan{PlanID: "p", TurnID: tr.TurnID}
				switch scenario {
				case "fabricated":
					plan.WriteObserved = []domain.CognitiveMemory{{Content: "假事实", SourceQuote: "完全不存在的台词", Confidence: "high"}}
				case "no reasoning":
					plan.InferBelief = []domain.CognitiveMemory{{Content: "她想伤害玩家", Confidence: "low"}}
				case "cancelled":
					cancel()
				case "wrong turn":
					plan.TurnID = "other"
				case "unknown fields":
					return sink.Chunk([]byte(`{"turnId":"` + tr.TurnID + `","executeShell":"bad"}`))
				}
				raw, _ := json.Marshal(plan)
				return sink.Chunk(raw)
			})
			svc := NewCognitiveService(st, func() (ports.ModelProvider, error) { return p, nil })
			defer svc.Close()
			if _, err := svc.RunOnce(ctx, sid, bid, tr.ResultNodeID, tr.TurnID); err == nil {
				t.Fatal("invalid plan accepted")
			}
			branch, _ := st.GetBranch(bid)
			if branch.HeadNodeID != tr.ResultNodeID {
				t.Fatal("rejected plan moved branch")
			}
			ms, _ := st.ListMemories(sid)
			if len(ms) != 0 {
				t.Fatal("rejected plan wrote memories")
			}
		})
	}
}

func TestCoalescingQueueCancelsAndRunsLatest(t *testing.T) {
	q := newCoalescingQueue(1, 0, true)
	defer q.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	var obsolete, latest atomic.Int32
	q.Submit("branch", func(ctx context.Context) {
		close(started)
		<-release
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Error("old task not cancelled")
		}
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker never started")
	}
	q.Submit("branch", func(context.Context) { obsolete.Add(1) })
	q.Submit("branch", func(context.Context) { latest.Add(1); close(done) })
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("latest task not run")
	}
	if obsolete.Load() != 0 || latest.Load() != 1 {
		t.Fatal("queue failed to coalesce")
	}
}

// 摘要维护使用"不取消在途"的合并模式：产物按不可变区间键控，晚到一样有效，
// 取消它只会让摘要永远完不成（实测 12 次调用 9 次被取消）。
// 这个用例与 TestCoalescingQueueCancelsAndRunsLatest 成对：两种语义都要被钉住。
func TestCoalescingQueueWithoutCancelLetsRunningWorkFinish(t *testing.T) {
	q := newCoalescingQueue(1, 0, false)
	defer q.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	runningCtxErr := make(chan error, 1)
	done := make(chan struct{})
	var latest atomic.Int32
	q.Submit("branch", func(ctx context.Context) {
		close(started)
		<-release
		runningCtxErr <- ctx.Err()
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker never started")
	}
	q.Submit("branch", func(context.Context) { latest.Add(1); close(done) })
	close(release)
	if err := <-runningCtxErr; err != nil {
		t.Fatalf("在途任务被取消：%v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("最新任务未在在途任务之后运行")
	}
	if latest.Load() != 1 {
		t.Fatalf("pending 合并后应只跑最新一次，实际 %d 次", latest.Load())
	}
}

// 关停语义：队列关闭时未开跑的 pending 任务必须被丢弃，而且这件事必须
// 可观测（计数 + 日志）。旧实现静默丢弃——那一轮的记忆抽取就永远消失了，
// 运维在读数里看不到任何痕迹。
func TestCoalescingQueueCloseCountsDroppedPending(t *testing.T) {
	metrics := &RuntimeMetrics{}
	q := newCoalescingQueue(1, 0, true)
	q.metrics = metrics
	started := make(chan struct{})
	release := make(chan struct{})
	q.Submit("branch", func(context.Context) { close(started); <-release })
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker never started")
	}
	q.Submit("branch", func(context.Context) { t.Error("pending 不应在关闭后运行") })
	close(release)
	q.Close()

	if got := metrics.Snapshot().QueueDropped; got != 1 {
		t.Fatalf("丢弃计数 = %d，期望 1", got)
	}
}

// 关闭之后仍然 Submit：不执行、计数、且不阻塞调用方。
func TestCoalescingQueueRejectsSubmitAfterClose(t *testing.T) {
	metrics := &RuntimeMetrics{}
	q := newCoalescingQueue(1, 0, true)
	q.metrics = metrics
	q.Close()
	ran := make(chan struct{}, 1)
	q.Submit("branch", func(context.Context) { ran <- struct{}{} })
	select {
	case <-ran:
		t.Fatal("关闭后提交的任务被执行")
	case <-time.After(50 * time.Millisecond):
	}
	if got := metrics.Snapshot().QueueRejected; got != 1 {
		t.Fatalf("拒绝计数 = %d，期望 1", got)
	}
}

// Close 必须等在途任务返回，否则任务会活过数据库关闭（sql: database is closed）。
func TestCoalescingQueueCloseWaitsForRunningWork(t *testing.T) {
	q := newCoalescingQueue(1, 0, true)
	finished := make(chan struct{})
	q.Submit("branch", func(ctx context.Context) {
		<-ctx.Done()
		time.Sleep(30 * time.Millisecond)
		close(finished)
	})
	time.Sleep(20 * time.Millisecond)
	q.Close()
	select {
	case <-finished:
	default:
		t.Fatal("Close 未等在途任务返回")
	}
}
