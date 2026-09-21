// 物品类事件的应用：整实例转移、数量消耗、新物品授予。
//
// 从 state.go 的 ApplyEvent 拆出（原 14 个 case 的单体 switch）。case 分支逐字未改，
// 只是换了一个文件；事件语义、错误文案与返回标签都与拆分前完全一致。
package domain

import (
	"encoding/json"
	"fmt"
)

// 本域的判定直接复用 state_replay.go 里的 isItemEvent：物品事件集合在
// "重放投影"与"事件应用"两处必须一致，各写一份迟早会漂移。

// applyItemEvent 应用本域事件，返回值契约与 ApplyEvent 相同。
func applyItemEvent(s *WorldState, ev *DomainEvent) (string, error) {
	switch ev.Type {
	case EventItemTransfer:
		var p ItemTransferPayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		if err := s.ValidateItemTransfer(p.ItemID, p.From, p.To, p.Quantity); err != nil {
			return "", err
		}
		s.ApplyItemTransfer(p.ItemID, p.To, p.Quantity)
		return fmt.Sprintf("transfer:%s->%s,%d", p.From, p.To, p.Quantity), nil
	case EventItemConsume:
		var p ItemTransferPayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		if p.To != "" && p.To != "consumed" {
			return "", fmt.Errorf("consumption cannot transfer ownership")
		}
		if err := s.validateItemQuantity(p.ItemID, p.From, p.Quantity); err != nil {
			return "", err
		}
		it := s.Items[p.ItemID]
		it.Quantity -= p.Quantity
		if it.Quantity == 0 {
			it.OwnerID, it.LocationID = "consumed", ""
		}
		s.Items[p.ItemID] = it
		return fmt.Sprintf("consume:%s,%d", p.ItemID, p.Quantity), nil
	case EventItemGrant:
		var p ItemGrantPayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		if err := s.ApplyItemGrant(p.Item); err != nil {
			return "", err
		}
		return fmt.Sprintf("grant:%s", p.Item.InstanceID), nil
	}
	return "", fmt.Errorf("event %q has no item applier", ev.Type)
}
