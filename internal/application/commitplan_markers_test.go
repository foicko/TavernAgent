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
