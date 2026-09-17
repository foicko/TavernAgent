package context_test

import (
	"encoding/json"
	"strings"
	"testing"

	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
)

func TestBuildCompactionChatRequest(t *testing.T) {
	tc1 := domain.TurnContent{
		InputText: "你好，请问这里是哪里？",
		Blocks: []domain.TextBlock{
			{Kind: "dialogue", Text: "这里是旧城区的迷雾钟楼。"},
			{Kind: "inner_monologue", Text: "看来他还没恢复记忆。"},
		},
	}
	tc1JSON, _ := json.Marshal(tc1)

	nodes := []*domain.PlotNode{
		{
			NodeID:      "turn_1",
			Kind:        domain.NodeKindTurn,
			TurnNumber:  1,
			ContentJSON: string(tc1JSON),
		},
	}

	prevSummary := &domain.SummaryArtifact{
		Text: "<story_checkpoint><narrative_arc>第一幕剧情</narrative_arc></story_checkpoint>",
	}

	req := ctxpkg.BuildCompactionChatRequest("沙织", "玩家", "故事开端于一个雨夜。", prevSummary, nodes, nil)

	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}
	sys := req.Messages[0].Content
	if !strings.Contains(sys, "<story_checkpoint>") || !strings.Contains(sys, "UNTRUSTED DATA") {
		t.Errorf("system prompt missing schema or security tags:\n%s", sys)
	}

	userMsg := req.Messages[1].Content
	if !strings.Contains(userMsg, "前序剧情交接快照") || !strings.Contains(userMsg, "迷雾钟楼") {
		t.Errorf("user message missing expected context:\n%s", userMsg)
	}
}

func TestParseCompactionSummary_Valid(t *testing.T) {
	raw := "```xml\n<story_checkpoint>\n<narrative_arc>主要剧情推进。</narrative_arc>\n<character_dynamics>\n<mindset character=\"沙织\">暗中戒备</mindset>\n</character_dynamics>\n<open_loops>\n- [承诺] 明早离开\n</open_loops>\n</story_checkpoint>\n```"

	clean, err := ctxpkg.ParseCompactionSummary(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(clean, "<story_checkpoint>") || !strings.HasSuffix(clean, "</story_checkpoint>") {
		t.Errorf("unexpected parsed output:\n%s", clean)
	}
	if strings.Contains(clean, "```") {
		t.Errorf("markdown code fence was not stripped:\n%s", clean)
	}
}

func TestParseCompactionSummary_Invalid(t *testing.T) {
	// 缺少 open_loops
	rawMissingOpenLoops := "<story_checkpoint><narrative_arc>内容</narrative_arc></story_checkpoint>"
	_, err := ctxpkg.ParseCompactionSummary(rawMissingOpenLoops)
	if err == nil {
		t.Errorf("expected error for missing open_loops, got nil")
	}

	// 缺少根标签
	rawNoRoot := "一些闲聊文字：<narrative_arc>只有子标签</narrative_arc>"
	_, err = ctxpkg.ParseCompactionSummary(rawNoRoot)
	if err == nil {
		t.Errorf("expected error for missing root tag, got nil")
	}
}
