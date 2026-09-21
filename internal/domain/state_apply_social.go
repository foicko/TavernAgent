// 社会关系类事件的应用：关系增量、情绪、承诺的提出与结算。
//
// 从 state.go 的 ApplyEvent 拆出。这三类共享同一个不变量——只改角色之间的
// 关系面，不触碰物品与场景，因此放在一起便于审阅「关系变化只能来自事件」。
package domain

import (
	"encoding/json"
	"fmt"
)

// isSocialEvent 判定事件是否属于本域（与下面 switch 的 case 集合必须一致）。
func isSocialEvent(t EventType) bool {
	switch t {
	case EventRelationshipDelta, EventPromisePropose, EventPromiseSettle, EventMoodSet:
		return true
	}
	return false
}

// applySocialEvent 应用本域事件，返回值契约与 ApplyEvent 相同。
func applySocialEvent(s *WorldState, ev *DomainEvent) (string, error) {
	switch ev.Type {
	case EventRelationshipDelta:
		var p RelationshipDeltaPayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		_, err := s.ApplyRelationshipDelta(p.CharacterID, RelationshipField(p.Field), p.Applied, AffectionMax, AffectionMax)
		return fmt.Sprintf("rel:%s/%s/%d", p.CharacterID, p.Field, p.Applied), err
	case EventPromisePropose:
		var p PromiseProposePayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		if _, exists := s.Promises[p.Promise.PromiseID]; exists {
			return "", fmt.Errorf("promise %q already exists", p.Promise.PromiseID)
		}
		s.Promises[p.Promise.PromiseID] = p.Promise
		return fmt.Sprintf("promise:%s", p.Promise.PromiseID), nil
	case EventPromiseSettle:
		var p PromiseSettlePayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		promise, ok := s.Promises[p.PromiseID]
		if !ok {
			return "", fmt.Errorf("unknown promise %s", p.PromiseID)
		}
		if promise.State != PromiseProposed && promise.State != PromiseActive {
			return "", fmt.Errorf("promise already settled")
		}
		if p.State != PromiseActive && p.State != PromiseFulfilled && p.State != PromiseBroken && p.State != PromiseCancelled {
			return "", fmt.Errorf("invalid promise transition")
		}
		promise.State, promise.SettledByRec = p.State, p.ReceiptID
		s.Promises[p.PromiseID] = promise
		return "promise-settle", nil
	case EventMoodSet:
		var p struct {
			CharacterID string        `json:"characterId"`
			Mood        CharacterMood `json:"mood"`
		}
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		s.Moods[p.CharacterID] = p.Mood
		return "mood", nil
	}
	return "", fmt.Errorf("event %q has no social applier", ev.Type)
}
