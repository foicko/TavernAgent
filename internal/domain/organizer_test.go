package domain

import (
	"fmt"
	"testing"
)

func organizationFixture(n int) []*MemoryRecord {
	var out []*MemoryRecord
	for i := 0; i < n; i++ {
		out = append(out, &MemoryRecord{MemoryID: fmt.Sprintf("m%03d", i), SourceNodeID: "old", Kind: MemoryObserved, Content: fmt.Sprintf("雨夜里酒馆的红葡萄酒售价讨论第%d次。", i), EntityIDs: []string{"npc"}, Importance: 3})
	}
	return out
}

func TestMemoryHeadroomBoundaries(t *testing.T) {
	for _, tc := range []struct {
		n    int
		tier string
	}{{0, "Normal"}, {49, "Normal"}, {50, "Notice"}, {64, "Notice"}, {65, "Degraded"}, {74, "Degraded"}, {75, "Critical"}, {80, "Critical"}, {81, "Critical"}} {
		t.Run(fmt.Sprint(tc.n), func(t *testing.T) {
			u := ScanMemoryUsage(organizationFixture(tc.n), NewWorldState())
			if u.Tier != tc.tier || u.Headroom != 80-tc.n {
				t.Fatalf("usage=%+v", u)
			}
		})
	}
}

func TestOrganizerProtectsKeepsakesPromisesAndSecrets(t *testing.T) {
	s := NewWorldState()
	s.Items["watch"] = ItemInstance{Name: "银色怀表", Keepsake: true, Quantity: 1}
	s.Promises["pledge"] = Promise{State: PromiseActive, Content: "日落前在钟楼汇合"}
	for _, m := range []*MemoryRecord{
		{Content: "还未送出的银色怀表"}, {EntityIDs: []string{"watch"}},
		{Content: "日落前在钟楼汇合的约定"}, {EntityIDs: []string{"pledge"}},
		{Kind: MemorySecret}, {SecretID: "secret"}, {Pinned: true},
	} {
		if !MemoryProtected(m, s) {
			t.Fatalf("unprotected: %+v", m)
		}
	}
	if MemoryProtected(&MemoryRecord{Content: "酒馆红酒价格"}, s) {
		t.Fatal("ordinary observation protected")
	}
}

func TestOrganizerGateAndBranchOverlay(t *testing.T) {
	records := organizationFixture(80)
	s := NewWorldState()
	plan := PlanMemoryOrganization(records, s)
	if len(plan.Merges) == 0 {
		t.Fatal("critical tier did not plan merges")
	}
	changes, err := BuildOrganizedMemories(plan, records, s, "new")
	if err != nil {
		t.Fatal(err)
	}
	u := ScanMemoryUsage(append(append([]*MemoryRecord{}, records...), changes...), s)
	if u.Used >= 65 {
		t.Fatalf("organizer did not make room: %+v", u)
	}
	if old := ScanMemoryUsage(records, s); old.Used != 80 {
		t.Fatal("mutated shared memories")
	}
	for _, m := range changes {
		if m.SourceNodeID != "new" {
			t.Fatal("wrong overlay anchor")
		}
	}
	// An independent gate must catch a source becoming protected after planning.
	records[0].Pinned = true
	if GateMemoryOrganization(plan, records, s) == nil {
		t.Fatal("planner bypassed protection gate")
	}
	records[0].Pinned = false
	plan.Merges[0].Content = "编造的新世界事实"
	if GateMemoryOrganization(plan, records, s) == nil {
		t.Fatal("invented summary passed gate")
	}
}

func TestOrganizerDoesNotMixPerspectivesOrKinds(t *testing.T) {
	records := organizationFixture(80)
	for i, m := range records {
		m.OwnerIDs = []string{fmt.Sprintf("owner%d", i)}
		if i%2 == 0 {
			m.Kind = MemoryInferred
		}
	}
	if p := PlanMemoryOrganization(records, NewWorldState()); len(p.Merges) != 0 {
		t.Fatal("merged separate perspectives")
	}
}
