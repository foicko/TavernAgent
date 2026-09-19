package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/search"
)

func TestM4HybridRecallFiltersBeforeChannelLimits(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)
	var old, revisions, sibling []*domain.MemoryRecord
	for i := 0; i < 260; i++ {
		suffix := fmt.Sprint(i)
		h := newMemory("hidden"+suffix, "铜钥匙")
		h.Hidden = true
		p := newMemory("private"+suffix, "铜钥匙")
		p.OwnerIDs = []string{"other"}
		secret := newMemory("secret"+suffix, "铜钥匙")
		secret.Kind = domain.MemorySecret
		secret.SecretID = "sealed"
		old = append(old, h, p, secret, newMemory("old"+suffix, "铜钥匙"))
		rev := newMemory("revision"+suffix, "一张普通的风景画")
		rev.Supersedes = "old" + suffix
		revisions = append(revisions, rev)
		sibling = append(sibling, newMemory("sibling"+suffix, "铜钥匙"))
	}
	head := commitNode(t, s, sess, "branch_main", root.NodeID, "seed", "开始", old, nil)
	if err := s.CreateBranch(&domain.Branch{BranchID: "sibling", SessionID: sess.SessionID, Name: "旁支", HeadNodeID: head}); err != nil {
		t.Fatal(err)
	}
	commitNode(t, s, sess, "sibling", head, "sib", "其他故事", sibling, nil)
	revisions = append(revisions, newMemory("visible", "她把铜钥匙藏在旧酒馆后方的木盒里，叮嘱旅人不要忘记"), newMemory("stem", "The keeper was running beside the river."), newMemory("substring", "记号为StarwindHarbor的港口"), newMemory("single", "她不喜欢雪"))
	head = commitNode(t, s, sess, "branch_main", head, "main", "继续", revisions, nil)
	for _, tc := range []struct{ query, match, want string }{
		{"铜钥匙", search.MatchExpr("铜钥匙"), "visible"},
		{"run", search.MatchExpr("run"), "stem"},
		{"windHar", "", "substring"},
		{"雪", "", "single"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			got, err := s.RecallMemories(head, ports.MemoryRecallQuery{RawText: tc.query, MatchExpr: tc.match, OwnerIDs: []string{"player"}, Limit: 1})
			if err != nil || len(got) != 1 || got[0].Memory.MemoryID != tc.want {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
	// Explicitly unlocked secrets join only their permitted path.
	got, err := s.RecallMemories(head, ports.MemoryRecallQuery{RawText: "铜钥匙", MatchExpr: search.MatchExpr("铜钥匙"), OwnerIDs: []string{"player"}, SecretIDs: []string{"sealed"}, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range got {
		if c.Memory.SecretID == "sealed" {
			found = true
		}
	}
	if !found {
		t.Fatal("revealed secrets were never recalled")
	}
}

func TestM4PartialIndexesRebuildAndTransactionsRollback(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)
	head := commitNode(t, s, sess, "branch_main", root.NodeID, "seed", "开始", []*domain.MemoryRecord{newMemory("one", "StarwindHarbor港口"), newMemory("two", "她喜欢鲜花")}, nil)
	for _, table := range []string{"memory_fts", "memory_fts_tri"} {
		if _, err := s.db.Exec("DELETE FROM " + table + " WHERE memory_id='one'"); err != nil {
			t.Fatal(err)
		}
		if err := s.backfillMemoryIndex(); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 2 {
			t.Fatalf("%s count=%d %v", table, count, err)
		}
	}
	if _, err := s.db.Exec("CREATE TRIGGER reject_memory_batch BEFORE INSERT ON memory_batches BEGIN SELECT RAISE(ABORT,'injected failure after index write'); END"); err != nil {
		t.Fatal(err)
	}
	br, _ := s.GetBranch("branch_main")
	memory := newMemory("late", "不会保存的港口")
	memory.SourceNodeID = "maintenance"
	payload, _ := json.Marshal(domain.MemoryAddPayload{Memory: *memory})
	b := &ports.MemoryBatch{BatchID: "fault", SessionID: sess.SessionID, BranchID: br.BranchID, ExpectedHeadID: head, ExpectedVersion: br.Version, NodeID: "maintenance", PayloadHash: "fault", Memories: []*domain.MemoryRecord{memory}, Events: []*domain.DomainEvent{{Type: domain.EventMemoryAdd, PayloadJSON: string(payload)}}}
	if _, err := s.CommitMemoryBatch(context.Background(), b); err == nil {
		t.Fatal("injected late failure was ignored")
	}
	after, _ := s.GetBranch(br.BranchID)
	if after.HeadNodeID != head || after.Version != br.Version {
		t.Fatal("failed transaction advanced branch")
	}
	for _, table := range []string{"memory_records", "memory_fts", "memory_fts_tri"} {
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table + " WHERE memory_id='late'").Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s leaked failed write: %d %v", table, count, err)
		}
	}
	if _, err := s.GetNode("maintenance"); err == nil {
		t.Fatal("failed batch left a node")
	}
	events, err := s.PollOutbox(sess.SessionID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.EventID == "fault_updated" {
			t.Fatal("failed batch published outbox event")
		}
	}
}

func TestM4ReceiptAtomicAcrossStoreInstances(t *testing.T) {
	dir := t.TempDir()
	a, err := Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var draws atomic.Int32
	results := make(chan *domain.ActionReceipt, 2)
	errors := make(chan error, 2)
	var wg sync.WaitGroup
	for i, store := range []*Store{a, b} {
		wg.Add(1)
		go func(i int, s *Store) {
			defer wg.Done()
			receipt, e := s.PrepareReceipt(context.Background(), &domain.ActionReceipt{ReceiptID: fmt.Sprintf("r%d", i), TurnID: fmt.Sprintf("t%d", i), ActionID: "gate", RollID: "shared-roll", BaseHeadID: "base", RulesetVersion: "v1", Status: domain.ReceiptPrepared}, func() (string, error) {
				n := draws.Add(1)
				time.Sleep(15 * time.Millisecond)
				return fmt.Sprintf("{\"natural\":%d}", n), nil
			})
			results <- receipt
			errors <- e
		}(i, store)
	}
	wg.Wait()
	close(results)
	close(errors)
	for e := range errors {
		if e != nil {
			t.Fatal(e)
		}
	}
	if draws.Load() != 1 {
		t.Fatalf("same action rolled %d times", draws.Load())
	}
	var previous string
	for r := range results {
		if previous != "" && previous != r.ResultJSON {
			t.Fatal("reused results differ")
		}
		previous = r.ResultJSON
	}
}

func TestM4LegacyV8MigrationBindsIdentityAndTimestamp(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "storage.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:8] {
		if _, err := db.Exec(migration); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("PRAGMA user_version=8"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"native", "unresolved"} {
		if _, err := db.Exec("INSERT INTO sessions(session_id,root_node_id,title,created_at) VALUES(?,?,?,?)", id, "root-"+id, "相同标题", "2026-09-01T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
		content := "{}"
		if id == "native" {
			content = "{\"cardRef\":\"card-tpl\"}"
		}
		if _, err := db.Exec("INSERT INTO plot_nodes(node_id,session_id,kind,depth,turn_number,schema_version,content_json,created_at) VALUES(?,?,'root',0,0,1,?,?)", "root-"+id, id, content, "2026-09-01T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO template_versions(template_version_id,kind,schema_version,content,content_hash,created_at) VALUES('card-tpl','character',2,?,'test',?)", "{\"cardId\":\"stable-imported-card\",\"name\":\"原始名字\"}", "2026-09-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for id, want := range map[string]string{"native": "stable-imported-card", "unresolved": "legacy_unresolved"} {
		sess, err := s.GetSession(id)
		if err != nil || sess.CharacterID != want || sess.UpdatedAt.IsZero() || !sess.UpdatedAt.Equal(sess.CreatedAt) {
			t.Fatalf("%s migrated to %+v err=%v", id, sess, err)
		}
	}
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != len(migrations) {
		t.Fatalf("version=%d %v", version, err)
	}
}

type noFTSStore struct{ *Store }

func (noFTSStore) LexicalIndexAvailable() bool { return false }

func TestM4NoFTSFallbackAndBudgetWithSummaries(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)
	head := commitNode(t, s, sess, "branch_main", root.NodeID, "m", "开场", []*domain.MemoryRecord{newMemory("key", "铜钥匙藏在旧酒馆后门")}, nil)
	stateSnap, _ := s.StateAt(head)
	state, _ := domain.UnmarshalWorld(stateSnap.StateJSON)
	c := ctxpkg.New(noFTSStore{s}, ctxpkg.DefaultOptions())
	req, err := c.Compile(context.Background(), sess.SessionID, head, "铜钥匙在哪里", ctxpkg.TurnDirectives{}, state, nil)
	if err != nil || len(req.InjectedMemoryIDs) != 1 || req.InjectedMemoryIDs[0] != "key" {
		t.Fatalf("fallback=%+v %v", req.InjectedMemoryIDs, err)
	}
	// Real rendering must include every summary in the budget, in both layouts.
	for i := 0; i < 18; i++ {
		head = commitNode(t, s, sess, "branch_main", head, fmt.Sprintf("h%d", i), "普通的历史对白", nil, nil)
	}
	// 摘要必须能装进输入预算：这里给足窗口，锁定"摘要会完整渲染进请求"。
	// 装不下的情况由 internal/context 的承重摘要用例锁定（此前这里是静默丢弃）。
	if err := s.SaveSummary(&domain.SummaryArtifact{SummaryID: "large", FromNodeID: root.NodeID, ToNodeID: stateSnap.NodeID, Text: strings.Repeat("旧事摘要", 3500)}); err != nil {
		t.Fatal(err)
	}
	for _, split := range []bool{true, false} {
		opts := ctxpkg.DefaultOptions()
		opts.SplitDynamicContext = split
		req, err := ctxpkg.New(s, opts).WithBudget(65536, 2048).Compile(context.Background(), sess.SessionID, head, "继续", ctxpkg.TurnDirectives{}, state, nil)
		if err != nil {
			t.Fatal(err)
		}
		if ctxpkg.MessageTokens(req.Messages) > req.InputBudget || req.EstimatedInputTokens != ctxpkg.MessageTokens(req.Messages) {
			t.Fatalf("bad rendered budget: %+v", req)
		}
		rendered := ""
		for _, m := range req.Messages {
			rendered += m.Content
		}
		if !strings.Contains(rendered, "旧事摘要") {
			t.Fatalf("预算充足时摘要应被完整渲染（split=%v）", split)
		}
	}
	// 反向：窗口收窄到装不下这条承重摘要时，必须显式报错而不是悄悄丢掉摘要。
	// 摘要区间覆盖第 1 个回合（在折叠区内），因此它是那段历史唯一的载体。
	_, narrowErr := ctxpkg.New(s, ctxpkg.DefaultOptions()).WithBudget(8192, 2048).Compile(context.Background(), sess.SessionID, head, "继续", ctxpkg.TurnDirectives{}, state, nil)
	if narrowErr == nil {
		t.Fatal("承重摘要装不下时必须报错")
	}
	if ce, ok := narrowErr.(*ctxpkg.Error); !ok || ce.Code != "CONTEXT_OVER_BUDGET" {
		t.Fatalf("应返回 CONTEXT_OVER_BUDGET，得到 %T: %v", narrowErr, narrowErr)
	}
}
