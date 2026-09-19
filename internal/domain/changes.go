package domain

import (
	"encoding/json"
	"fmt"
)

// StateChange 是"这一轮世界变了什么"的一条可读摘要。
//
// 只能由提交方从**已生效的领域事件**推导，绝不由模型书写：这层展示的用途是让读者
// 核验"这次演绎凭什么这样走"，一旦摘要与真正落库的事件不一致，它就从证据变成了掩护。
type StateChange struct {
	Kind  string `json:"kind"`            // relationship | mood | goal | promise | item | scene | milestone | secret
	Label string `json:"label"`           // 针对谁/哪一项，如"艾琳娜 · 好感"
	Text  string `json:"text"`            // 变化内容，如"+2（25 → 27）"
	Delta int    `json:"delta,omitempty"` // 有方向的变化量，供前端上色
}

// MemoryRef 是"本轮参考了哪条记忆"的展示引用。
//
// 存**当时的文本快照**而不是只存 ID：记忆会被修订（copy-on-write）或隐藏，只存 ID
// 的话，回看旧回合时看到的是"记忆被改写之后的样子"，而不是当时真正影响这次演绎的
// 那句话——那等于把依据换成了另一份。
type MemoryRef struct {
	MemoryID string `json:"memoryId"`
	Text     string `json:"text"`
	Kind     string `json:"kind,omitempty"`
}

// SummarizeChanges 把已生效的事件折叠成玩家可读的变化列表。
//
// state 必须是**事件生效之后**的状态：角色显示名与物品名从这里取，关系数值也要用它
// 反推变化前的值。无法解释或不值得展示的事件（记忆、导演改动、被夹到 0 的变化）
// 会被跳过而不是凑成空条目。
func SummarizeChanges(events []*DomainEvent, state *WorldState) []StateChange {
	if state == nil {
		return nil
	}
	out := make([]StateChange, 0, len(events))
	for _, ev := range events {
		if change, ok := summarizeEvent(ev, state); ok {
			out = append(out, change)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func summarizeEvent(ev *DomainEvent, state *WorldState) (StateChange, bool) {
	if ev == nil {
		return StateChange{}, false
	}
	switch ev.Type {
	case EventRelationshipDelta:
		var p RelationshipDeltaPayload
		if !decodePayload(ev, &p) || p.Applied == 0 {
			return StateChange{}, false
		}
		field := RelationshipField(p.Field)
		after := relationshipValue(state.Relationships[p.CharacterID], field)
		return StateChange{
			Kind:  "relationship",
			Label: characterLabel(state, p.CharacterID) + " · " + relationshipFieldLabel(field),
			// 显示**实际生效**的量（Applied）而不是提议量（Delta）：提议可能被
			// 规则夹断，玩家看到的必须是真正发生的那部分。
			Text:  fmt.Sprintf("%+d（%d → %d）", p.Applied, after-p.Applied, after),
			Delta: p.Applied,
		}, true
	case EventMoodSet:
		var p struct {
			CharacterID string        `json:"characterId"`
			Mood        CharacterMood `json:"mood"`
		}
		if !decodePayload(ev, &p) || p.Mood.Text == "" {
			return StateChange{}, false
		}
		return StateChange{Kind: "mood", Label: characterLabel(state, p.CharacterID) + " · 心情", Text: p.Mood.Text}, true
	case EventGoalSet:
		var p struct {
			GoalID      string `json:"goalId"`
			CharacterID string `json:"characterId"`
			Text        string `json:"text"`
		}
		if !decodePayload(ev, &p) || p.Text == "" {
			return StateChange{}, false
		}
		return StateChange{Kind: "goal", Label: characterLabel(state, p.CharacterID) + " · 目标", Text: p.Text}, true
	case EventPromisePropose:
		var p PromiseProposePayload
		if !decodePayload(ev, &p) || p.Promise.Content == "" {
			return StateChange{}, false
		}
		return StateChange{Kind: "promise", Label: "约定", Text: p.Promise.Content}, true
	case EventPromiseSettle:
		var p PromiseSettlePayload
		if !decodePayload(ev, &p) {
			return StateChange{}, false
		}
		return StateChange{Kind: "promise", Label: "约定", Text: promiseStateLabel(p.State)}, true
	case EventItemGrant:
		var p ItemGrantPayload
		if !decodePayload(ev, &p) || p.Item.Name == "" {
			return StateChange{}, false
		}
		return StateChange{
			Kind: "item", Label: "物品",
			Text:  fmt.Sprintf("获得「%s」×%d", p.Item.Name, maxInt(p.Item.Quantity, 1)),
			Delta: maxInt(p.Item.Quantity, 1),
		}, true
	case EventItemTransfer:
		var p ItemTransferPayload
		if !decodePayload(ev, &p) {
			return StateChange{}, false
		}
		return StateChange{
			Kind: "item", Label: "物品",
			Text: fmt.Sprintf("「%s」转交（×%d）", itemLabel(state, p.ItemID), maxInt(p.Quantity, 1)),
		}, true
	case EventItemConsume:
		var p ItemTransferPayload
		if !decodePayload(ev, &p) {
			return StateChange{}, false
		}
		return StateChange{
			Kind: "item", Label: "物品",
			Text:  fmt.Sprintf("消耗「%s」×%d", itemLabel(state, p.ItemID), maxInt(p.Quantity, 1)),
			Delta: -maxInt(p.Quantity, 1),
		}, true
	case EventScenePropose:
		var p ScenePayload
		if !decodePayload(ev, &p) || p.Scene.Title == "" {
			return StateChange{}, false
		}
		return StateChange{Kind: "scene", Label: "场景", Text: p.Scene.Title}, true
	case EventMilestonePropose:
		var p MilestonePayload
		if !decodePayload(ev, &p) || p.Description == "" {
			return StateChange{}, false
		}
		return StateChange{Kind: "milestone", Label: "里程碑", Text: p.Description}, true
	case EventSecretUnlock:
		var p SecretUnlockPayload
		if !decodePayload(ev, &p) {
			return StateChange{}, false
		}
		title := p.Title
		if title == "" {
			title = p.SecretID
		}
		return StateChange{Kind: "secret", Label: "秘密", Text: "解锁「" + title + "」"}, true
	default:
		// 记忆与导演改动不进这层：记忆有自己的面板与证据链，导演改动属于创作侧。
		return StateChange{}, false
	}
}

func decodePayload(ev *DomainEvent, target any) bool {
	if ev.PayloadJSON == "" {
		return false
	}
	return json.Unmarshal([]byte(ev.PayloadJSON), target) == nil
}

func relationshipValue(v RelationValue, field RelationshipField) int {
	switch field {
	case FieldTrust:
		return v.Trust
	case FieldAlertness:
		return v.Alertness
	default:
		return v.Affection
	}
}

func relationshipFieldLabel(field RelationshipField) string {
	switch field {
	case FieldTrust:
		return "信任"
	case FieldAlertness:
		return "戒备"
	default:
		return "好感"
	}
}

func promiseStateLabel(state PromiseState) string {
	switch state {
	case PromiseActive:
		return "已成为有效约定"
	case PromiseFulfilled:
		return "已兑现"
	case PromiseBroken:
		return "已违背"
	case PromiseCancelled:
		return "已作废"
	default:
		return string(state)
	}
}

func characterLabel(state *WorldState, characterID string) string {
	if info, ok := state.Characters[characterID]; ok && info.Name != "" {
		return info.Name
	}
	return characterID
}

func itemLabel(state *WorldState, itemID string) string {
	if item, ok := state.Items[itemID]; ok && item.Name != "" {
		return item.Name
	}
	return itemID
}
