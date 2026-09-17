package application

import (
	"encoding/json"
	"fmt"
	"tavernagent/internal/domain"
	"tavernagent/internal/protocol"
)

func validatedRuleEvents(base *domain.WorldState, checks []domain.CheckResult, version string) ([]*domain.DomainEvent, error) {
	var events []*domain.DomainEvent
	seen := map[string]bool{}
	probe := base.Clone()
	for _, check := range checks {
		if seen[check.RollID] {
			continue
		}
		seen[check.RollID] = true
		for _, effect := range check.Effects {
			switch effect.Type {
			case domain.EventItemGrant, domain.EventItemTransfer, domain.EventItemConsume, domain.EventPromiseSettle, domain.EventScenePropose, domain.EventMilestonePropose:
			default:
				return nil, fmt.Errorf("unsupported rule effect %s", effect.Type)
			}
			event := &domain.DomainEvent{Type: effect.Type, PayloadJSON: string(effect.Payload), RulesetVersion: version}
			if err := domain.VersionItemEvent(event); err != nil {
				return nil, fmt.Errorf("invalid rule effect payload: %w", err)
			}
			if _, err := domain.ApplyEvent(probe, event); err != nil {
				return nil, fmt.Errorf("invalid rule effect: %w", err)
			}
			events = append(events, event)
		}
	}
	return events, nil
}

func authorizedItemProposal(p protocol.Proposal, checks []domain.CheckResult) bool {
	for _, check := range checks {
		if p.ReceiptRef != check.RollID || p.ActionRef != check.ActionID {
			continue
		}
		for _, e := range check.Effects {
			if p.Type == "item_grant" && e.Type == domain.EventItemGrant {
				var effect domain.ItemGrantPayload
				if json.Unmarshal(e.Payload, &effect) == nil && p.ItemID == effect.Item.InstanceID && p.To == effect.Item.OwnerID && p.Quantity == effect.Item.Quantity {
					return true
				}
			}
			if (p.Type == "item_transfer" && e.Type == domain.EventItemTransfer) || (p.Type == "item_consume" && e.Type == domain.EventItemConsume) {
				var effect domain.ItemTransferPayload
				to := p.To
				if p.Type == "item_consume" {
					to = "consumed"
				}
				if json.Unmarshal(e.Payload, &effect) == nil && p.ItemID == effect.ItemID && p.From == effect.From && to == effect.To && p.Quantity == effect.Quantity {
					return true
				}
			}
		}
	}
	return false
}
