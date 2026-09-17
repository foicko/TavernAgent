package application

import (
	"context"
	"strings"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/ports"
)

func TestExpandCardMacros(t *testing.T) {
	cases := []struct {
		in       string
		want     string
		charName string
		userName string
	}{
		{"{{char}}从不离开{{user}}。", "玛丽从不离开旅人。", "玛丽", "旅人"},
		{"{{ char }}与{{ USER }}", "玛丽与旅人", "玛丽", "旅人"},
		{"{{Char}}和{{User}}", "玛丽和旅人", "玛丽", "旅人"},
		{"没有宏的文本", "没有宏的文本", "玛丽", "旅人"},
		{"", "", "玛丽", "旅人"},
		{"{{unknown}}保留", "{{unknown}}保留", "玛丽", "旅人"},
	}
	for _, c := range cases {
		if got := expandCardMacros(c.in, c.charName, c.userName); got != c.want {
			t.Errorf("expandCardMacros(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestCharacterCardExpandMacros(t *testing.T) {
	raw := `{
		"name":"玛丽",
		"description":"{{char}}是巨人。{{user}}无法离开{{char}}。",
		"personality":"温柔",
		"scenario":"{{user}}和{{char}}住在云端。",
		"openingVariants":[{"title":"默认","text":"{{char}}看向{{user}}。"}],
		"secrets":[{"title":"秘密","content":"{{char}}的妹妹是{{user}}","revealWhen":"turn >= 3"}],
		"characters":[{"characterId":"npc_mary","name":"玛丽","description":"{{char}}爱{{user}}","participant":true}],
		"lorebookRefs":[{"name":"设定","entries":[{"entryId":"lbe_1","keys":["{{char}}"],"content":"{{char}}住在云端","enabled":true}]}]
	}`
	card, err := ParseCharacterCard(raw)
	if err != nil {
		t.Fatal(err)
	}
	card.ExpandMacros("旅人")

	if strings.Contains(card.Description, "{{") {
		t.Fatalf("description 未展开: %q", card.Description)
	}
	if !strings.Contains(card.Description, "玛丽是巨人") || !strings.Contains(card.Description, "旅人无法离开玛丽") {
		t.Fatalf("description 展开错误: %q", card.Description)
	}
	if !strings.Contains(card.Scenario, "旅人和玛丽") {
		t.Fatalf("scenario 展开错误: %q", card.Scenario)
	}
	if got := card.OpeningVariants[0].Text; got != "玛丽看向旅人。" {
		t.Fatalf("开场展开错误: %q", got)
	}
	if got := card.Secrets[0].Content; got != "玛丽的妹妹是旅人" {
		t.Fatalf("秘密展开错误: %q", got)
	}
	// 规则表达式不是文案，不能被替换。
	if card.Secrets[0].RevealWhen != "turn >= 3" {
		t.Fatalf("RevealWhen 被误改: %q", card.Secrets[0].RevealWhen)
	}
	if got := card.Characters[0].Description; got != "玛丽爱旅人" {
		t.Fatalf("角色描述展开错误: %q", got)
	}
	entry := card.LorebookRefs[0].Entries[0]
	if entry.Content != "玛丽住在云端" || entry.Keys[0] != "玛丽" {
		t.Fatalf("世界书条目展开错误: %+v", entry)
	}
}

// 宏展开必须发生在会话创建时：进入世界状态与开场正文的都已经是可读文本，
// 否则提示词里会原样出现「{{user}}」。
func TestSetupExpandsCardMacros(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	svc := NewSessionService(st)
	res, err := svc.Setup(context.Background(), &SessionSetupRequest{
		Title: "宏展开",
		CharacterJSON: `{"schemaVersion":2,"name":"玛丽","description":"{{char}}是巨人。",
			"characters":[{"characterId":"npc_mary","name":"玛丽","description":"{{char}}从不让{{user}}离开。","participant":true}],
			"openingVariants":[{"variantId":"open_a","title":"默认","text":"{{char}}看向{{user}}。"}]}`,
		Player:           Player{Name: "旅人"},
		OpeningVariantID: "open_a",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if res.OpeningText != "玛丽看向旅人。" {
		t.Fatalf("开场正文未展开: %q", res.OpeningText)
	}
	info, ok := res.State.Characters["npc_mary"]
	if !ok {
		t.Fatalf("角色未进入世界状态: %+v", res.State.Characters)
	}
	if info.Description != "玛丽从不让旅人离开。" {
		t.Fatalf("世界状态里的人设未展开: %q", info.Description)
	}
}
