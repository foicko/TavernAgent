package domain

import (
	"fmt"
	"sort"
	"strings"
	"tavernagent/internal/search"
	"unicode/utf8"
)

const MemoryQuota = 80

type MemoryUsage struct {
	Limit       int    `json:"limit"`
	Used        int    `json:"used"`
	Protected   int    `json:"protected"`
	Headroom    int    `json:"headroom"`
	Tier        string `json:"tier"`
	Reclaimable int    `json:"reclaimable"`
}

func MemoryProtected(m *MemoryRecord, state *WorldState) bool {
	if m.Pinned || m.Kind == MemorySecret || m.SecretID != "" {
		return true
	}
	if state == nil {
		return false
	}
	refs := map[string]bool{}
	for _, eid := range m.EntityIDs {
		refs[eid] = true
	}
	text := strings.ToLower(m.Content)
	for iid, it := range state.Items {
		if it.Quantity <= 0 || (!it.Keepsake && it.Protection <= 0) {
			continue
		}
		if refs[iid] || (it.Name != "" && strings.Contains(text, strings.ToLower(it.Name))) {
			return true
		}
		for _, a := range it.Aliases {
			if a != "" && strings.Contains(text, strings.ToLower(a)) {
				return true
			}
		}
	}
	for pid, p := range state.Promises {
		if p.State != PromiseActive {
			continue
		}
		if refs[pid] || strings.Contains(text, "誓言") || strings.Contains(text, "承诺") || strings.Contains(text, "约定") || lexicalSimilarity(text, p.Content) >= 0.2 {
			return true
		}
	}
	return false
}

func ScanMemoryUsage(records []*MemoryRecord, state *WorldState) MemoryUsage {
	u := MemoryUsage{Limit: MemoryQuota, Tier: "Normal"}
	for _, m := range ApplyMemoryOverlays(records) {
		if m.Hidden {
			continue
		}
		if MemoryProtected(m, state) {
			u.Protected++
			continue
		}
		u.Used++
		if ClampMemoryImportance(m.Importance) <= 5 && utf8.RuneCountInString(m.Content) <= 220 {
			u.Reclaimable++
		}
	}
	u.Headroom = MemoryQuota - u.Used
	switch {
	case u.Headroom <= 5:
		u.Tier = "Critical"
	case u.Headroom <= 15:
		u.Tier = "Degraded"
	case u.Headroom <= 30:
		u.Tier = "Notice"
	}
	return u
}

type MemoryMerge struct {
	Sources []string `json:"sources"`
	Content string   `json:"content"`
}
type MemoryOrganization struct {
	Usage  MemoryUsage   `json:"usage"`
	Merges []MemoryMerge `json:"merges"`
}

// The initial organizer is deliberately extractive: combine five small,
// related observations using their original wording. No model can silently
// invent facts or promote an inferred belief into an observed fact.
func PlanMemoryOrganization(records []*MemoryRecord, state *WorldState) MemoryOrganization {
	out := MemoryOrganization{Usage: ScanMemoryUsage(records, state), Merges: []MemoryMerge{}}
	if out.Usage.Tier == "Normal" || out.Usage.Tier == "Notice" {
		return out
	}
	var candidates []*MemoryRecord
	for _, m := range ApplyMemoryOverlays(records) {
		if m.Hidden || MemoryProtected(m, state) || ClampMemoryImportance(m.Importance) > 5 || utf8.RuneCountInString(m.Content) > 220 {
			continue
		}
		candidates = append(candidates, m)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].MemoryID < candidates[j].MemoryID })
	used := map[string]bool{}
	remaining := out.Usage.Used
	for _, seed := range candidates {
		if used[seed.MemoryID] || remaining < 50 {
			continue
		}
		group := []*MemoryRecord{seed}
		for _, m := range candidates {
			if m.MemoryID == seed.MemoryID || used[m.MemoryID] || m.Kind != seed.Kind || ownerKey(m.OwnerIDs) != ownerKey(seed.OwnerIDs) {
				continue
			}
			if lexicalSimilarity(seed.Content, m.Content) < 0.2 {
				continue
			}
			if len(seed.EntityIDs) > 0 && !overlapIDs(seed.EntityIDs, m.EntityIDs) {
				continue
			}
			group = append(group, m)
			if len(group) == 5 {
				break
			}
		}
		if len(group) < 5 {
			continue
		}
		merge := MemoryMerge{}
		var parts []string
		for _, m := range group {
			used[m.MemoryID] = true
			merge.Sources = append(merge.Sources, m.MemoryID)
			parts = append(parts, m.Content)
		}
		merge.Content = "相关见闻整理：" + strings.Join(parts, "；")
		out.Merges = append(out.Merges, merge)
		remaining -= 4
	}
	return out
}

// Gate is independent of planning and re-checks every referenced source.
func GateMemoryOrganization(plan MemoryOrganization, records []*MemoryRecord, state *WorldState) error {
	byID := map[string]*MemoryRecord{}
	for _, m := range ApplyMemoryOverlays(records) {
		if !m.Hidden {
			byID[m.MemoryID] = m
		}
	}
	seen := map[string]bool{}
	for _, merge := range plan.Merges {
		if len(merge.Sources) != 5 || utf8.RuneCountInString(merge.Content) > 1400 {
			return fmt.Errorf("invalid merge size")
		}
		var first *MemoryRecord
		var parts []string
		for _, sid := range merge.Sources {
			m := byID[sid]
			if m == nil || seen[sid] || MemoryProtected(m, state) {
				return fmt.Errorf("protected, missing or duplicate merge source %q", sid)
			}
			if ClampMemoryImportance(m.Importance) > 5 {
				return fmt.Errorf("merge source is not a trivial observation")
			}
			if first == nil {
				first = m
			} else if m.Kind != first.Kind || ownerKey(m.OwnerIDs) != ownerKey(first.OwnerIDs) {
				return fmt.Errorf("merge changes memory perspective")
			}
			seen[sid] = true
			parts = append(parts, m.Content)
		}
		if merge.Content != "相关见闻整理："+strings.Join(parts, "；") {
			return fmt.Errorf("merge content must preserve source wording")
		}
	}
	return nil
}

func BuildOrganizedMemories(plan MemoryOrganization, records []*MemoryRecord, state *WorldState, nodeID string) ([]*MemoryRecord, error) {
	if err := GateMemoryOrganization(plan, records, state); err != nil {
		return nil, err
	}
	byID := map[string]*MemoryRecord{}
	for _, m := range records {
		byID[m.MemoryID] = m
	}
	var additions, overlays []*MemoryRecord
	for i, merge := range plan.Merges {
		first := byID[merge.Sources[0]]
		m := &MemoryRecord{MemoryID: fmt.Sprintf("mem_%s_merge_%d", nodeID, i), SourceNodeID: nodeID, Kind: first.Kind,
			Content: merge.Content, OwnerIDs: first.OwnerIDs, Importance: first.Importance, Confidence: first.Confidence, MergedFrom: merge.Sources}
		entitySet := map[string]bool{}
		for j, sid := range merge.Sources {
			old := byID[sid]
			if old.Confidence < m.Confidence {
				m.Confidence = old.Confidence
			}
			for _, eid := range old.EntityIDs {
				entitySet[eid] = true
			}
			copy := *old
			copy.MemoryID = fmt.Sprintf("mem_%s_merge_%d_hide_%d", nodeID, i, j)
			copy.SourceNodeID = nodeID
			copy.Supersedes = sid
			copy.Hidden = true
			overlays = append(overlays, &copy)
		}
		for eid := range entitySet {
			m.EntityIDs = append(m.EntityIDs, eid)
		}
		sort.Strings(m.EntityIDs)
		additions = append(additions, m)
	}
	return append(additions, overlays...), nil
}

func ownerKey(ids []string) string {
	v := append([]string{}, ids...)
	sort.Strings(v)
	return strings.Join(v, "\x00")
}
func overlapIDs(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}
func lexicalSimilarity(a, b string) float64 {
	x, y := search.Tokens(a), search.Tokens(b)
	set := map[string]bool{}
	for _, t := range x {
		set[t] = true
	}
	hits := 0
	for _, t := range y {
		if set[t] {
			hits++
		}
	}
	denom := len(x) + len(y) - hits
	if denom == 0 {
		return 0
	}
	return float64(hits) / float64(denom)
}
