package sqlite

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

func TestDirectorRestartRecoveryAndProjectionReplay(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	session, root, branch := seedSession(t, st)
	plan := domain.DirectorPlan{PlanID: "activation", RevisionID: "activation", Title: "相遇", Beats: []domain.DirectorBeat{
		{BeatID: "first", Title: "认识", Instruction: "交换姓名", CompletionCriteria: "双方说出姓名"},
		{BeatID: "second", Title: "近况", Instruction: "交流近况", CompletionCriteria: "双方说出近况"},
	}}
	apply := func(node, parent string, version int64, change domain.DirectorChange) {
		t.Helper()
		result, err := st.CommitDirector(context.Background(), &ports.DirectorCommit{SessionID: session.SessionID, BranchID: branch.BranchID,
			ExpectedHeadID: parent, ExpectedVersion: version, IdempotencyKey: node, PayloadHash: node,
			Node: &domain.PlotNode{NodeID: node, ContentJSON: `{}`}, Change: change})
		if err != nil || !result.Committed {
			t.Fatalf("director commit: %+v %v", result, err)
		}
	}
	apply("activation", root.NodeID, 0, domain.DirectorChange{Action: "activate", Plan: &plan})
	apply("completion", "activation", 1, domain.DirectorChange{Action: "complete", BeatID: "first", BaseRevisionID: "activation"})
	expected, err := st.DirectorAt("completion")
	if err != nil {
		t.Fatal(err)
	}
	draft := &domain.DirectorDraft{SessionID: session.SessionID, BranchID: branch.BranchID, BaseRevisionID: "activation", Plan: plan}
	if err := st.SaveDirectorDraft(draft, 0); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(draft)
	request := &domain.DirectorRequest{RequestID: "unfinished", SessionID: session.SessionID, BranchID: branch.BranchID,
		IdempotencyKey: "unfinished", PayloadHash: "unfinished", BaseNodeID: "completion", DraftVersion: draft.Version, DraftJSON: string(raw), Text: "细化后续安排"}
	if _, err := st.CreateDirectorRequest(request); err != nil {
		t.Fatal(err)
	}
	// A process crash loses the reconstructible cache, but not confirmed events,
	// working drafts, or accepted asynchronous requests.
	if _, err := st.db.Exec(`DELETE FROM director_projections`); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RecoverDirectorRequests(); err != nil {
		t.Fatal(err)
	}
	replayed, err := st.DirectorAt("completion")
	if err != nil || !reflect.DeepEqual(expected, replayed) {
		t.Fatalf("projection replay after restart: %+v %v", replayed, err)
	}
	got, err := st.GetDirectorRequest(request.RequestID)
	if err != nil || got.Status != "interrupted" || got.Text != request.Text {
		t.Fatalf("request not recovered: %+v %v", got, err)
	}
	st.RecoverDirectorRequests()
	events, err := st.PollOutbox(request.RequestID, 0, 10)
	if err != nil || len(events) != 2 || events[1].Type != "director.interrupted" {
		t.Fatalf("recovery must emit exactly one terminal event: %+v %v", events, err)
	}
	if _, err := st.FinishDirectorRequest(request.RequestID, "completed", "迟到结果", "", &plan); err != nil {
		t.Fatal(err)
	}
	saved, _ := st.GetDirectorDraft(branch.BranchID)
	if saved.Version != draft.Version {
		t.Fatal("late output overwrote a recovered draft")
	}
}

func TestDirectorUpgradePreservesLegacyStory(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	session, root, branch := seedSession(t, st)
	// Remove v10 and later additions to recreate a v9 database with a real story.
	// 迁移是追加式的：新增 schema 后必须同步在此登记，否则本用例会带着
	// "新表已存在" 的前提重放迁移，导致 CREATE TABLE 失败。
	_, err = st.db.Exec(`DROP TABLE director_commands; DROP TABLE director_requests;
		DROP TABLE memory_projection_snapshots;
		DROP INDEX idx_turns_status; DROP INDEX idx_attempts_turn_number;
		ALTER TABLE turn_requests DROP COLUMN active_attempt_id;
		DROP TABLE director_drafts; DROP TABLE director_projections;
		DROP TABLE turn_usage; DROP TABLE character_cards;
		DROP INDEX IF EXISTS idx_memory_subject;
		ALTER TABLE memory_records DROP COLUMN subject_key;
		ALTER TABLE memory_records DROP COLUMN created_turn;
		ALTER TABLE memory_records DROP COLUMN valid_from_turn;
		ALTER TABLE memory_records DROP COLUMN valid_until_turn;
		DROP INDEX idx_director_events; PRAGMA user_version=9;`)
	if err != nil {
		st.Close()
		t.Fatal(err)
	}
	st.Close()
	st, err = Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.GetSession(session.SessionID)
	if err != nil || got.RootNodeID != root.NodeID {
		t.Fatalf("legacy story changed: %+v %v", got, err)
	}
	state, err := st.DirectorAt(root.NodeID)
	if err != nil || state != nil {
		t.Fatalf("legacy story acquired a plan: %+v %v", state, err)
	}
	if err := st.SaveDirectorDraft(&domain.DirectorDraft{SessionID: session.SessionID, BranchID: branch.BranchID, Plan: domain.DirectorPlan{Title: "新草稿"}}, 0); err != nil {
		t.Fatalf("upgraded schema cannot save drafts: %v", err)
	}
}
