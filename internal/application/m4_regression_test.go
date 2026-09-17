package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"tavernagent/internal/adapters/providers/mock"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

func TestM4OptionReplayAfterCommitAndIntentConflict(t *testing.T) {
	st, svc, _, sid, bid, _ := newTestServices(t, optionScript("check.strength"))
	first := acceptAndWait(t, svc, st, sid, bid, "first", "我走进酒馆")
	br, _ := st.GetBranch(bid)
	req := TurnAcceptRequest{IdempotencyKey: "choice", ExpectedHeadID: br.HeadNodeID, ExpectedVersion: br.Version,
		Input: domain.TurnInput{Kind: "option", Text: "我走过去。", OptionRef: &domain.OptionRef{NodeID: first.ResultNodeID, OptionID: "o1"}}}
	tr, err := svc.Accept(context.Background(), sid, bid, &req)
	if err != nil {
		t.Fatal(err)
	}
	waitTurn(t, svc, tr.TurnID, domain.TurnCommitted)
	replay, err := svc.Accept(context.Background(), sid, bid, &req)
	if err != nil || replay.TurnID != tr.TurnID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	req.Mode = "narrative"
	if _, err := svc.Accept(context.Background(), sid, bid, &req); err == nil {
		t.Fatal("different mode reused an old request")
	}
}

type failingIdempotencyStore struct{ ports.Store }

func (f failingIdempotencyStore) FindTurnByIdempotency(string, string) (*domain.TurnRequest, error) {
	return nil, errors.New("injected read failure")
}

func TestM4IdempotencyReadFailureNeverStartsTurn(t *testing.T) {
	st, svc, _, sid, bid, root := newTestServices(t, happyScript())
	svc.store = failingIdempotencyStore{st}
	_, err := svc.Accept(context.Background(), sid, bid, &TurnAcceptRequest{IdempotencyKey: "read-fail", ExpectedHeadID: root, Input: domain.TurnInput{Kind: "text", Text: "你好"}})
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "STORAGE_UNAVAILABLE" {
		t.Fatalf("err=%v", err)
	}
	b, _ := st.GetBranch(bid)
	if b.ActiveTurnID != "" || b.HeadNodeID != root {
		t.Fatal("failed lookup mutated the branch")
	}
}

func TestM4NativeRulesRecheckDeriveAndPack(t *testing.T) {
	st, turns, sessions, _, _, _ := newTestServices(t, optionScript(""))
	var card map[string]any
	if err := json.Unmarshal([]byte(testCard), &card); err != nil {
		t.Fatal(err)
	}
	card["cardId"] = "native_m4_keeper"
	rules := domain.Ruleset{Version: "keeper.rules.v2", Actions: map[string]domain.ActionRule{
		"gate.open": {ActionID: "gate.open", Label: "抬起铁门", Attribute: "strength", DC: 12,
			Consequences:     map[domain.CheckOutcome][]domain.RuleEffect{domain.OutcomeSuccess: {{Type: domain.EventMilestonePropose, Payload: json.RawMessage("{\"milestoneId\":\"gate-open\",\"description\":\"铁门已开启\"}")}}},
			PermanentEffects: map[domain.CheckOutcome]string{domain.OutcomeFailure: "右肩留下伤痕"}},
	}}
	card["rules"] = rules
	raw, _ := json.Marshal(card)
	setup, err := sessions.Setup(context.Background(), &SessionSetupRequest{CharacterJSON: string(raw), Player: Player{Name: "旅人"}, OpeningText: "铁门挡住了山路。"})
	if err != nil {
		t.Fatal(err)
	}
	sid, bid := setup.Session.SessionID, setup.Branch.BranchID
	if setup.Session.CharacterID != "native_m4_keeper" || setup.Session.RulesetVersion != rules.Version {
		t.Fatalf("session=%+v", setup.Session)
	}
	rng := &detRand{values: []int{13, 4}}
	turns.SetRandom(rng)
	tr, err := turns.Accept(context.Background(), sid, bid, &TurnAcceptRequest{IdempotencyKey: "gate", ExpectedHeadID: setup.RootNode.NodeID, ExpectedCharacterID: "native_m4_keeper", Input: domain.TurnInput{Kind: "action", Text: "我抬起铁门", ActionRef: "gate.open"}})
	if err != nil {
		t.Fatal(err)
	}
	tr = waitTurn(t, turns, tr.TurnID, domain.TurnCommitted)
	checkOf := func(nodeID string) domain.CheckResult {
		t.Helper()
		n, e := st.GetNode(nodeID)
		if e != nil {
			t.Fatal(e)
		}
		var tc domain.TurnContent
		if json.Unmarshal([]byte(n.ContentJSON), &tc) != nil || len(tc.Checks) != 1 {
			t.Fatalf("missing checks: %s", n.ContentJSON)
		}
		return tc.Checks[0]
	}
	first := checkOf(tr.ResultNodeID)
	if first.Natural != 14 {
		t.Fatalf("first=%+v", first)
	}
	sn, _ := st.StateAt(tr.ResultNodeID)
	state, _ := domain.UnmarshalWorld(sn.StateJSON)
	if state.Milestones["gate-open"] != "铁门已开启" {
		t.Fatal("backend rule effect not applied")
	}
	branches := NewBranchService(st, turns)
	derive := func(node, key string, recheck bool) *DeriveResult {
		t.Helper()
		res, e := branches.DeriveTurn(context.Background(), sid, DeriveRequest{NodeID: node, IdempotencyKey: key, Recheck: recheck, ExpectedCharacterID: "native_m4_keeper"})
		if e != nil {
			t.Fatal(e)
		}
		res.Turn = waitTurn(t, turns, res.Turn.TurnID, domain.TurnCommitted)
		return res
	}
	plain := derive(tr.ResultNodeID, "plain", false)
	if got := checkOf(plain.Turn.ResultNodeID); got.RollID != first.RollID || rng.i != 1 {
		t.Fatalf("plain rerolled: %+v", got)
	}
	fresh := derive(tr.ResultNodeID, "fresh", true)
	second := checkOf(fresh.Turn.ResultNodeID)
	if second.RollID == first.RollID || second.Natural != 5 || second.PermanentEffect == "" || rng.i != 2 {
		t.Fatalf("fresh=%+v draws=%d", second, rng.i)
	}
	after := derive(fresh.Turn.ResultNodeID, "after-fresh", false)
	if got := checkOf(after.Turn.ResultNodeID); got.RollID != second.RollID || rng.i != 2 {
		t.Fatalf("regenerating a recheck lost its ruling: %+v", got)
	}
	before, _ := st.ListBranches(sid)
	again, err := branches.DeriveTurn(context.Background(), sid, DeriveRequest{NodeID: fresh.Turn.ResultNodeID, IdempotencyKey: "after-fresh"})
	if err != nil || again.Branch.BranchID != after.Branch.BranchID {
		t.Fatalf("derive replay=%+v %v", again, err)
	}
	now, _ := st.ListBranches(sid)
	if len(now) != len(before) {
		t.Fatal("idempotent derive left an orphan branch")
	}
	archive := NewArchiveService(st, "test-m4")
	exported, err := archive.Export(context.Background(), sid, "")
	if err != nil {
		t.Fatal(err)
	}
	imported, err := archive.Import(context.Background(), exported.Data)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Session.CharacterID != "native_m4_keeper" || imported.Session.RulesetVersion != rules.Version {
		t.Fatalf("import lost identity/rules: %+v", imported.Session)
	}
	bundle, err := st.ExportSession(imported.Session.SessionID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Receipts) != 4 {
		t.Fatalf("portable receipt count=%d", len(bundle.Receipts))
	}
	var rerolledNode string
	for _, p := range bundle.Receipts {
		cr, e := p.Receipt.CheckResultOf()
		if e != nil {
			t.Fatal(e)
		}
		if cr.RollID == first.RollID || cr.RollID == second.RollID {
			t.Fatal("roll reference was not remapped")
		}
		if cr.PermanentEffect != "" {
			rerolledNode = p.NodeID
		}
	}
	snap, _ := st.StateAt(rerolledNode)
	ws, _ := domain.UnmarshalWorld(snap.StateJSON)
	req, err := ctxpkg.New(st, ctxpkg.DefaultOptions()).Compile(context.Background(), imported.Session.SessionID, rerolledNode, "继续", ws, nil)
	if err != nil {
		t.Fatal(err)
	}
	all := ""
	for _, m := range req.Messages {
		all += m.Content
	}
	if !strings.Contains(all, "右肩留下伤痕") || !strings.Contains(all, "gate.open") {
		t.Fatal("imported ledger or native actions missing from compiler")
	}
}

func TestM4MemoryReplayReturnsSavedRecord(t *testing.T) {
	f := newMemoryFixture(t)
	head := f.commitOn(t, f.main.BranchID, "root", memRec("original", "她来自北方"))
	br, _ := f.store.GetBranch(f.main.BranchID)
	content := "她来自南方"
	p := MemoryPatch{Content: &content, ExpectedHeadID: head, ExpectedVersion: &br.Version, IdempotencyKey: "fix-1"}
	first, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "original", p)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "original", p)
	if err != nil || second.MemoryID != first.MemoryID || second.SourceNodeID != first.SourceNodeID {
		t.Fatalf("replay=%+v %v", second, err)
	}
	content = "另一个内容"
	if _, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "original", p); err == nil {
		t.Fatal("reused key accepted different patch")
	}
	node, _ := f.store.GetNode(first.SourceNodeID)
	if !strings.Contains(node.ContentJSON, "她来自南方") {
		t.Fatal("summary provenance lacks the actual correction")
	}
}

func TestM4OrganizerEvidenceRoundTripAndHistoricalIsolation(t *testing.T) {
	st, turns, _, sid, bid, _ := newTestServices(t, happyScript())
	tr := acceptAndWait(t, turns, st, sid, bid, "seed", "玩家拔出了腰间的佩剑")
	br, _ := st.GetBranch(bid)
	nodeID := "memory_seed"
	mems := make([]*domain.MemoryRecord, 0, 81)
	for i := 0; i < 80; i++ {
		mems = append(mems, &domain.MemoryRecord{MemoryID: fmt.Sprintf("m%03d", i), SourceNodeID: nodeID, Kind: domain.MemoryObserved, Content: fmt.Sprintf("雨夜里酒馆的红葡萄酒售价讨论第%d次", i), Importance: 3})
	}
	mems = append(mems, &domain.MemoryRecord{MemoryID: "protected", SourceNodeID: nodeID, Kind: domain.MemoryObserved, Content: "玩家拔出了腰间的佩剑", Pinned: true, Evidence: &domain.MemoryEvidence{SourceNodeID: tr.ResultNodeID, SourceQuote: "拔出了腰间的佩剑", Confidence: "high"}})
	if _, err := commitMemoryBatch(context.Background(), st, &ports.MemoryBatch{BatchID: "seed", SessionID: sid, BranchID: bid, ExpectedHeadID: br.HeadNodeID, ExpectedVersion: br.Version, NodeID: nodeID, PayloadHash: "seed", Memories: mems, Events: memoryEvents(mems)}); err != nil {
		t.Fatal(err)
	}
	ms := NewMemoryService(st)
	plan, err := ms.Organize(context.Background(), sid, bid, "")
	if err != nil || len(plan.Merges) == 0 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	usage, err := ms.Usage(sid, bid)
	if err != nil || usage.Used >= 65 || usage.Protected != 1 {
		t.Fatalf("usage=%+v %v", usage, err)
	}
	historical, _ := pathMemories(st, nodeID)
	if len(domain.ApplyMemoryOverlays(historical)) != 81 {
		t.Fatal("organization mutated its historical source")
	}
	archive := NewArchiveService(st, "test")
	out, err := archive.Export(context.Background(), sid, bid)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := archive.Import(context.Background(), out.Data)
	if err != nil {
		t.Fatal(err)
	}
	bs, _ := st.ListBranches(imported.Session.SessionID)
	got, err := ms.Usage(imported.Session.SessionID, bs[0].BranchID)
	if err != nil || got != usage {
		t.Fatalf("roundtrip quota changed: %+v %+v %v", got, usage, err)
	}
	all, _ := st.ListMemories(imported.Session.SessionID)
	ids := map[string]bool{}
	for _, m := range all {
		ids[m.MemoryID] = true
	}
	evidenceCount := 0
	for _, m := range all {
		if m.Supersedes != "" && !ids[m.Supersedes] {
			t.Fatal("broken overlay reference")
		}
		for _, source := range m.MergedFrom {
			if !ids[source] {
				t.Fatal("broken merge reference")
			}
		}
		if m.Evidence != nil {
			evidenceCount++
			if m.Evidence.SourceNodeID == tr.ResultNodeID {
				t.Fatal("old evidence node survived import")
			}
		}
	}
	if evidenceCount != 1 {
		t.Fatalf("evidence count=%d", evidenceCount)
	}
}

func TestM4CognitiveLateResultAndBudgetAreRejected(t *testing.T) {
	for _, test := range []string{"late", "budget"} {
		t.Run(test, func(t *testing.T) {
			st, turns, _, sid, bid, _ := newTestServices(t, happyScript())
			tr := acceptAndWait(t, turns, st, sid, bid, "source", "玩家拔出了腰间的佩剑")
			called := false
			provider := cognitiveProviderFunc(func(_ context.Context, _ ports.ChatRequest, sink ports.StreamSink) error {
				called = true
				acceptAndWait(t, turns, st, sid, bid, "newer", "玩家已离开酒馆")
				raw, _ := json.Marshal(domain.CognitivePlan{TurnID: tr.TurnID, WriteObserved: []domain.CognitiveMemory{{Content: "曾经拔剑", SourceQuote: "拔出了腰间的佩剑", Confidence: "high"}}})
				return sink.Chunk(raw)
			})
			cog := NewCognitiveService(st, func() (ports.ModelProvider, error) { return provider, nil })
			defer cog.Close()
			if test == "budget" {
				cog.resolve = func() (ports.ModelProvider, ports.ProviderConfig, error) {
					return provider, ports.ProviderConfig{ContextWindow: 1000, MaxTokens: 512}, nil
				}
			}
			if _, err := cog.RunOnce(context.Background(), sid, bid, tr.ResultNodeID, tr.TurnID); err == nil {
				t.Fatal("unsafe extraction accepted")
			}
			if test == "budget" && called {
				t.Fatal("over-budget model was called")
			}
			mem, _ := st.ListMemories(sid)
			if len(mem) != 0 {
				t.Fatal("late extraction wrote memories")
			}
		})
	}
}

func TestM4GraphCapAndCursorIsolation(t *testing.T) {
	st, turns, sessions, sid, bid, root := newTestServices(t, happyScript())
	base := acceptAndWait(t, turns, st, sid, bid, "base", "开始")
	for i := 0; i < 340; i++ {
		b := &domain.Branch{BranchID: fmt.Sprintf("wide%d", i), SessionID: sid, HeadNodeID: base.ResultNodeID}
		if err := st.CreateBranch(b); err != nil {
			t.Fatal(err)
		}
		if _, _, err := turns.SubmitBlocks(context.Background(), SubmitBlocksRequest{SessionID: sid, BranchID: b.BranchID, HeadID: b.HeadNodeID, Blocks: []domain.TextBlock{{Kind: "narration", Text: "候选正文"}}, Input: domain.TurnInput{Kind: "text", Text: "继续"}}); err != nil {
			t.Fatal(err)
		}
	}
	graph, err := sessions.Graph(sid, base.ResultNodeID, 50, 10)
	if err != nil || len(graph.Nodes) > 300 || len(graph.Nodes) < 2 || !graph.Truncated {
		t.Fatalf("graph=%+v err=%v", graph, err)
	}
	raw, _ := json.Marshal(graph)
	if strings.Contains(string(raw), "候选正文") {
		t.Fatal("graph carried full content")
	}
	other, _ := st.GetBranch("wide0")
	if _, err := sessions.View(sid, ViewQuery{BranchID: bid, ViewNodeID: root, Before: other.HeadNodeID}); err == nil {
		t.Fatal("foreign path cursor accepted")
	}
	second, err := sessions.Setup(context.Background(), &SessionSetupRequest{CharacterJSON: testCard, Player: Player{Name: "别人"}, OpeningText: "另一个故事"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.View(sid, ViewQuery{BranchID: bid, Before: second.RootNode.NodeID}); err == nil {
		t.Fatal("cross-session cursor accepted")
	}
	if _, err := sessions.Graph(second.Session.SessionID, base.ResultNodeID, 1, 1); err == nil {
		t.Fatal("cross-session graph accepted")
	}
}

func TestM4PrimaryEvidenceMustNotInventOrPromoteBeliefs(t *testing.T) {
	for _, proposal := range []string{
		"{\"proposalId\":\"m\",\"type\":\"memory_add\",\"text\":\"她想离开\",\"memoryKind\":\"inferred\",\"evidenceConfidence\":\"high\"}",
		"{\"proposalId\":\"m\",\"type\":\"memory_add\",\"text\":\"不存在的事情\",\"memoryKind\":\"observed\",\"sourceQuote\":\"从未出现的逐字引文\",\"evidenceConfidence\":\"high\"}",
	} {
		st, turns, _, sid, bid, _ := newTestServices(t, []mock.Item{mock.Frame(mockBlock(1, "narration", "她静静地看着窗外。")), mock.Frame(mockFinal(2, proposal, ""))})
		br, _ := st.GetBranch(bid)
		tr, err := turns.Accept(context.Background(), sid, bid, &TurnAcceptRequest{IdempotencyKey: "bad-evidence", ExpectedHeadID: br.HeadNodeID, Input: domain.TurnInput{Kind: "text", Text: "你好"}})
		if err != nil {
			t.Fatal(err)
		}
		waitTurn(t, turns, tr.TurnID, domain.TurnFailed)
		mems, _ := st.ListMemories(sid)
		if len(mems) > 0 {
			t.Fatal("invalid main-generation evidence committed")
		}
	}
}
