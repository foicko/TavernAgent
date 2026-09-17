package sqlite

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"tavernagent/internal/domain"
)

func seedLorebook(t *testing.T, s *Store, suffix string, books []domain.Lorebook) string {
	t.Helper()
	raw, _ := json.Marshal(books)
	tpl := &domain.TemplateVersion{TemplateVersionID: "lore-" + suffix, Kind: domain.TemplateLorebook, SchemaVersion: 1, Content: string(raw), ContentHash: fmt.Sprintf("%x", sha256.Sum256(raw))}
	sess := &domain.Session{SessionID: "session-" + suffix, RootNodeID: "root-" + suffix, CreatedAt: time.Now()}
	root := &domain.PlotNode{NodeID: sess.RootNodeID, SessionID: sess.SessionID, Kind: domain.NodeKindRoot, ContentJSON: "{}"}
	branch := &domain.Branch{BranchID: "branch-" + suffix, SessionID: sess.SessionID, HeadNodeID: root.NodeID}
	if err := s.CreateSession(sess, root, branch, []*domain.TemplateVersion{tpl}); err != nil {
		t.Fatal(err)
	}
	return tpl.TemplateVersionID
}

func TestLorebookSearchScopeRankingAndPaging(t *testing.T) {
	s := newTestStore(t)
	if !s.loreFTS {
		t.Fatal("test requires actual FTS5")
	}
	noise := make([]domain.LorebookEntry, 220)
	for i := range noise {
		noise[i] = domain.LorebookEntry{EntryID: fmt.Sprint(i), Title: "龙渊港", Content: "running charts", Enabled: true}
	}
	seedLorebook(t, s, "foreign", []domain.Lorebook{{Entries: noise}})
	tpl := seedLorebook(t, s, "visible", []domain.Lorebook{{Name: "航海手册", Entries: []domain.LorebookEntry{
		{EntryID: "same", Title: "龙渊港航线", Content: "The keeper is running across the deck.", Enabled: true},
		{EntryID: "same", Keys: []string{"海雾"}, Content: "沿西侧灯标靠岸。", Enabled: false},
	}}, {Name: "港务纪要", Entries: []domain.LorebookEntry{{Content: "午夜钟声响过两次。", Enabled: true}}}})
	for _, query := range []string{"龙渊港", "run", "deck"} {
		hits, err := s.SearchLorebook(tpl, query, 0, 1)
		if err != nil || len(hits) != 1 || hits[0].BookName != "航海手册" || hits[0].EntryIndex != 0 {
			t.Fatalf("query %q hits=%+v err=%v", query, hits, err)
		}
	}
	hits, err := s.SearchLorebook(tpl, "", 1, 1)
	if err != nil || len(hits) != 1 || hits[0].EntryIndex != 1 || hits[0].Entry.Enabled {
		t.Fatalf("disabled entry/page lost: %+v %v", hits, err)
	}
	hits, err = s.SearchLorebook(tpl, "", 2, 1)
	if err != nil || len(hits) != 1 || hits[0].BookIndex != 1 {
		t.Fatalf("duplicate/blank IDs displaced distinct entries: %+v %v", hits, err)
	}
	for _, query := range []string{`" OR *`, `'; DROP TABLE lorebook_entries; --`, "NOT NEAR AND"} {
		if _, err := s.SearchLorebook(tpl, query, 0, 5); err != nil {
			t.Fatalf("query syntax was interpreted: %q %v", query, err)
		}
	}
}

func TestLorebookIndexBackfillAndPartialRepair(t *testing.T) {
	s := newTestStore(t)
	tpl := seedLorebook(t, s, "repair", []domain.Lorebook{{Entries: []domain.LorebookEntry{
		{Title: "灯塔", Content: "第一幅海图"}, {Title: "海港", Content: "第二幅海图"},
	}}})
	// Simulate an older database without a derived index for this template.
	if _, err := s.db.Exec(`DELETE FROM lorebook_sources; DELETE FROM lorebook_entries`); err != nil {
		t.Fatal(err)
	}
	if err := s.ensureLorebookIndex(); err != nil {
		t.Fatal(err)
	}
	// Independent loss of one FTS row must be repaired even when other rows exist.
	if _, err := s.db.Exec(`DELETE FROM lorebook_fts WHERE rowid=(SELECT MIN(id) FROM lorebook_entries)`); err != nil {
		t.Fatal(err)
	}
	if err := s.ensureLorebookIndex(); err != nil {
		t.Fatal(err)
	}
	hits, err := s.SearchLorebook(tpl, "灯塔", 0, 10)
	if err != nil || len(hits) != 1 || hits[0].Entry.Content != "第一幅海图" {
		t.Fatalf("repair failed: %+v %v", hits, err)
	}
	if _, err := s.db.Exec(`DELETE FROM lorebook_entries WHERE template_version_id=? AND entry_index=1`, tpl); err != nil {
		t.Fatal(err)
	}
	if err := s.ensureLorebookIndex(); err != nil {
		t.Fatal(err)
	}
	hits, err = s.SearchLorebook(tpl, "", 0, 10)
	if err != nil || len(hits) != 2 {
		t.Fatalf("partial source repair failed: %+v %v", hits, err)
	}
}

func TestLorebookFallbackKeepsLiteralSemantics(t *testing.T) {
	s := newTestStore(t)
	tpl := seedLorebook(t, s, "fallback", []domain.Lorebook{{Entries: []domain.LorebookEntry{
		{Title: "铃", Content: "舱门标有 100%_[A]，Keeper 负责值夜。"},
	}}})
	s.loreFTS = false
	for _, query := range []string{"铃", "100%_[A]", "KEEPER"} {
		hits, err := s.SearchLorebook(tpl, query, 0, 10)
		if err != nil || len(hits) != 1 {
			t.Fatalf("fallback %q: %+v %v", query, hits, err)
		}
	}
	hits, err := s.SearchLorebook(tpl, "' OR 1=1", 0, 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("nonliteral fallback: %+v %v", hits, err)
	}
}

func TestLorebookIndexRollsBackWithSession(t *testing.T) {
	s := newTestStore(t)
	_, root, _ := seedSession(t, s)
	tpl := &domain.TemplateVersion{TemplateVersionID: "rejected-lore", Kind: domain.TemplateLorebook, Content: `[{"entries":[{"title":"不可见","content":"未提交的资料"}]}]`}
	session := &domain.Session{SessionID: "rejected-session", RootNodeID: root.NodeID, CreatedAt: time.Now()}
	branch := &domain.Branch{BranchID: "rejected-branch", SessionID: session.SessionID, HeadNodeID: root.NodeID}
	if err := s.CreateSession(session, root, branch, []*domain.TemplateVersion{tpl}); err == nil {
		t.Fatal("expected duplicate-root transaction failure")
	}
	hits, err := s.SearchLorebook(tpl.TemplateVersionID, "未提交", 0, 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("rolled-back content remains searchable: %+v %v", hits, err)
	}
	if _, err := s.GetTemplateVersion(tpl.TemplateVersionID); err == nil {
		t.Fatal("rolled-back template persisted")
	}
}
