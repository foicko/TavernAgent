package application

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"tavernagent/internal/domain"
)

func memoryIDs(records []*domain.MemoryRecord) []string {
	ids := make([]string, 0, len(records))
	for _, m := range records {
		ids = append(ids, m.MemoryID)
	}
	sort.Strings(ids)
	return ids
}

func assertMemoryProjection(t *testing.T, f *memoryFixture, node string) []*domain.MemoryRecord {
	t.Helper()
	raw, err := pathMemories(f.store, node)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.store.ProjectedMemories(node)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(memoryIDs(got), memoryIDs(domain.ApplyMemoryOverlays(raw))) {
		t.Fatalf("projection at %s differs from replay: %v / %v", node, memoryIDs(got), memoryIDs(domain.ApplyMemoryOverlays(raw)))
	}
	return got
}

func TestMemoryProjectionBranchesRevisionsAndSubjectRevival(t *testing.T) {
	f := newMemoryFixture(t)
	old := memRec("old", "守桥人仍在桥边")
	old.SubjectKey = "keeper.location"
	old.CreatedTurn = 1
	newer := memRec("newer", "守桥人在灯塔")
	newer.SubjectKey = old.SubjectKey
	newer.CreatedTurn = 2
	a := f.commitOn(t, f.main.BranchID, "root", old, newer)
	if got := assertMemoryProjection(t, f, a); len(got) != 1 || got[0].MemoryID != "newer" {
		t.Fatal(got)
	}
	f.forkBranch(t, "other", a)
	revision := memRec("revision", "这是船长的位置")
	revision.Supersedes = "newer"
	revision.SubjectKey = "captain.location"
	revision.CreatedTurn = 3
	b := f.commitOn(t, f.main.BranchID, a, revision)
	if got := assertMemoryProjection(t, f, b); len(got) != 2 {
		t.Fatalf("independent subject loser must revive: %v", memoryIDs(got))
	}
	if got := assertMemoryProjection(t, f, a); len(got) != 1 {
		t.Fatal("sibling changed")
	}
	hidden := memRec("hidden", "这是船长的位置")
	hidden.Supersedes = "revision"
	hidden.Hidden = true
	c := f.commitOn(t, f.main.BranchID, b, hidden)
	assertMemoryProjection(t, f, c)
	// Legacy writes at an existing node invalidate every derived checkpoint.
	old.Content = "旧式修订后的桥边记录"
	if err := f.store.UpdateMemory(old); err != nil {
		t.Fatal(err)
	}
	for _, node := range []string{a, b, c} {
		assertMemoryProjection(t, f, node)
	}
	got, _ := f.store.ProjectedMemories(b)
	for _, m := range got {
		if m.MemoryID == "old" && m.Content != old.Content {
			t.Fatal("stale cache after mutation")
		}
	}
	got[0].Content = "caller mutation"
	again, _ := f.store.ProjectedMemories(b)
	for _, m := range again {
		if m.Content == "caller mutation" {
			t.Fatal("cache escaped by reference")
		}
	}
}

func TestMemoryProjectionCheckpointsMatchFullReplay(t *testing.T) {
	f := newMemoryFixture(t)
	head := "root"
	previous := ""
	for i := 0; i < 140; i++ {
		m := memRec(fmt.Sprintf("r%03d", i), fmt.Sprintf("第 %d 次修订", i))
		m.Supersedes = previous
		m.CreatedTurn = i
		head = f.commitOn(t, f.main.BranchID, head, m)
		previous = m.MemoryID
		if i%5 == 0 {
			assertMemoryProjection(t, f, head)
		}
	}
	got := assertMemoryProjection(t, f, head)
	if len(got) != 1 || got[0].MemoryID != previous {
		t.Fatal(memoryIDs(got))
	}
}

func TestMemoryPaginationPinsNodeAndFiltersBeforePaging(t *testing.T) {
	f := newMemoryFixture(t)
	memories := make([]*domain.MemoryRecord, 0, 95)
	for i := 0; i < 95; i++ {
		m := memRec(fmt.Sprintf("m%03d", i), "灯塔守卫的记录")
		m.CreatedTurn = i
		memories = append(memories, m)
	}
	hidden := memRec("hidden", "隐藏灯塔记录")
	hidden.Hidden = true
	secret := memRec("secret", "未揭露的秘密")
	secret.Kind = domain.MemorySecret
	secret.SecretID = "sealed"
	head := f.commitOn(t, f.main.BranchID, "root", append(memories, hidden, secret)...)
	page, err := f.svc.ListPageAt(f.sess.SessionID, f.main.BranchID, "", MemoryQuery{Kind: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Memories) != 40 || page.Total != 95 || page.NextCursor == "" || page.NodeID != head {
		t.Fatalf("page=%+v", page)
	}
	f.commitOn(t, f.main.BranchID, head, memRec("new-head", "后台反思记录"))
	seen := map[string]bool{}
	for _, m := range page.Memories {
		seen[m.MemoryID] = true
	}
	for page.NextCursor != "" {
		page, err = f.svc.ListPageAt(f.sess.SessionID, f.main.BranchID, "", MemoryQuery{Kind: "all", Cursor: page.NextCursor})
		if err != nil {
			t.Fatal(err)
		}
		if page.NodeID != head || page.Total != 95 {
			t.Fatal("cursor followed moving head")
		}
		for _, m := range page.Memories {
			if seen[m.MemoryID] {
				t.Fatal("duplicate row")
			}
			seen[m.MemoryID] = true
		}
	}
	if len(seen) != 95 {
		t.Fatal(len(seen))
	}
	first, _ := f.svc.ListPageAt(f.sess.SessionID, f.main.BranchID, head, MemoryQuery{Limit: 1, Kind: "all"})
	if _, err := f.svc.ListPageAt(f.sess.SessionID, f.main.BranchID, head, MemoryQuery{Kind: "hidden", Cursor: first.NextCursor}); err == nil {
		t.Fatal("cross-query cursor accepted")
	}
	filtered, err := f.svc.ListPageAt(f.sess.SessionID, f.main.BranchID, head, MemoryQuery{Kind: "hidden", Search: "灯塔"})
	if err != nil || len(filtered.Memories) != 1 || filtered.Memories[0].MemoryID != "hidden" {
		t.Fatalf("hidden recovery view: %+v %v", filtered, err)
	}
	for _, m := range first.Memories {
		if m.MemoryID == "secret" {
			t.Fatal("secret leaked")
		}
	}
}
