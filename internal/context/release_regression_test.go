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

// 次要关键词全部短于下限被忽略时，条目应按"没有次要关键词"处理，而不是永远无法触发。
func TestLorebookEntryWithOnlyShortSecondaryKeysStillTriggers(t *testing.T) {
	e := domain.LorebookEntry{Keys: []string{"古堡"}, SecondaryKeys: []string{"门", "夜"}}
	if !entryMatches(e, "我们走向古堡。", 2) {
		t.Fatal("次要关键词全部不可用时条目被静默锁死")
	}
	if entryMatches(e, "我们走向森林。", 2) {
		t.Fatal("主关键词未命中不应触发")
	}
	// 仍有可用次要关键词时照常要求命中其一。
	e.SecondaryKeys = []string{"门", "夜晚"}
	if entryMatches(e, "我们走向古堡。", 2) {
		t.Fatal("存在可用次要关键词却未命中时不应触发")
	}
	if !entryMatches(e, "夜晚我们走向古堡。", 2) {
		t.Fatal("命中可用次要关键词时应触发")
	}
}
