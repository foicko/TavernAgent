// 场景/里程碑/目标/导演/记忆/秘密类事件的应用。
//
// 从 state.go 的 ApplyEvent 拆出。其中记忆与导演事件是**空操作**：它们各自有独立
// 投影（C04 认知与状态分离），世界状态哈希必须保持不变——这一点由 case 分支里的
// 注释与重放等价测试共同钉住，拆文件时原样保留。
package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// isDirectorEvent 判定事件是否属于本域（与下面 switch 的 case 集合必须一致）。
func isDirectorEvent(t EventType) bool {
	switch t {
	case EventScenePropose, EventMilestonePropose, EventGoalSet, EventMemoryAdd, EventMemoryRevised, EventMemoryPinned, EventMemoryHidden, EventDirectorChange, EventSecretUnlock:
		return true
	}
	return false
}

// applyDirectorEvent 应用本域事件，返回值契约与 ApplyEvent 相同。
func applyDirectorEvent(s *WorldState, ev *DomainEvent) (string, error) {
	switch ev.Type {
	case EventScenePropose:
		var p ScenePayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		if p.Scene.SceneID == "" {
			return "", fmt.Errorf("scene id required")
		}
		s.Scene = &p.Scene
		return "scene", nil
	case EventMilestonePropose:
		var p MilestonePayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		if p.MilestoneID == "" || strings.TrimSpace(p.Description) == "" {
			return "", fmt.Errorf("milestone id and description required")
		}
		s.Milestones[p.MilestoneID] = p.Description
		return "milestone", nil
	case EventMemoryAdd, EventMemoryRevised, EventMemoryPinned, EventMemoryHidden:
		return "memory", nil // 记忆不进入世界状态投影（C04：认知与状态分离）
	case EventDirectorChange:
		return "director", nil // Intentions have their own projection; world hashes stay unchanged.
	case EventGoalSet:
		var p struct {
			GoalID      string `json:"goalId"`
			CharacterID string `json:"characterId"`
			Text        string `json:"text"`
		}
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		if _, ok := s.Characters[p.CharacterID]; !ok {
			return "", fmt.Errorf("unknown goal character")
		}
		if p.GoalID == "" {
			p.GoalID = "goal_" + p.CharacterID
		}
		s.Goals[p.GoalID] = Goal{CharacterID: p.CharacterID, Text: p.Text}
		return "goal", nil
	case EventSecretUnlock:
		var p SecretUnlockPayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		s.UnlockSecret(p.SecretID)
		return "secret:" + p.SecretID, nil
	}
	return "", fmt.Errorf("event %q has no director applier", ev.Type)
}
