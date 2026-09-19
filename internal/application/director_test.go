package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/pack"
	"tavernagent/internal/ports"
	"tavernagent/internal/util/id"
	"testing"
	"time"
)

func directorTestPlan() domain.DirectorPlan {
	return domain.DirectorPlan{Title: "重逢", Guidance: "克制温柔，尊重玩家选择", Beats: []domain.DirectorBeat{
		{BeatID: "meeting", Title: "确认身份", Instruction: "请她说出旅人的旧称呼", CompletionCriteria: "她明确说出旧称呼"},
		{BeatID: "conflict", Title: "误会", Instruction: "未来专属暗号：银色狐狸其实是叛徒", CompletionCriteria: "双方说清误会"},
		{BeatID: "peace", Title: "和解", Instruction: "自然修复关系", CompletionCriteria: "双方互相道歉"},
	}}
}

func testDirector(t *testing.T, st *sqlite.Store, provider ports.ModelProvider) *DirectorService {
	t.Helper()
	s := NewDirectorService(st, nil, nil, NewEventBus(st), provider)
	t.Cleanup(s.Close)
	return s
}
func saveTestDirector(t *testing.T, s *DirectorService, sid, bid string, p domain.DirectorPlan) *domain.DirectorDraft {
	t.Helper()
	v, err := s.View(sid, bid, "")
	if err != nil {
		t.Fatal(err)
	}
	var version int64
	var revision string
	if v.Draft != nil {
		version = v.Draft.Version
	}
	if v.State != nil {
		revision = v.State.Plan.RevisionID
	}
	d, err := s.SaveDraft(sid, bid, SaveDirectorDraftRequest{ExpectedCharacterID: "card_elena", ExpectedDraftVersion: version, BaseRevisionID: revision, Plan: p})
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func commandTestDirector(t *testing.T, s *DirectorService, st *sqlite.Store, sid, bid, action, beat string) *DirectorCommandResult {
	t.Helper()
	b, _ := st.GetBranch(bid)
	d, _ := st.GetDirectorDraft(bid)
	var version int64
	if d != nil {
		version = d.Version
	}
	r, err := s.Command(context.Background(), sid, bid, DirectorCommandRequest{ExpectedCharacterID: "card_elena", ExpectedHeadID: b.HeadNodeID, ExpectedVersion: b.Version, IdempotencyKey: id.New(), Action: action, BeatID: beat, DraftVersion: version})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func waitDirector(t *testing.T, s *DirectorService, requestID string) *domain.DirectorRequest {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, err := s.GetRequest(requestID)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != "generating" {
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("director request did not finish")
	return nil
}
func directorFinal(revision, beat, status, quote string) string {
	raw, _ := json.Marshal(domain.DirectorReport{RevisionID: revision, BeatID: beat, Status: status, Evidence: []domain.DirectorEvidence{{BlockSeq: 1, Quote: quote}}})
	return `{"v":1,"seq":2,"type":"final","proposals":[],"options":[],"director":` + string(raw) + `}`
}
func submitDirectorTurn(t *testing.T, st *sqlite.Store, turns *TurnService, sid, bid string) *domain.TurnRequest {
	t.Helper()
	b, _ := st.GetBranch(bid)
	r, err := turns.Accept(context.Background(), sid, bid, &TurnAcceptRequest{ExpectedHeadID: b.HeadNodeID, ExpectedVersion: b.Version, IdempotencyKey: id.New(), Input: domain.TurnInput{Kind: "text", Text: "听她继续说。"}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestDirectorCommandsBranchIsolationAndOptions(t *testing.T) {
	st, turns, views, sid, bid, root := newTestServices(t, happyScript())
	s := testDirector(t, st, nil)
	turn := submitDirectorTurn(t, st, turns, sid, bid)
	turn = waitTurn(t, turns, turn.TurnID, domain.TurnCommitted)
	before, _ := st.StateAt(turn.ResultNodeID)
	originalBranch, _ := st.GetBranch(bid)
	draft := saveTestDirector(t, s, sid, bid, directorTestPlan())
	b, _ := st.GetBranch(bid)
	if b.HeadNodeID != originalBranch.HeadNodeID || b.Version != originalBranch.Version {
		t.Fatal("draft advanced story")
	}
	req := DirectorCommandRequest{ExpectedCharacterID: "card_elena", ExpectedHeadID: b.HeadNodeID, ExpectedVersion: b.Version, IdempotencyKey: "apply-once", Action: "activate", DraftVersion: draft.Version}
	result, err := s.Command(context.Background(), sid, bid, req)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := s.Command(context.Background(), sid, bid, req)
	if err != nil || duplicate.NodeID != result.NodeID {
		t.Fatalf("idempotent apply: %+v %v", duplicate, err)
	}
	n, _ := st.GetNode(result.NodeID)
	old, _ := st.GetNode(turn.ResultNodeID)
	after, _ := st.StateAt(result.NodeID)
	if n.TurnNumber != old.TurnNumber || n.Depth != old.Depth+1 || n.Kind != domain.NodeKindDirectorEvent || before.StateHash != after.StateHash {
		t.Fatal("director changed world or turn numbering")
	}
	_, option, err := turns.validateTurnInput(result.NodeID, domain.TurnInput{Kind: "option", Text: "告诉她，我仍记得那个约定。", OptionRef: &domain.OptionRef{NodeID: turn.ResultNodeID, OptionID: "o1"}})
	if err != nil || option == nil {
		t.Fatalf("director invalidated option: %v", err)
	}
	historical, err := views.View(sid, ViewQuery{BranchID: bid, ViewNodeID: root})
	if err != nil {
		t.Fatal(err)
	}
	if historical.Director != nil {
		t.Fatal("future plan leaked into past")
	}
	branchSvc := NewBranchService(st, turns)
	fork, err := branchSvc.Fork(context.Background(), sid, ForkRequest{FromNodeID: result.NodeID})
	if err != nil {
		t.Fatal(err)
	}
	commandTestDirector(t, s, st, sid, bid, "complete", "meeting")
	forkState, _ := st.DirectorAt(fork.HeadNodeID)
	if forkState.CurrentBeatID != "meeting" {
		t.Fatal("sibling progress changed")
	}
	commandTestDirector(t, s, st, sid, bid, "skip", "conflict")
	commandTestDirector(t, s, st, sid, bid, "complete", "peace")
	current, _ := s.View(sid, bid, "")
	if current.State.Status != "completed" {
		t.Fatal("plan not completed")
	}
	commandTestDirector(t, s, st, sid, bid, "rewind", "conflict")
	current, _ = s.View(sid, bid, "")
	if current.State.CurrentBeatID != "conflict" || len(current.State.Progress) != 1 {
		t.Fatal("rewind did not clear future progress")
	}
	commandTestDirector(t, s, st, sid, bid, "pause", "")
	rewound := commandTestDirector(t, s, st, sid, bid, "rewind", "meeting")
	if rewound.State.Status != "paused" {
		t.Fatal("rewind unexpectedly resumed a paused plan")
	}
	commandTestDirector(t, s, st, sid, bid, "resume", "")
	forkView, _ := s.View(sid, fork.BranchID, "")
	if forkView.Draft != nil || len(forkView.Requests) != 0 {
		t.Fatal("fork copied working discussion")
	}
	st.ClaimActiveTurn(bid, "pending")
	b, _ = st.GetBranch(bid)
	_, err = s.Command(context.Background(), sid, bid, DirectorCommandRequest{ExpectedHeadID: b.HeadNodeID, ExpectedVersion: b.Version, IdempotencyKey: id.New(), Action: "pause"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "TURN_BUSY" {
		t.Fatalf("active turn should block commands: %v", err)
	}
	saveTestDirector(t, s, sid, bid, current.State.Plan)
	st.ReleaseActiveTurn(bid, "pending")
}

func TestDirectorCompilerVisibilityBudgetAndAtomicProgress(t *testing.T) {
	st, turns, _, sid, bid, _ := newTestServices(t, nil)
	s := testDirector(t, st, nil)
	saveTestDirector(t, s, sid, bid, directorTestPlan())
	activated := commandTestDirector(t, s, st, sid, bid, "activate", "")
	snapshot, _ := st.StateAt(activated.NodeID)
	world, _ := domain.UnmarshalWorld(snapshot.StateJSON)
	for _, split := range []bool{false, true} {
		opts := ctxpkg.DefaultOptions()
		opts.SplitDynamicContext = split
		compiler := ctxpkg.New(st, opts)
		req, err := compiler.Compile(context.Background(), sid, activated.NodeID, "你好", ctxpkg.TurnDirectives{}, world, nil)
		if err != nil {
			t.Fatal(err)
		}
		var prompt string
		for _, msg := range req.Messages {
			prompt += msg.Content
		}
		if !strings.Contains(prompt, "请她说出旅人的旧称呼") || strings.Contains(prompt, "银色狐狸") || strings.Contains(prompt, "自然修复关系") {
			t.Fatal("incorrect director prompt visibility")
		}
		_, err = compiler.WithBudget(600, 100).Compile(context.Background(), sid, activated.NodeID, "你好", ctxpkg.TurnDirectives{}, world, nil)
		if err == nil {
			t.Fatal("mandatory plan silently dropped to fit budget")
		}
	}
	quote := "她望着旅人，清楚地喊出了旧日的称呼。"
	provider := mock.New([]mock.Item{mock.Frame(mockBlock(1, "narration", quote)), mock.Frame(directorFinal(activated.State.Plan.RevisionID, "meeting", "completed", quote))})
	turns.provider = provider
	r := submitDirectorTurn(t, st, turns, sid, bid)
	r = waitTurn(t, turns, r.TurnID, domain.TurnCommitted)
	progress, _ := st.DirectorAt(r.ResultNodeID)
	if progress.CurrentBeatID != "conflict" || progress.Progress["meeting"].SourceNodeID != r.ResultNodeID {
		t.Fatal("progress not committed with story")
	}
	old, _ := st.DirectorAt(activated.NodeID)
	if old.CurrentBeatID != "meeting" {
		t.Fatal("committed progress mutated old state")
	}
	// Replaying the same completed request cannot apply its report twice.
	b, _ := st.GetBranch(bid)
	turns.Cancel(context.Background(), r.TurnID)
	latest, _ := st.GetBranch(bid)
	if latest.HeadNodeID != b.HeadNodeID {
		t.Fatal("terminal request advanced twice")
	}
	derived, err := NewBranchService(st, turns).DeriveTurn(context.Background(), sid, DeriveRequest{NodeID: r.ResultNodeID, IdempotencyKey: "regenerate-director"})
	if err != nil {
		t.Fatal(err)
	}
	regenerated := waitTurn(t, turns, derived.Turn.TurnID, domain.TurnCommitted)
	replayed, _ := st.DirectorAt(regenerated.ResultNodeID)
	if len(replayed.Progress) != 1 || replayed.CurrentBeatID != "conflict" {
		t.Fatal("regeneration reused post-turn progress")
	}
}

func TestDirectorTruncationContinuationAndInvalidReports(t *testing.T) {
	st, turns, _, sid, bid, _ := newTestServices(t, nil)
	s := testDirector(t, st, nil)
	saveTestDirector(t, s, sid, bid, directorTestPlan())
	active := commandTestDirector(t, s, st, sid, bid, "activate", "")
	quote := "她站在门边，终于叫出了那熟悉的旧称呼。"
	provider := mock.New([]mock.Item{{Line: mockBlock(1, "narration", quote) + "\n", Err: ports.ErrTruncatedStream}})
	turns.provider = provider
	r := submitDirectorTurn(t, st, turns, sid, bid)
	waitTurn(t, turns, r.TurnID, domain.TurnAwaitingContinuation)
	b, _ := st.GetBranch(bid)
	state, _ := st.DirectorAt(b.HeadNodeID)
	if state.CurrentBeatID != "meeting" {
		t.Fatal("truncated story advanced plan")
	}
	provider.SetScript([]mock.Item{mock.Frame(directorFinal(active.State.Plan.RevisionID, "meeting", "completed", quote))})
	if _, err := turns.Continue(context.Background(), r.TurnID); err != nil {
		t.Fatal(err)
	}
	r = waitTurn(t, turns, r.TurnID, domain.TurnCommitted)
	state, _ = st.DirectorAt(r.ResultNodeID)
	if state.CurrentBeatID != "conflict" {
		t.Fatal("continuation lost report")
	}
	for _, tc := range []struct{ name, raw, kind, text string }{
		{"missing", "", "narration", quote},
		{"wrong shape", `{"status":12}`, "narration", quote},
		{"wrong revision", `{"revisionId":"old","beatId":"meeting","status":"completed"}`, "narration", quote},
		{"thought", ``, "inner_monologue", quote},
		{"intention", ``, "narration", "她打算叫出那熟悉的旧日称呼。"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage(tc.raw)
			if tc.name == "thought" || tc.name == "intention" {
				raw, _ = json.Marshal(domain.DirectorReport{RevisionID: active.State.Plan.RevisionID, BeatID: "meeting", Status: "completed", Evidence: []domain.DirectorEvidence{{BlockSeq: 1, Quote: tc.text}}})
			}
			e := directorProgressEvent(active.State, raw, []domain.TextBlock{{Kind: tc.kind, Text: tc.text}}, "new-node")
			newState, err := domain.ApplyDirectorEvent(active.State, e)
			if err != nil || newState.CurrentBeatID != "meeting" || newState.Warning == "" {
				t.Fatalf("invalid report advanced progress: %+v %v", newState, err)
			}
		})
	}
}

type directorFunctionProvider struct {
	stream func(context.Context, ports.ChatRequest, ports.StreamSink) error
}

func (p directorFunctionProvider) Stream(ctx context.Context, r ports.ChatRequest, sink ports.StreamSink) error {
	return p.stream(ctx, r, sink)
}
func (p directorFunctionProvider) Capabilities(context.Context) (ports.ProviderCapabilities, error) {
	return ports.ProviderCapabilities{ID: "director-test", Streaming: true}, nil
}

func TestDirectorDiscussionDurabilityAndLateEditorIsolation(t *testing.T) {
	st, _, _, sid, bid, root := newTestServices(t, nil)
	started, release := make(chan struct{}), make(chan struct{})
	provider := directorFunctionProvider{stream: func(ctx context.Context, req ports.ChatRequest, sink ports.StreamSink) error {
		var prompt string
		for _, m := range req.Messages {
			prompt += m.Content
		}
		if !strings.Contains(prompt, "【导演协商】") || strings.Contains(prompt, ctxpkg.FrameProtocolInstruction) || strings.Contains(prompt, "用第二人称叙述玩家所见所感") {
			return errors.New("wrong planning compiler")
		}
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		raw, _ := json.Marshal(map[string]any{"reply": "建议先确认身份，再处理误会。", "plan": directorTestPlan()})
		return sink.Chunk(raw)
	}}
	s := testDirector(t, st, provider)
	d := saveTestDirector(t, s, sid, bid, directorTestPlan())
	input := DirectorMessageRequest{ExpectedHeadID: root, ExpectedDraftVersion: d.Version, ExpectedCharacterID: "card_elena", IdempotencyKey: "discussion-once", Text: "请帮我细化这个大纲，讨论专用标记。"}
	r, err := s.Discuss(sid, bid, input)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("discussion did not start")
	}
	repeat, err := s.Discuss(sid, bid, input)
	if err != nil || repeat.RequestID != r.RequestID {
		t.Fatal("duplicate discussion not idempotent")
	}
	manual := directorTestPlan()
	manual.Title = "用户正在编辑的新标题"
	saved := saveTestDirector(t, s, sid, bid, manual)
	close(release)
	done := waitDirector(t, s, r.RequestID)
	if done.Status != "completed" || done.DraftApplied || done.Candidate == nil {
		t.Fatalf("late response: %+v", done)
	}
	v, _ := s.View(sid, bid, "")
	if v.Draft.Version != saved.Version || v.Draft.Plan.Title != manual.Title || len(v.Requests) != 1 {
		t.Fatal("late response overwrote draft or transcript")
	}
	b, _ := st.GetBranch(bid)
	memories, _ := st.ListMemories(sid)
	if b.HeadNodeID != root || b.Version != 0 || len(memories) != 0 {
		t.Fatal("planning modified story")
	}
	input.Text = "same key different text"
	if _, err := s.Discuss(sid, bid, input); err == nil {
		t.Fatal("idempotency payload mismatch accepted")
	}
	// Persisted requests survive service restart without restarting model work.
	s.Close()
	restarted := testDirector(t, st, nil)
	if err := restarted.Recover(); err != nil {
		t.Fatal(err)
	}
	got, _ := restarted.GetRequest(r.RequestID)
	if got.Reply != done.Reply || got.Status != "completed" {
		t.Fatal("discussion lost after restart")
	}
}

func TestDirectorDiscussionAppliesDraftAndCancelWins(t *testing.T) {
	st, _, _, sid, bid, root := newTestServices(t, nil)
	response, _ := json.Marshal(map[string]any{"reply": "这是三个阶段的草稿。", "plan": directorTestPlan()})
	s := testDirector(t, st, mock.New([]mock.Item{{Line: string(response)}}))
	r, err := s.Discuss(sid, bid, DirectorMessageRequest{ExpectedHeadID: root, IdempotencyKey: "new-draft", Text: "制定三阶段大纲"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitDirector(t, s, r.RequestID)
	if done.Status != "completed" || !done.DraftApplied {
		t.Fatalf("draft not applied: %+v", done)
	}
	d, _ := st.GetDirectorDraft(bid)
	if d.Version != 1 {
		t.Fatal("wrong draft version")
	}
	started, release := make(chan struct{}), make(chan struct{})
	s.provider = directorFunctionProvider{stream: func(_ context.Context, _ ports.ChatRequest, sink ports.StreamSink) error {
		close(started)
		<-release
		return sink.Chunk(response)
	}}
	r, err = s.Discuss(sid, bid, DirectorMessageRequest{ExpectedHeadID: root, ExpectedDraftVersion: d.Version, IdempotencyKey: "cancel-discussion", Text: "再修改"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := s.CancelRequest(r.RequestID, "card_elena"); err != nil {
		t.Fatal(err)
	}
	close(release)
	s.Close()
	got, _ := s.GetRequest(r.RequestID)
	d2, _ := st.GetDirectorDraft(bid)
	if got.Status != "cancelled" || d2.Version != d.Version {
		t.Fatal("late model output bypassed cancel")
	}
}

func TestDirectorPackRoundTripAndEvidenceValidation(t *testing.T) {
	st, turns, _, sid, bid, _ := newTestServices(t, nil)
	s := testDirector(t, st, nil)
	saveTestDirector(t, s, sid, bid, directorTestPlan())
	draftOnly, err := st.ExportSession(sid, bid)
	if err != nil || draftOnly.FormatVersion != 1 {
		t.Fatalf("unconfirmed draft changed archive format: %v", err)
	}
	active := commandTestDirector(t, s, st, sid, bid, "activate", "")
	quote := "她清楚地说出了旅人多年未听见的称呼。"
	turns.provider = mock.New([]mock.Item{mock.Frame(mockBlock(1, "narration", quote)), mock.Frame(directorFinal(active.State.Plan.RevisionID, "meeting", "completed", quote))})
	r := submitDirectorTurn(t, st, turns, sid, bid)
	r = waitTurn(t, turns, r.TurnID, domain.TurnCommitted)
	revised := directorTestPlan()
	revised.Beats[1].Instruction = "通过旧信解释误会"
	saveTestDirector(t, s, sid, bid, revised)
	active = commandTestDirector(t, s, st, sid, bid, "activate", "")
	bundle, err := st.ExportSession(sid, bid)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	manifest, err := pack.Write(&encoded, bundle, "director-test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if manifest.FormatVersion != 2 {
		t.Fatal("director data exported as legacy format")
	}
	result, err := NewArchiveService(st, "director-test").Import(context.Background(), encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	branches, _ := st.ListBranches(result.Session.SessionID)
	restored, err := st.DirectorAt(branches[0].HeadNodeID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.CurrentBeatID != "conflict" || restored.Plan.RevisionID == active.State.Plan.RevisionID || restored.Progress["meeting"].SourceNodeID == r.ResultNodeID {
		t.Fatal("director references were not remapped")
	}
	n, _ := st.GetNode(restored.Plan.RevisionID)
	if n.SessionID != result.Session.SessionID {
		t.Fatal("import points outside session")
	}
	for _, ev := range bundle.Events {
		if ev.Type == domain.EventDirectorChange {
			var c domain.DirectorChange
			json.Unmarshal([]byte(ev.PayloadJSON), &c)
			if c.Report != nil {
				c.Report.Evidence[0].Quote = "正文中不存在的捏造完成依据"
				raw, _ := json.Marshal(c)
				ev.PayloadJSON = string(raw)
			}
		}
	}
	encoded.Reset()
	pack.Write(&encoded, bundle, "bad-evidence", time.Now())
	if _, err := NewArchiveService(st, "test").Import(context.Background(), encoded.Bytes()); err == nil {
		t.Fatal("forged director evidence imported")
	}
}

func TestDirectorPlanRevisionsAndReplacement(t *testing.T) {
	st, _, _, sid, bid, _ := newTestServices(t, nil)
	s := testDirector(t, st, nil)
	saveTestDirector(t, s, sid, bid, directorTestPlan())
	first := commandTestDirector(t, s, st, sid, bid, "activate", "")
	commandTestDirector(t, s, st, sid, bid, "complete", "meeting")
	command := func(action, beat string, replace bool) (*DirectorCommandResult, error) {
		b, _ := st.GetBranch(bid)
		d, _ := st.GetDirectorDraft(bid)
		return s.Command(context.Background(), sid, bid, DirectorCommandRequest{ExpectedHeadID: b.HeadNodeID, ExpectedVersion: b.Version, IdempotencyKey: id.New(), Action: action, BeatID: beat, DraftVersion: d.Version, Replace: replace})
	}
	if _, err := command("rewind", "peace", false); err == nil {
		t.Fatal("rewind jumped forward to an unstarted stage")
	}
	changed := directorTestPlan()
	changed.Beats[0].Instruction = "修改已经完成的历史安排"
	saveTestDirector(t, s, sid, bid, changed)
	if _, err := command("activate", "", false); err == nil {
		t.Fatal("settled stage was changed")
	}
	changed = directorTestPlan()
	changed.Title = "修订后的重逢"
	changed.Beats[1].Instruction = "让误会以一封旧信展开"
	saveTestDirector(t, s, sid, bid, changed)
	second := commandTestDirector(t, s, st, sid, bid, "activate", "")
	if second.State.Plan.PlanID != first.State.Plan.PlanID || second.State.Plan.RevisionID == first.State.Plan.RevisionID || second.State.CurrentBeatID != "conflict" || len(second.State.Progress) != 1 {
		t.Fatal("revision lost plan identity or settled progress")
	}
	past, _ := st.DirectorAt(first.NodeID)
	if past.Plan.Title != "重逢" || past.CurrentBeatID != "meeting" {
		t.Fatal("revision modified immutable history")
	}
	draft, _ := st.GetDirectorDraft(bid)
	_, err := s.SaveDraft(sid, bid, SaveDirectorDraftRequest{ExpectedDraftVersion: draft.Version, BaseRevisionID: first.State.Plan.RevisionID, Plan: changed})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := command("activate", "", false); err == nil {
		t.Fatal("stale base revision was applied")
	}
	replacement, err := command("activate", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.State.Plan.PlanID == first.State.Plan.PlanID || replacement.State.CurrentBeatID != "meeting" || len(replacement.State.Progress) != 0 {
		t.Fatal("replacement did not start a new plan")
	}
}

func TestDirectorPauseAndCompletionRestoreFreeStory(t *testing.T) {
	st, _, _, sid, bid, _ := newTestServices(t, nil)
	s := testDirector(t, st, nil)
	saveTestDirector(t, s, sid, bid, directorTestPlan())
	commandTestDirector(t, s, st, sid, bid, "activate", "")
	paused := commandTestDirector(t, s, st, sid, bid, "pause", "")
	assertFree := func(nodeID string) {
		t.Helper()
		snap, _ := st.StateAt(nodeID)
		world, _ := domain.UnmarshalWorld(snap.StateJSON)
		req, err := ctxpkg.New(st, ctxpkg.DefaultOptions()).Compile(context.Background(), sid, nodeID, "继续探索", ctxpkg.TurnDirectives{}, world, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, message := range req.Messages {
			if strings.Contains(message.Content, "【导演安排") || strings.Contains(message.Content, "请她说出旅人的旧称呼") {
				t.Fatal("inactive outline constrained story context")
			}
		}
	}
	assertFree(paused.NodeID)
	if ev := directorProgressEvent(paused.State, nil, nil, "ignored"); ev != nil {
		t.Fatal("paused plan consumed a progress report")
	}
	commandTestDirector(t, s, st, sid, bid, "resume", "")
	commandTestDirector(t, s, st, sid, bid, "complete", "meeting")
	commandTestDirector(t, s, st, sid, bid, "skip", "conflict")
	completed := commandTestDirector(t, s, st, sid, bid, "complete", "peace")
	assertFree(completed.NodeID)
}

func TestDirectorInvalidDiscussionPreservesDraft(t *testing.T) {
	st, _, _, sid, bid, root := newTestServices(t, nil)
	s := testDirector(t, st, mock.New([]mock.Item{{Line: `{"reply":"新安排","plan":{"title":"缺少阶段内容","beats":[{}]}}`}}))
	draft := saveTestDirector(t, s, sid, bid, directorTestPlan())
	r, err := s.Discuss(sid, bid, DirectorMessageRequest{ExpectedHeadID: root, ExpectedDraftVersion: draft.Version, IdempotencyKey: "invalid-plan", Text: "重新安排"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitDirector(t, s, r.RequestID)
	got, _ := st.GetDirectorDraft(bid)
	if done.Status != "failed" || got.Version != draft.Version || got.Plan.Title != draft.Plan.Title {
		t.Fatal("invalid model plan replaced the original draft")
	}
}

func TestDirectorDetourAndRuleConflictKeepStage(t *testing.T) {
	st, turns, _, sid, bid, _ := newTestServices(t, nil)
	s := testDirector(t, st, nil)
	saveTestDirector(t, s, sid, bid, directorTestPlan())
	active := commandTestDirector(t, s, st, sid, bid, "activate", "")
	for _, tc := range []struct{ status, body, reason string }{
		{"continue", "她暂时停下叙旧，陪你查看窗外传来的动静。", "玩家正在调查，可以在调查结束后继续确认身份。"},
		{"blocked", "她在柜台边停住脚步，没有交出那封旧信。", "当前规则结果没有允许取得旧信，建议调整阶段安排，保留已有结果。"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			raw, _ := json.Marshal(domain.DirectorReport{RevisionID: active.State.Plan.RevisionID, BeatID: "meeting", Status: tc.status, Reason: tc.reason})
			turns.provider = mock.New([]mock.Item{mock.Frame(mockBlock(1, "narration", tc.body)), mock.Frame(`{"v":1,"seq":2,"type":"final","proposals":[],"options":[],"director":` + string(raw) + `}`)})
			r := submitDirectorTurn(t, st, turns, sid, bid)
			r = waitTurn(t, turns, r.TurnID, domain.TurnCommitted)
			got, err := st.DirectorAt(r.ResultNodeID)
			if err != nil || got.CurrentBeatID != "meeting" || len(got.Progress) != 0 || got.Warning != tc.reason {
				t.Fatalf("detour/conflict changed the outline: %+v %v", got, err)
			}
		})
	}
}

func TestDirectorModelReferencesAreBoundAndCanonicalized(t *testing.T) {
	state := &domain.DirectorState{Plan: directorTestPlan(), Status: "active", CurrentBeatID: "meeting", Progress: map[string]domain.DirectorBeatProgress{}}
	state.Plan.RevisionID = "immutable-revision-one"
	revision, beat := state.ReportReferences()
	quote := "她望着你，清楚地说出了那个旧日称呼。"
	report := domain.DirectorReport{RevisionID: revision, BeatID: beat, Status: "completed", Evidence: []domain.DirectorEvidence{{BlockSeq: 1, Quote: quote}}}
	raw, _ := json.Marshal(report)
	ev := directorProgressEvent(state, raw, []domain.TextBlock{{Kind: "narration", Text: quote}}, "story-node")
	var change domain.DirectorChange
	json.Unmarshal([]byte(ev.PayloadJSON), &change)
	if change.Report.RevisionID != state.Plan.RevisionID || change.Report.BeatID != "meeting" || change.Report.Status != "completed" {
		t.Fatal("valid model reference was not stored with canonical IDs")
	}
	state.Plan.RevisionID = "immutable-revision-two"
	ev = directorProgressEvent(state, raw, []domain.TextBlock{{Kind: "narration", Text: quote}}, "later-node")
	result, err := domain.ApplyDirectorEvent(state, ev)
	if err != nil || result.CurrentBeatID != "meeting" || len(result.Progress) != 0 || result.Warning == "" {
		t.Fatal("old model references advanced a newer revision")
	}
}

func TestParseDirectorResponse(t *testing.T) {
	// Case 1: Markdown code fence with unknown fields and thought tags
	input1 := "好的，我为你规划了大纲：\n<think>一些思考内容</think>\n```json\n{\n  \"reply\": \"已经规划好\",\n  \"thought\": \"额外字段\",\n  \"plan\": {\n    \"title\": \"新大纲\",\n    \"guidance\": \"保持沉静\",\n    \"beats\": [\n      {\"beat_id\": \"b1\", \"title\": \"阶段1\", \"description\": \"说明\", \"completion_criteria\": \"条件\"}\n    ]\n  }\n}\n```\n希望你满意！"
	reply, plan, err := parseDirectorResponse([]byte(input1))
	if err != nil {
		t.Fatalf("unexpected error parsing input1: %v", err)
	}
	if reply != "已经规划好" || plan == nil || plan.Title != "新大纲" || len(plan.Beats) != 1 {
		t.Fatalf("unexpected parsed result: reply=%q, plan=%+v", reply, plan)
	}
	if plan.Beats[0].BeatID != "b1" || plan.Beats[0].Instruction != "说明" || plan.Beats[0].CompletionCriteria != "条件" {
		t.Fatalf("alias fields not mapped: %+v", plan.Beats[0])
	}

	// Case 2: Direct plan object without envelope
	input2 := "```json\n{\n  \"title\": \"直接输出的大纲\",\n  \"guidance\": \"风格指引\",\n  \"beats\": [\n    {\"id\": \"stage1\", \"title\": \"阶段一\", \"content\": \"安排\", \"criteria\": \"完成条件\"}\n  ]\n}\n```"
	reply2, plan2, err := parseDirectorResponse([]byte(input2))
	if err != nil {
		t.Fatalf("unexpected error parsing input2: %v", err)
	}
	if reply2 == "" || plan2 == nil || plan2.Title != "直接输出的大纲" || len(plan2.Beats) != 1 {
		t.Fatalf("unexpected direct plan: reply=%q, plan=%+v", reply2, plan2)
	}
	if plan2.Beats[0].BeatID != "stage1" || plan2.Beats[0].Instruction != "安排" || plan2.Beats[0].CompletionCriteria != "完成条件" {
		t.Fatalf("direct plan beats alias mismatch: %+v", plan2.Beats[0])
	}

	// Case 3: Upstream safety policy violation
	input3 := "The prompt could not be submitted. The prompt contains sensitive words that violate Google's Generative AI Prohibited Use policy."
	_, _, err = parseDirectorResponse([]byte(input3))
	if err == nil {
		t.Fatal("expected error on safety policy block")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "DIRECTOR_POLICY_VIOLATION" {
		t.Fatalf("expected DIRECTOR_POLICY_VIOLATION, got: %v", err)
	}
}
