package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// A complete, deterministic story exercises the real SQLite transaction, replay,
// memory, compaction and archive paths. No external provider or user DB is used.
func TestQualityStoryMemoryInventoryStatusCompressionAndRecovery(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	script := []mock.Item{
		mock.Frame(mockBlock(1, "narration", "艾莲娜记下旅人喝了两份茶。她来自北方，约好明晚在钟楼见面。")),
		mock.Frame(mockFinal(2, strings.Join([]string{
			`{"proposalId":"rel","type":"relationship_delta","characterId":"npc_elena","field":"trust","delta":4}`,
			`{"proposalId":"mood","type":"mood_set","characterId":"npc_elena","moodCode":"calm","text":"放心地等待明晚的约定"}`,
			`{"proposalId":"goal","type":"goal_set","characterId":"npc_elena","text":"明晚去钟楼见旅人"}`,
			`{"proposalId":"mem","type":"memory_add","text":"艾莲娜来自北方","sourceQuote":"她来自北方","memoryKind":"observed","evidenceConfidence":"high","entityIds":["npc_elena"]}`,
			`{"proposalId":"promise","type":"promise_propose","text":"明晚在钟楼见面","participants":["player","npc_elena"]}`,
		}, ","), "")),
	}
	newTurns := func() *TurnService {
		s := NewTurnService(st, mock.New(script), ctxpkg.New(st, ctxpkg.DefaultOptions()), NewEventBus(st))
		s.SetRandom(&detRand{values: []int{13}})
		return s
	}
	turns := newTurns()
	t.Cleanup(func() { turns.Close() })
	card, _ := ParseCharacterCard(testCard)
	card.Items = append(card.Items, domain.ItemInstance{InstanceID: "tea", Name: "旅行茶", OwnerID: "player", Quantity: 5})
	card.Rules = &domain.Ruleset{Version: "quality.rules.v1", Actions: map[string]domain.ActionRule{
		"drink.tea": {ActionID: "drink.tea", Label: "饮用两份茶", Attribute: "wisdom", DC: 12,
			Consequences: map[domain.CheckOutcome][]domain.RuleEffect{domain.OutcomeSuccess: {
				{Type: domain.EventItemConsume, Payload: json.RawMessage(`{"itemId":"tea","from":"player","to":"consumed","quantity":2}`)},
			}}},
	}}
	raw, _ := json.Marshal(card)
	setup, err := NewSessionService(st).Setup(ctx, &SessionSetupRequest{CharacterJSON: string(raw), Player: Player{Name: "林舟", Role: "游历者"}, OpeningText: "你带着五份旅行茶和怀表走进酒馆。"})
	if err != nil {
		t.Fatal(err)
	}
	sid, bid, root := setup.Session.SessionID, setup.Branch.BranchID, setup.RootNode.NodeID
	view, err := NewSessionService(st).View(sid, ViewQuery{BranchID: bid})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Actions) != 1 || view.Actions[0].ActionID != "drink.tea" || len(view.Actions[0].ItemIDs) != 1 || view.Actions[0].ItemIDs[0] != "tea" {
		t.Fatalf("inventory rule action missing from public view: %+v", view.Actions)
	}
	stateAt := func(node string) *domain.WorldState {
		t.Helper()
		sn, e := st.StateAt(node)
		if e != nil {
			t.Fatal(e)
		}
		state, e := domain.UnmarshalWorld(sn.StateJSON)
		if e != nil || !state.MatchesHash(sn.StateHash) {
			t.Fatalf("invalid persisted state: %v", e)
		}
		return state
	}
	req := &TurnAcceptRequest{IdempotencyKey: "drink-once", ExpectedHeadID: root,
		Input: domain.TurnInput{Kind: "action", Text: "我喝两份茶，问她来自哪里。", ActionRef: "drink.tea"}}
	first, err := turns.Accept(ctx, sid, bid, req)
	if err != nil {
		t.Fatal(err)
	}
	first = waitTurn(t, turns, first.TurnID, domain.TurnCommitted)
	changed := stateAt(first.ResultNodeID)
	if changed.Items["tea"].Quantity != 3 || changed.Items["tea"].OwnerID != "player" || changed.Items["item_pocketwatch"].Quantity != 1 {
		t.Fatalf("inventory after drink: %+v", changed.Items)
	}
	if changed.Relationships["npc_elena"].Trust != 4 || changed.Moods["npc_elena"].MoodCode != "calm" || changed.Goals["goal_npc_elena"].Text != "明晚去钟楼见旅人" || len(changed.Promises) != 1 {
		t.Fatalf("character state lost: %s", mustJSON(changed))
	}
	if again, err := turns.Accept(ctx, sid, bid, req); err != nil || again.TurnID != first.TurnID || stateAt(first.ResultNodeID).Items["tea"].Quantity != 3 {
		t.Fatalf("idempotent action consumed twice: %v", err)
	}
	branches := NewBranchService(st, turns)
	beforeDrink, err := branches.Fork(ctx, sid, ForkRequest{FromNodeID: root, Name: "饮茶之前"})
	if err != nil {
		t.Fatal(err)
	}
	uncorrected, err := branches.Fork(ctx, sid, ForkRequest{FromNodeID: first.ResultNodeID, Name: "未修订的记忆"})
	if err != nil {
		t.Fatal(err)
	}
	mem := NewMemoryService(st)
	initial, _, err := mem.ListAt(sid, bid, "")
	if err != nil || len(initial) != 1 || initial[0].Evidence == nil || initial[0].Evidence.SourceNodeID != first.ResultNodeID {
		t.Fatalf("sourced memory missing: %+v %v", initial, err)
	}
	correctedText, pinned := "艾莲娜来自南方的港口", true
	patch := MemoryPatch{Content: &correctedText, Pinned: &pinned, IdempotencyKey: "correct-once"}
	corrected, err := mem.Overlay(sid, bid, initial[0].MemoryID, patch)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := mem.Overlay(sid, bid, initial[0].MemoryID, patch); err != nil || again.MemoryID != corrected.MemoryID {
		t.Fatalf("correction retry duplicated memory: %v", err)
	}
	for _, hidden := range []bool{true, false} {
		if _, err := mem.Overlay(sid, bid, initial[0].MemoryID, MemoryPatch{Hidden: &hidden}); err != nil {
			t.Fatal(err)
		}
		got, _, err := mem.ListAt(sid, bid, "")
		if err != nil || len(got) != 1 || got[0].Hidden != hidden || got[0].Content != correctedText || !got[0].Pinned {
			t.Fatalf("hide/restore failed: %+v %v", got, err)
		}
	}
	if _, err := mem.Organize(ctx, sid, bid, card.CardID); err != nil {
		t.Fatal(err)
	}
	original, _, err := mem.ListAt(sid, uncorrected.BranchID, "")
	if err != nil || len(original) != 1 || original[0].Content != "艾莲娜来自北方" {
		t.Fatalf("correction changed sibling history: %+v %v", original, err)
	}
	empty, _, err := mem.ListAt(sid, beforeDrink.BranchID, "")
	if err != nil || len(empty) != 0 || stateAt(beforeDrink.HeadNodeID).Items["tea"].Quantity != 5 {
		t.Fatalf("pre-action branch did not retain its state: %v", err)
	}
	head := ""
	for i := 1; i <= 8; i++ {
		b, _ := st.GetBranch(bid)
		n, _, e := turns.SubmitBlocks(ctx, SubmitBlocksRequest{SessionID: sid, BranchID: bid, HeadID: b.HeadNodeID, Version: b.Version,
			Input: domain.TurnInput{Kind: "text", Text: "继续旅途"}, Blocks: []domain.TextBlock{{Kind: "narration", Text: fmt.Sprintf("旅途第%d段：窗外雨声渐渐停歇。", i)}}})
		if e != nil {
			t.Fatal(e)
		}
		head = n.NodeID
	}
	beforeCompression := stateAt(head).HashID()
	reflectionCalls := 0
	reflection := cognitiveProviderFunc(func(_ context.Context, r ports.ChatRequest, sink ports.StreamSink) error {
		reflectionCalls++
		if !strings.Contains(r.Messages[1].Content, correctedText) || ctxpkg.MessageTokens(r.Messages) > r.InputBudget {
			t.Error("reflection omitted the correction or exceeded its input budget")
		}
		return sink.Chunk([]byte(`<story_checkpoint><narrative_arc>艾莲娜与旅人谈起南方港口，雨夜逐渐过去。</narrative_arc><open_loops>明晚在钟楼见面。</open_loops></story_checkpoint>`))
	})
	cfg := fakeStoreWithSlot("reflection", ports.ProviderConfig{Slot: "reflection", Enabled: true, Kind: "mock", ContextWindow: 8192, MaxTokens: 1024})
	manager := NewProviderManager(cfg, func(ports.ProviderConfig) (ports.ModelProvider, error) { return reflection, nil }, nil)
	if err := manager.Reload(); err != nil {
		t.Fatal(err)
	}
	policy := ctxpkg.CompactionPolicy{TailWindowTurns: 2, MinUncompactedTurns: 2}
	compactor := NewCompactorService(st, manager, nil, policy)
	defer compactor.Close()
	artifact, err := compactor.RunOnce(ctx, sid, bid, head)
	if err != nil || artifact == nil || reflectionCalls != 1 || artifact.SourceHash == "" {
		t.Fatalf("compaction failed: %+v %v, calls=%d", artifact, err, reflectionCalls)
	}
	if stateAt(head).HashID() != beforeCompression {
		t.Fatal("compression modified authoritative state")
	}
	opts := ctxpkg.DefaultOptions()
	opts.CompactionPolicy = policy
	compiled, err := ctxpkg.New(st, opts).WithBudget(8192, 1024).Compile(ctx, sid, head, "艾莲娜来自哪里？我还有多少旅行茶？", stateAt(head), nil)
	if err != nil {
		t.Fatal(err)
	}
	var prompt strings.Builder
	for _, message := range compiled.Messages {
		prompt.WriteString(message.Content)
	}
	for _, required := range []string{"story_checkpoint", "旅途第8段", correctedText, "旅行茶 (3份", "怀表", "明晚在钟楼见面", "放心地等待明晚的约定"} {
		if !strings.Contains(prompt.String(), required) {
			t.Errorf("compiled context omitted %q", required)
		}
	}
	if strings.Contains(prompt.String(), "旅途第1段") || ctxpkg.MessageTokens(compiled.Messages) > compiled.InputBudget {
		t.Fatal("compressed history was duplicated or input budget was exceeded")
	}
	turns.Close()
	compactor.Close()
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = sqlite.Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	turns = newTurns()
	if stateAt(head).HashID() != beforeCompression {
		t.Fatal("restart changed the story state")
	}
	archive := NewArchiveService(st, "quality-test")
	exported, err := archive.Export(ctx, sid, "")
	if err != nil {
		t.Fatal(err)
	}
	imported, err := archive.Import(ctx, exported.Data)
	if err != nil {
		t.Fatal(err)
	}
	importBranches, err := st.ListBranches(imported.Session.SessionID)
	if err != nil || len(importBranches) != 3 {
		t.Fatalf("archive lost branches: %v", err)
	}
	var main *domain.Branch
	for _, branch := range importBranches {
		if branch.Name == "main" {
			main = branch
		}
	}
	if main == nil {
		t.Fatal("archive lost main branch")
	}
	importState := stateAt(main.HeadNodeID)
	// Provenance IDs are intentionally remapped. Compare state after normalizing
	// those references, then independently verify they point to imported nodes.
	for key, promise := range importState.Promises {
		node, err := st.GetNode(promise.SourceNodeID)
		if err != nil || node.SessionID != imported.Session.SessionID {
			t.Fatalf("promise has a dangling source: %v", err)
		}
		promise.SourceNodeID = changed.Promises[key].SourceNodeID
		importState.Promises[key] = promise
	}
	if importState.HashID() != beforeCompression {
		t.Fatal("archive changed inventory or character/promise state")
	}
	importMemories, _, err := NewMemoryService(st).ListAt(imported.Session.SessionID, main.BranchID, "")
	if err != nil || len(importMemories) != 1 || importMemories[0].Content != correctedText || !importMemories[0].Pinned {
		t.Fatalf("archive lost effective memory: %+v %v", importMemories, err)
	}
	continued, err := turns.Accept(ctx, imported.Session.SessionID, main.BranchID, &TurnAcceptRequest{IdempotencyKey: "after-import", ExpectedHeadID: main.HeadNodeID, ExpectedVersion: main.Version,
		Input: domain.TurnInput{Kind: "action", Text: "我再喝两份茶。", ActionRef: "drink.tea"}})
	if err != nil {
		t.Fatal(err)
	}
	continued = waitTurn(t, turns, continued.TurnID, domain.TurnCommitted)
	if stateAt(continued.ResultNodeID).Items["tea"].Quantity != 1 || stateAt(head).Items["tea"].Quantity != 3 {
		t.Fatal("continuing an imported story corrupted the original inventory")
	}
}
