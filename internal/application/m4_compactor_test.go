package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"tavernagent/internal/adapters/providers/mock"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

const m4Summary = "<story_checkpoint><narrative_arc>她解释了自己的故乡。</narrative_arc><open_loops>等待继续交谈。</open_loops></story_checkpoint>"

func TestM4SummaryWaitsForCorrectionAndHashesItsSource(t *testing.T) {
	st, turns, _, sid, bid, _ := newTestServices(t, []mock.Item{
		mock.Frame(mockBlock(1, "narration", "她说自己来自北方。")),
		mock.Frame(mockFinal(2, "{\"proposalId\":\"m\",\"type\":\"memory_add\",\"memoryKind\":\"observed\",\"text\":\"她来自北方\",\"sourceQuote\":\"自己来自北方\",\"evidenceConfidence\":\"high\"}", "")),
	})
	first := acceptAndWait(t, turns, st, sid, bid, "first", "你来自哪里")
	addTurn := func() string {
		t.Helper()
		b, _ := st.GetBranch(bid)
		n, _, e := turns.SubmitBlocks(context.Background(), SubmitBlocksRequest{SessionID: sid, BranchID: bid, HeadID: b.HeadNodeID, Version: b.Version, Input: domain.TurnInput{Kind: "text", Text: "继续交谈"}, Blocks: []domain.TextBlock{{Kind: "narration", Text: "她继续聊着旅途。"}}})
		if e != nil {
			t.Fatal(e)
		}
		return n.NodeID
	}
	oldHead := ""
	for i := 0; i < 7; i++ {
		oldHead = addTurn()
	}
	if err := st.SaveSummary(&domain.SummaryArtifact{SummaryID: "old", FromNodeID: first.ResultNodeID, ToNodeID: oldHead, Text: "旧摘要说她来自北方"}); err != nil {
		t.Fatal(err)
	}
	memories, _ := st.ListMemories(sid)
	content := "她来自南方"
	corrected, err := NewMemoryService(st).Overlay(sid, bid, memories[0].MemoryID, MemoryPatch{Content: &content})
	if err != nil {
		t.Fatal(err)
	}
	if sums, _ := st.SummariesOnPath(corrected.SourceNodeID); len(sums) != 0 {
		t.Fatal("stale summary survived correction")
	}
	if sums, _ := st.SummariesOnPath(oldHead); len(sums) != 1 {
		t.Fatal("correction affected historical summary")
	}
	calls := 0
	p := cognitiveProviderFunc(func(_ context.Context, req ports.ChatRequest, sink ports.StreamSink) error {
		calls++
		if !strings.Contains(req.Messages[1].Content, content) {
			t.Error("summary omitted correction")
		}
		if ctxpkg.MessageTokens(req.Messages) > req.InputBudget {
			t.Error("summary exceeded provider budget")
		}
		return sink.Chunk([]byte(m4Summary))
	})
	cfg := fakeStoreWithSlot("reflection", ports.ProviderConfig{Slot: "reflection", Enabled: true, Kind: "mock", ContextWindow: 8192, MaxTokens: 1024})
	manager := NewProviderManager(cfg, func(ports.ProviderConfig) (ports.ModelProvider, error) { return p, nil }, nil)
	if err := manager.Reload(); err != nil {
		t.Fatal(err)
	}
	c := NewCompactorService(st, manager, nil, ctxpkg.CompactionPolicy{TailWindowTurns: 1, MinUncompactedTurns: 1})
	defer c.Close()
	if art, err := c.RunOnce(context.Background(), sid, bid, corrected.SourceNodeID); err != nil || art != nil || calls != 0 {
		t.Fatalf("immediately stale summary generated: %+v %v %d", art, err, calls)
	}
	addTurn()
	head := addTurn()
	art, err := c.RunOnce(context.Background(), sid, bid, head)
	if err != nil || art == nil {
		t.Fatalf("correction-ready compaction failed: %+v %v", art, err)
	}
	chain, _ := st.AncestorChain(art.ToNodeID, true)
	var sources []*domain.PlotNode
	collect := false
	for _, n := range chain {
		if n.NodeID == art.FromNodeID {
			collect = true
		}
		if collect {
			sources = append(sources, n)
		}
	}
	if art.SourceHash != domain.SummarySourceHash(sources) {
		t.Fatal("hash did not cover the exact correction interval")
	}
	if sums, _ := st.SummariesOnPath(head); len(sums) != 1 {
		t.Fatal("corrected summary was not adopted")
	}
}

func TestM4CompactorPressureSurvivesNormalTrigger(t *testing.T) {
	st, turns, _, sid, bid, _ := newTestServices(t, happyScript())
	head := ""
	for i := 0; i < 6; i++ {
		b, _ := st.GetBranch(bid)
		n, _, err := turns.SubmitBlocks(context.Background(), SubmitBlocksRequest{SessionID: sid, BranchID: bid, HeadID: b.HeadNodeID, Version: b.Version, Input: domain.TurnInput{Kind: "text", Text: "闲聊"}, Blocks: []domain.TextBlock{{Kind: "narration", Text: "聊起往事"}}})
		if err != nil {
			t.Fatal(err)
		}
		head = n.NodeID
	}
	p := mock.New([]mock.Item{{Line: m4Summary}})
	cfg := fakeStoreWithSlot("reflection", ports.ProviderConfig{Slot: "reflection", Enabled: true, Kind: "mock"})
	manager := NewProviderManager(cfg, func(ports.ProviderConfig) (ports.ModelProvider, error) { return p, nil }, nil)
	if err := manager.Reload(); err != nil {
		t.Fatal(err)
	}
	c := NewCompactorService(st, manager, nil, ctxpkg.DefaultCompactionPolicy())
	defer c.Close()
	c.TriggerPressure(sid, bid, head, 4)
	c.TriggerAsync(sid, bid, head)
	deadline := time.Now().Add(asyncWaitBudget)
	for time.Now().Before(deadline) {
		sums, _ := st.SummariesOnPath(head)
		if len(sums) > 0 {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("normal trigger erased pressure request")
}

func TestM4CompactorCancellationDoesNotPublish(t *testing.T) {
	st, turns, _, sid, bid, _ := newTestServices(t, happyScript())
	head := ""
	for i := 0; i < 3; i++ {
		head = acceptAndWait(t, turns, st, sid, bid, "turn"+string(rune('a'+i)), "你好").ResultNodeID
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := cognitiveProviderFunc(func(_ context.Context, _ ports.ChatRequest, sink ports.StreamSink) error {
		cancel()
		return sink.Chunk([]byte(m4Summary))
	})
	cfg := fakeStoreWithSlot("reflection", ports.ProviderConfig{Slot: "reflection", Enabled: true, Kind: "mock"})
	manager := NewProviderManager(cfg, func(ports.ProviderConfig) (ports.ModelProvider, error) { return p, nil }, nil)
	if err := manager.Reload(); err != nil {
		t.Fatal(err)
	}
	c := NewCompactorService(st, manager, nil, ctxpkg.CompactionPolicy{TailWindowTurns: 1, MinUncompactedTurns: 1})
	defer c.Close()
	if _, err := c.RunOnce(ctx, sid, bid, head); err == nil {
		t.Fatal("cancelled summary accepted")
	}
	if sums, _ := st.SummariesOnPath(head); len(sums) != 0 {
		t.Fatal("cancelled summary saved")
	}
}
