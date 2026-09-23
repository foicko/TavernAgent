package context

import (
	"strings"
	"testing"

	"tavernagent/internal/domain"
)

func anchorTestState() *domain.WorldState {
	return &domain.WorldState{Characters: map[string]domain.CharacterInfo{
		"npc_a": {CharacterID: "npc_a", Name: "阿澜", Participant: true,
			Personality: "嘴硬心软，句尾爱带“哼”", Scenario: "雨夜的旧书店",
			PostHistory: "始终称玩家为“小鬼”"},
	}}
}

// personality / scenario 必须进入角色设定：此前它们从未进入上下文。
func TestCharacterProfilesIncludePersonalityAndScenario(t *testing.T) {
	got := renderCharacterProfiles(anchorTestState(), false, false)
	for _, want := range []string{"嘴硬心软", "雨夜的旧书店", "始终称玩家为"} {
		if !strings.Contains(got, want) {
			t.Fatalf("角色设定缺少 %q：\n%s", want, got)
		}
	}
}

// 分离模式下 post_history 只出现在尾部锚点，不重复进静态前缀。
func TestPostHistoryMovesToTailAnchor(t *testing.T) {
	st := anchorTestState()
	if strings.Contains(renderCharacterProfiles(st, false, true), "始终称玩家为") {
		t.Fatal("分离模式下 post_history 不应出现在静态前缀")
	}
	anchors := renderCharacterAnchors(st)
	for _, want := range []string{CharacterAnchorHeader, "阿澜", "嘴硬心软", "始终称玩家为"} {
		if !strings.Contains(anchors, want) {
			t.Fatalf("角色锚点缺少 %q：\n%s", want, anchors)
		}
	}
}

func TestCharacterAnchorTruncatesLongPersonality(t *testing.T) {
	st := &domain.WorldState{Characters: map[string]domain.CharacterInfo{
		"npc_b": {Name: "长", Participant: true, Personality: strings.Repeat("字", 1000)},
	}}
	if n := strings.Count(renderCharacterAnchors(st), "字"); n != anchorPersonalityRunes {
		t.Fatalf("锚点性格摘录应截断为 %d 字，实际 %d", anchorPersonalityRunes, n)
	}
}
