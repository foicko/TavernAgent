package context

import (
	"fmt"
	"strings"

	"tavernagent/internal/domain"
)

// Both prompt layouts share the same authoritative current status. These fields
// must survive when the dialogue that originally established them is folded.
func renderCharacterState(state *domain.WorldState) string {
	if state == nil {
		return ""
	}
	var b strings.Builder
	name := func(id string) string {
		if ch, ok := state.Characters[id]; ok && ch.Name != "" {
			return oneLine(ch.Name)
		}
		return oneLine(id)
	}
	if len(state.Relationships) > 0 {
		b.WriteString("当前关系（离散档位）：\n")
		for _, id := range sortedKeys(state.Relationships) {
			r := state.Relationships[id]
			fmt.Fprintf(&b, "- %s：好感%d，信任%d，戒备%d\n", name(id), r.Affection, r.Trust, r.Alertness)
		}
	}
	if len(state.Moods) > 0 {
		b.WriteString("当前人物情绪（以状态为准）：\n")
		for _, id := range sortedKeys(state.Moods) {
			mood := state.Moods[id]
			fmt.Fprintf(&b, "- %s：%s，%s\n", name(id), oneLine(mood.MoodCode), oneLine(mood.Text))
		}
	}
	if len(state.Goals) > 0 {
		b.WriteString("当前人物目标（以状态为准）：\n")
		for _, id := range sortedKeys(state.Goals) {
			goal := state.Goals[id]
			fmt.Fprintf(&b, "- %s：%s\n", name(goal.CharacterID), oneLine(goal.Text))
		}
	}
	if state.Scene != nil {
		fmt.Fprintf(&b, "当前场景：%s（场景 %s，地点 %s）\n", oneLine(state.Scene.Title), oneLine(state.Scene.SceneID), oneLine(state.Scene.LocationID))
	}
	return b.String()
}
