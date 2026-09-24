package application

import (
	"testing"

	"tavernagent/internal/domain"
	"tavernagent/internal/protocol"
)

func TestPreparePlanExtractsUIHints(t *testing.T) {
	draft := protocol.TurnDraft{Blocks: []protocol.BlockFrame{
		{Kind: protocol.BlockNarration, Text: "雪停了。[SCENE:教堂]"},
		{Kind: protocol.BlockDialogue, Text: "你好。[HUD:理智=80|体力=55]"},
	}}
	blocks, _, hints, _, _ := preparePlanBlocksAndOptions(draft, domain.TurnInput{}, domain.DefaultRuleset(), false)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d", len(blocks))
	}
	if blocks[0].Text != "雪停了。" || blocks[1].Text != "你好。" {
		t.Errorf("markers not stripped: %q / %q", blocks[0].Text, blocks[1].Text)
	}
	if len(hints) != 2 {
		t.Fatalf("hints = %+v", hints)
	}
	if hints[0].Kind != "scene" || hints[0].Text != "教堂" {
		t.Errorf("scene hint = %+v", hints[0])
	}
	if hints[1].Kind != "hud" || hints[1].Values["理智"] != "80" {
		t.Errorf("hud hint = %+v", hints[1])
	}
}

func TestVarEventsFromHintsAreSorted(t *testing.T) {
	hints := []domain.UIHint{
		{Kind: "hud", Values: map[string]string{"x": "1"}},
		{Kind: "var", Values: map[string]string{"b": "2", "a": "1"}},
	}
	evs := varEventsFromHints(hints)
	if len(evs) != 2 {
		t.Fatalf("events = %d, want 2 (hud must be ignored)", len(evs))
	}
	// 顺序确定：a 在 b 之前（map 迭代无序，必须排序）。
	if evs[0].PayloadJSON != `{"key":"a","value":"1"}` || evs[1].PayloadJSON != `{"key":"b","value":"2"}` {
		t.Fatalf("payloads not sorted: %s / %s", evs[0].PayloadJSON, evs[1].PayloadJSON)
	}
	if evs[0].Type != domain.EventVarSet {
		t.Fatalf("type = %s", evs[0].Type)
	}
}

func TestPreparePlanStripsOptionMarkers(t *testing.T) {
	draft := protocol.TurnDraft{
		Options: []protocol.OptionFrame{
			{OptionID: "o1", Intent: "clever", Text: "上前询问 [HUD:理智=70]"},
		},
	}
	_, options, hints, _, _ := preparePlanBlocksAndOptions(draft, domain.TurnInput{}, domain.DefaultRuleset(), false)
	if len(options) != 1 || options[0].Text != "上前询问" {
		t.Fatalf("option markers not stripped: %+v", options)
	}
	// 选项里的标记不生成提示：选项只是提案，不驱动 HUD/变量。
	if len(hints) != 0 {
		t.Errorf("option markers must not produce hints: %+v", hints)
	}
}

func TestPreparePlanKeepsImageMarkers(t *testing.T) {
	draft := protocol.TurnDraft{Blocks: []protocol.BlockFrame{
		{Kind: protocol.BlockNarration, Text: `她笑了。<img cmd="smile">`},
	}}
	blocks, _, hints, _, _ := preparePlanBlocksAndOptions(draft, domain.TurnInput{}, domain.DefaultRuleset(), false)
	if blocks[0].Text != `她笑了。<img cmd="smile">` {
		t.Errorf("image marker must stay: %q", blocks[0].Text)
	}
	if len(hints) != 0 {
		t.Errorf("unexpected hints: %+v", hints)
	}
}
