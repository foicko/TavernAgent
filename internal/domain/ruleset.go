package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

func ValidateRuleset(rs Ruleset) error {
	if rs.Version == "" || len(rs.Actions) > 100 {
		return fmt.Errorf("规则版本必填，动作不能超过 100 条")
	}
	for key, rule := range rs.Actions {
		if key == "" || rule.ActionID != key || rule.DC < 1 || rule.DC > 100 || strings.TrimSpace(rule.Attribute) == "" {
			return fmt.Errorf("动作 %s 的 ID、属性或难度无效", key)
		}
		if rule.Requires != nil {
			if _, err := rule.Requires.Eval(RuleContext{}); err != nil {
				return err
			}
		}
		for outcome, effects := range rule.Consequences {
			if !validOutcome(outcome) || len(effects) > 8 {
				return fmt.Errorf("规则结果或效果数量无效")
			}
			for _, e := range effects {
				switch e.Type {
				case EventItemGrant, EventItemTransfer, EventItemConsume, EventPromiseSettle, EventScenePropose, EventMilestonePropose:
				default:
					return fmt.Errorf("不支持规则效果 %s", e.Type)
				}
				if !json.Valid(e.Payload) || len(e.Payload) > 8192 {
					return fmt.Errorf("规则效果 payload 无效")
				}
			}
		}
		for outcome, text := range rule.PermanentEffects {
			if !validOutcome(outcome) || len([]rune(text)) > 512 {
				return fmt.Errorf("永久效果无效")
			}
		}
	}
	return nil
}
func validOutcome(o CheckOutcome) bool {
	return o == OutcomeSuccess || o == OutcomeFailure || o == OutcomeCriticalSuccess || o == OutcomeCriticalFailure
}
