package sqlite

import (
	"encoding/json"
	"fmt"
	"tavernagent/internal/domain"
)

func remapMemory(m *domain.MemoryRecord, nodes, memories map[string]string) (*domain.MemoryRecord, error) {
	n := *m
	n.MemoryID, n.SourceNodeID = memories[m.MemoryID], nodes[m.SourceNodeID]
	if n.MemoryID == "" || n.SourceNodeID == "" {
		return nil, fmt.Errorf("unmapped memory source")
	}
	if m.Supersedes != "" {
		n.Supersedes = memories[m.Supersedes]
		if n.Supersedes == "" {
			return nil, fmt.Errorf("unmapped memory revision")
		}
	}
	if m.Evidence != nil {
		e := *m.Evidence
		n.Evidence = &e
		if e.SourceNodeID != "" {
			n.Evidence.SourceNodeID = nodes[e.SourceNodeID]
			if n.Evidence.SourceNodeID == "" {
				return nil, fmt.Errorf("unmapped evidence source")
			}
		}
	}
	n.MergedFrom = make([]string, 0, len(m.MergedFrom))
	for _, old := range m.MergedFrom {
		mapped := memories[old]
		if mapped == "" {
			return nil, fmt.Errorf("unmapped merged memory")
		}
		n.MergedFrom = append(n.MergedFrom, mapped)
	}
	return &n, nil
}

func remapEventPayload(kind domain.EventType, raw string, nodes, memories, receipts map[string]string) (string, error) {
	var payload any
	switch kind {
	case domain.EventDirectorChange:
		var change domain.DirectorChange
		if err := json.Unmarshal([]byte(raw), &change); err != nil {
			return "", err
		}
		remap := func(ref *string) error {
			if *ref == "" {
				return nil
			}
			mapped := nodes[*ref]
			if mapped == "" {
				return fmt.Errorf("unmapped director revision %s", *ref)
			}
			*ref = mapped
			return nil
		}
		if err := remap(&change.BaseRevisionID); err != nil {
			return "", err
		}
		if change.Plan != nil {
			if err := remap(&change.Plan.PlanID); err != nil {
				return "", err
			}
			if err := remap(&change.Plan.RevisionID); err != nil {
				return "", err
			}
		}
		if change.Report != nil {
			if err := remap(&change.Report.RevisionID); err != nil {
				return "", err
			}
		}
		payload = change
	case domain.EventMemoryAdd, domain.EventMemoryRevised, domain.EventMemoryPinned, domain.EventMemoryHidden:
		var p domain.MemoryAddPayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return "", err
		}
		m, err := remapMemory(&p.Memory, nodes, memories)
		if err != nil {
			return "", err
		}
		p.Memory = *m
		payload = p
	case domain.EventPromisePropose:
		var p domain.PromiseProposePayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return "", err
		}
		if mapped := nodes[p.Promise.SourceNodeID]; mapped != "" {
			p.Promise.SourceNodeID = mapped
		}
		if mapped := receipts[p.Promise.SettledByRec]; mapped != "" {
			p.Promise.SettledByRec = mapped
		}
		payload = p
	case domain.EventPromiseSettle:
		var p domain.PromiseSettlePayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return "", err
		}
		if mapped := receipts[p.ReceiptID]; mapped != "" {
			p.ReceiptID = mapped
		}
		payload = p
	default:
		return raw, nil
	}
	b, err := json.Marshal(payload)
	return string(b), err
}

func remapSnapshot(raw string, nodes, receipts map[string]string) (string, string, error) {
	state, err := domain.UnmarshalWorld(raw)
	if err != nil {
		return "", "", err
	}
	for id, p := range state.Promises {
		if mapped := nodes[p.SourceNodeID]; mapped != "" {
			p.SourceNodeID = mapped
		}
		if mapped := receipts[p.SettledByRec]; mapped != "" {
			p.SettledByRec = mapped
		}
		state.Promises[id] = p
	}
	blob, err := state.Marshal()
	return blob, state.HashID(), err
}

func remapExtraContent(raw string, nodes, memories, turns, receipts, rolls map[string]string) (string, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", err
	}
	for field, mapping := range map[string]map[string]string{"sourceTurnId": turns, "sourceNodeId": nodes} {
		if value, ok := doc[field]; ok {
			var old string
			if json.Unmarshal(value, &old) != nil {
				return "", fmt.Errorf("invalid source reference")
			}
			if mapping[old] != "" {
				doc[field], _ = json.Marshal(mapping[old])
			} else {
				delete(doc, field)
			}
		}
	}
	if data, ok := doc["memoryChanges"]; ok {
		var changes []*domain.MemoryRecord
		if err := json.Unmarshal(data, &changes); err != nil {
			return "", err
		}
		for i, m := range changes {
			mapped, err := remapMemory(m, nodes, memories)
			if err != nil {
				return "", err
			}
			changes[i] = mapped
		}
		doc["memoryChanges"], _ = json.Marshal(changes)
	}
	if data, ok := doc["checks"]; ok {
		var checks []domain.CheckResult
		if err := json.Unmarshal(data, &checks); err != nil {
			return "", err
		}
		for i := range checks {
			if mapped := rolls[checks[i].RollID]; mapped != "" {
				checks[i].RollID = mapped
			}
			for j, e := range checks[i].Effects {
				mapped, err := remapEventPayload(e.Type, string(e.Payload), nodes, memories, receipts)
				if err != nil {
					return "", err
				}
				checks[i].Effects[j].Payload = json.RawMessage(mapped)
			}
		}
		doc["checks"], _ = json.Marshal(checks)
	}
	blob, err := json.Marshal(doc)
	return string(blob), err
}
