package application

import (
	"encoding/json"
	"tavernagent/internal/domain"
)

// Enforce quotas in the synchronous commit too: disabling the evaluator must
// not allow a model to grow the active branch's memory without bounds.
func (s *TurnService) applyMemoryQuota(pd *planData, headID, nodeID string) error {
	if len(pd.Memories) == 0 {
		return nil
	}
	count, err := s.store.ActiveMemoryCount(headID)
	if err != nil {
		return err
	}
	if count+len(pd.Memories) < 65 {
		return nil
	}
	records, err := s.store.ProjectedMemories(headID)
	if err != nil {
		return err
	}
	plan := domain.PlanMemoryOrganization(records, pd.NewState)
	organized, err := domain.BuildOrganizedMemories(plan, records, pd.NewState, nodeID)
	if err != nil {
		return err
	}
	u := domain.ScanMemoryUsage(append(append([]*domain.MemoryRecord{}, records...), organized...), pd.NewState)
	kept := []*domain.MemoryRecord{}
	dropped := map[string]bool{}
	for _, m := range pd.Memories {
		if !domain.MemoryProtected(m, pd.NewState) {
			if u.Used >= domain.MemoryQuota || (u.Used >= 75 && m.Kind == domain.MemoryObserved && domain.ClampMemoryImportance(m.Importance) <= 5) {
				dropped[m.MemoryID] = true
				continue
			}
			u.Used++
		}
		kept = append(kept, m)
	}
	events := pd.Events[:0]
	for _, ev := range pd.Events {
		if ev.Type == domain.EventMemoryAdd {
			var p domain.MemoryAddPayload
			if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
				return err
			}
			if dropped[p.Memory.MemoryID] {
				continue
			}
		}
		events = append(events, ev)
	}
	pd.Memories = append(kept, organized...)
	pd.Events = append(events, memoryEvents(organized)...)
	return nil
}
