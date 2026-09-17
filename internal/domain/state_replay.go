package domain

import (
	"encoding/json"
	"fmt"
)

// VersionItemEvent fixes the projection semantics of a new authoritative item
// event. Legacy persisted payloads intentionally retain their historical result.
func VersionItemEvent(event *DomainEvent) error {
	if !isItemEvent(event.Type) {
		return nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
		return err
	}
	if payload == nil {
		return fmt.Errorf("item event payload required")
	}
	payload["stateProjectionVersion"] = json.RawMessage("2")
	raw, err := json.Marshal(payload)
	event.PayloadJSON = string(raw)
	return err
}

// ReplayEvents is the read path for persisted events. ApplyEvents validates and
// applies new proposals; replay also understands old, unversioned item payloads.
// In v1 a consume moved the entire instance to its destination without reducing
// quantity, and transfers retained location metadata. Reinterpreting these
// events as v2 would make state depend on which checkpoint happened to be read.
func ReplayEvents(state *WorldState, events []*DomainEvent) error {
	for _, event := range events {
		if event == nil {
			return fmt.Errorf("nil state event")
		}
		if isItemEvent(event.Type) {
			var header struct {
				Version int `json:"stateProjectionVersion"`
			}
			if err := json.Unmarshal([]byte(event.PayloadJSON), &header); err != nil {
				return err
			}
			switch header.Version {
			case 0, 1:
				if err := replayLegacyItem(state, event); err != nil {
					return err
				}
				continue
			case 2:
			default:
				return fmt.Errorf("unsupported item projection version %d", header.Version)
			}
		}
		if _, err := ApplyEvent(state, event); err != nil {
			return err
		}
	}
	return nil
}

func isItemEvent(kind EventType) bool {
	return kind == EventItemGrant || kind == EventItemTransfer || kind == EventItemConsume
}

func replayLegacyItem(state *WorldState, event *DomainEvent) error {
	if event.Type == EventItemGrant {
		var p ItemGrantPayload
		if err := json.Unmarshal([]byte(event.PayloadJSON), &p); err != nil {
			return err
		}
		if _, exists := state.Items[p.Item.InstanceID]; exists || p.Item.Quantity < 0 {
			return fmt.Errorf("invalid legacy item grant")
		}
		state.Items[p.Item.InstanceID] = p.Item
		return nil
	}
	var p ItemTransferPayload
	if err := json.Unmarshal([]byte(event.PayloadJSON), &p); err != nil {
		return err
	}
	if err := state.validateItemQuantity(p.ItemID, p.From, p.Quantity); err != nil {
		return err
	}
	item := state.Items[p.ItemID]
	item.OwnerID = p.To
	state.Items[p.ItemID] = item
	return nil
}
