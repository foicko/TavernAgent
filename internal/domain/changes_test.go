package domain

import (
	"encoding/json"
	"testing"
)

func payloadOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(b)
}

func eventOf(t *testing.T, kind EventType, payload any) *DomainEvent {
	t.Helper()
	return &DomainEvent{EventID: "e", Type: kind, PayloadJSON: payloadOf(t, payload)}
}

// SummarizeChanges 是"这一轮世界变了什么"的唯一来源，它只允许由事件推导。
// 这里钉住三件事：显示**实际生效**的量、跳过不该出现的条目、读得懂人话。
func TestSummarizeChanges(t *testing.T) {
	state := NewWorldState()
	state.Characters["npc_a"] = CharacterInfo{CharacterID: "npc_a", Name: "艾琳娜"}
	// 注意：摘要要的是**事件生效之后**的状态，所以这里从变化前开始，
	// 后面统一走 ApplyEvents——与提交路径（SummarizeChanges(pd.Events, pd.NewState)）一致。
	state.Relationships["npc_a"] = RelationValue{Affection: 28, Trust: 5}
	state.Items["it_1"] = ItemInstance{InstanceID: "it_1", Name: "铜钥匙", OwnerID: "player", Quantity: 2}

	events := []*DomainEvent{
		// 提议 +5，实际只生效 +2（被上限夹断）：必须显示实际生效的那个数。
		eventOf(t, EventRelationshipDelta, RelationshipDeltaPayload{CharacterID: "npc_a", Field: "affection", Delta: 5, Applied: 2}),
		eventOf(t, EventMoodSet, map[string]any{"characterId": "npc_a", "mood": CharacterMood{MoodCode: "wary", Text: "表面平静，指尖发凉"}}),
		eventOf(t, EventGoalSet, map[string]any{"goalId": "g1", "characterId": "npc_a", "text": "找回遗失的地图"}),
		eventOf(t, EventPromisePropose, PromiseProposePayload{Promise: Promise{PromiseID: "pr1", Content: "明天正午在桥头会面"}}),
		eventOf(t, EventItemGrant, ItemGrantPayload{Item: ItemInstance{InstanceID: "it_2", Name: "旧信", Quantity: 1}}),
		eventOf(t, EventItemConsume, ItemTransferPayload{ItemID: "it_1", From: "player", Quantity: 1}),
		eventOf(t, EventScenePropose, ScenePayload{Scene: Scene{SceneID: "s1", Title: "钟楼二层"}}),
		eventOf(t, EventSecretUnlock, SecretUnlockPayload{SecretID: "sec1", Title: "她的旧名"}),
		// 记忆与导演改动不进这一层：记忆有自己的面板与证据链。
		eventOf(t, EventMemoryAdd, MemoryAddPayload{Memory: MemoryRecord{MemoryID: "m1", Content: "她怕冷"}}),
		// 被夹到 0 的变化不是变化，不该凑成条目。
		eventOf(t, EventRelationshipDelta, RelationshipDeltaPayload{CharacterID: "npc_a", Field: "trust", Delta: 0, Applied: 0}),
	}
	if _, err := ApplyEvents(state, events); err != nil {
		t.Fatalf("apply events: %v", err)
	}

	got := SummarizeChanges(events, state)
	want := []StateChange{
		{Kind: "relationship", Label: "艾琳娜 · 好感", Text: "+2（28 → 30）", Delta: 2},
		{Kind: "mood", Label: "艾琳娜 · 心情", Text: "表面平静，指尖发凉"},
		{Kind: "goal", Label: "艾琳娜 · 目标", Text: "找回遗失的地图"},
		{Kind: "promise", Label: "约定", Text: "明天正午在桥头会面"},
		{Kind: "item", Label: "物品", Text: "获得「旧信」×1", Delta: 1},
		{Kind: "item", Label: "物品", Text: "消耗「铜钥匙」×1", Delta: -1},
		{Kind: "scene", Label: "场景", Text: "钟楼二层"},
		{Kind: "secret", Label: "秘密", Text: "解锁「她的旧名」"},
	}
	if len(got) != len(want) {
		t.Fatalf("条目数 = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 条 = %+v, want %+v", i+1, got[i], want[i])
		}
	}
}

// 没有可展示的变化时返回 nil（前端据此不渲染这一层），而不是一个空切片。
func TestSummarizeChangesEmptyCases(t *testing.T) {
	if got := SummarizeChanges(nil, NewWorldState()); got != nil {
		t.Fatalf("无事件应返回 nil，得到 %+v", got)
	}
	state := NewWorldState()
	only := []*DomainEvent{eventOf(t, EventMemoryAdd, MemoryAddPayload{Memory: MemoryRecord{MemoryID: "m"}})}
	if got := SummarizeChanges(only, state); got != nil {
		t.Fatalf("只有记忆事件应返回 nil，得到 %+v", got)
	}
	// 状态缺失时宁可什么都不展示，也不能靠猜造出变化。
	if got := SummarizeChanges(only, nil); got != nil {
		t.Fatalf("状态缺失应返回 nil，得到 %+v", got)
	}
}

// 角色名与物品名缺失时退回到 ID：显示 ID 很丑，但比显示"某人"要诚实。
func TestSummarizeChangesFallsBackToIDs(t *testing.T) {
	state := NewWorldState()
	rel := eventOf(t, EventRelationshipDelta, RelationshipDeltaPayload{CharacterID: "npc_ghost", Field: "trust", Delta: 3, Applied: 3})
	if _, err := ApplyEvent(state, rel); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got := SummarizeChanges([]*DomainEvent{
		rel,
		eventOf(t, EventItemConsume, ItemTransferPayload{ItemID: "it_ghost", Quantity: 1}),
	}, state)
	if len(got) != 2 {
		t.Fatalf("条目数 = %d, want 2", len(got))
	}
	if got[0].Label != "npc_ghost · 信任" || got[0].Text != "+3（0 → 3）" {
		t.Fatalf("关系条目 = %+v", got[0])
	}
	if got[1].Text != "消耗「it_ghost」×1" {
		t.Fatalf("物品条目 = %+v", got[1])
	}
}
