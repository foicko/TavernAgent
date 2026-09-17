package context

import (
	"encoding/json"
	"strings"
	"testing"

	"tavernagent/internal/domain"
)

func TestReleaseDirectorSeesRecentStoryAlongsideOlderSummary(t *testing.T) {
	c := New(nil, DefaultOptions()).ForDirectorDiscussion("你是导演助手。", nil)
	content, err := json.Marshal(domain.TurnContent{
		InputText: "检查当前的通行证",
		Blocks:    []domain.TextBlock{{Kind: "narration", Text: "最新事实：通行证已经被守卫收走。"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	history := []*domain.PlotNode{{NodeID: "recent", Kind: domain.NodeKindTurn, ContentJSON: string(content)}}
	summaries := []*domain.SummaryArtifact{{
		FromNodeID: "older-start", ToNodeID: "older-end",
		Text: "<story_checkpoint><narrative_arc>早先旅人获得了通行证。</narrative_arc><open_loops>进入城门。</open_loops></story_checkpoint>",
	}}
	messages := c.buildMessages(nil, nil, history, summaries, nil, sessionContext{}, domain.NewWorldState(), "接下来如何安排？", "", 0)
	var prompt strings.Builder
	for _, message := range messages {
		prompt.WriteString(message.Content)
	}
	t.Logf("older fact present=%v latest fact present=%v", strings.Contains(prompt.String(), "早先旅人获得了通行证"), strings.Contains(prompt.String(), "通行证已经被守卫收走"))
	if !strings.Contains(prompt.String(), "通行证已经被守卫收走") {
		t.Fatal("adding an older summary removed the current, uncovered story facts from the director prompt")
	}
}
