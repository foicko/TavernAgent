package context

import (
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"testing"
)

func TestCurrentTopicBeatsRecentHighImportanceIncidentalMatch(t *testing.T) {
	state := domain.NewWorldState()
	state.Characters["guide"] = domain.CharacterInfo{CharacterID: "guide", Name: "向导", Aliases: []string{"灯姐"}, Participant: true}
	candidates := []*ports.MemoryCandidate{
		{Memory: &domain.MemoryRecord{MemoryID: "key", Content: "钥匙留在东边抽屉。", EntityIDs: []string{"guide"}, Importance: 3}, HasRank: true, Rank: -0.016, SourceTurn: 1},
		{Memory: &domain.MemoryRecord{MemoryID: "lamp", Content: "灯罩留在楼上。", EntityIDs: []string{"guide"}, Importance: 9}, HasRank: true, Rank: -0.017, SourceTurn: 30},
	}
	compiler := New(nil, DefaultOptions())
	query := "灯姐用来解锁的小工具在哪儿？"
	got := compiler.selectMemories(candidates, effectiveOf(candidates), state, query+"昨天谈过灯罩", 30, query)
	if len(got) != 2 || got[0].MemoryID != "key" {
		t.Fatalf("topic lost to incidental match: %v", idsOf(got))
	}
}
