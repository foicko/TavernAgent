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

// 重叠的名字（"莉尔"/"莉尔娜"）必须长的先剥：此前按 map 顺序剥，先剥短名会
// 留下"娜"残片，让含"娜的"的无关记忆时有时无地得分。
func TestCurrentMemoryRelevanceStripsOverlappingNamesDeterministically(t *testing.T) {
	state := domain.NewWorldState()
	state.Characters["npc_a"] = domain.CharacterInfo{CharacterID: "npc_a", Name: "莉尔"}
	state.Characters["npc_b"] = domain.CharacterInfo{CharacterID: "npc_b", Name: "莉尔娜"}
	candidates := []*ports.MemoryCandidate{
		{Memory: &domain.MemoryRecord{MemoryID: "leftover", Content: "娜的旧事。"}},
		{Memory: &domain.MemoryRecord{MemoryID: "sword", Content: "的剑放好了。"}},
		{Memory: &domain.MemoryRecord{MemoryID: "other", Content: "天气晴朗。"}},
	}
	for i := 0; i < 64; i++ {
		scores := currentMemoryRelevance(candidates, "莉尔娜的剑在哪", state)
		if scores["leftover"] != 0 {
			t.Fatalf("第 %d 次：名字残片参与了打分：%v", i, scores)
		}
		if scores["sword"] != 1 {
			t.Fatalf("第 %d 次：真正的话题应得满分：%v", i, scores)
		}
	}
}
