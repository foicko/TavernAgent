package context_test

import (
	"strings"
	"testing"

	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
)

func TestCurrentCharacterStateSurvivesBothPromptLayouts(t *testing.T) {
	for _, split := range []bool{false, true} {
		opts := ctxpkg.DefaultOptions()
		opts.SplitDynamicContext = split
		f := newFixture(t, nil, opts)
		state := domain.NewWorldState()
		state.Characters["guide"] = domain.CharacterInfo{CharacterID: "guide", Name: "向导", Participant: true}
		state.Relationships["guide"] = domain.RelationValue{Trust: 45}
		state.Moods["guide"] = domain.CharacterMood{MoodCode: "worried", Text: "担心错过渡船"}
		state.Goals["trip"] = domain.Goal{CharacterID: "guide", Text: "日落前抵达码头"}
		state.Scene = &domain.Scene{SceneID: "harbor", Title: "雾港", LocationID: "pier"}
		var text strings.Builder
		for _, message := range f.compileMessages(t, testRootID, "继续", state) {
			text.WriteString(message.Content)
		}
		for _, required := range []string{"信任45", "担心错过渡船", "日落前抵达码头", "雾港", "pier"} {
			if !strings.Contains(text.String(), required) {
				t.Errorf("split=%v omitted %q", split, required)
			}
		}
	}
}
