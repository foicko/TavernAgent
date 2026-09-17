package context

import (
	"math"
	"strings"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/search"
)

// currentMemoryRelevance measures distinctive content in the current request.
// RRF is useful for merging candidate channels, but loses the distance between
// a precise topic match and a common history word. Query-local IDF restores
// that distinction before importance and recency are applied. Names are scored
// separately through the entity channel; they cannot stand in for a topic.
func currentMemoryRelevance(candidates []*ports.MemoryCandidate, input string, state *domain.WorldState) map[string]float64 {
	query := strings.ToLower(input)
	if state != nil {
		for id, ch := range state.Characters {
			query = strings.ReplaceAll(query, strings.ToLower(id), " ")
			for _, name := range ch.SearchTerms() {
				if name != "" {
					query = strings.ReplaceAll(query, strings.ToLower(name), " ")
				}
			}
		}
	}
	query += " " + strings.Join(search.ConceptTerms(input), " ")
	terms := search.Tokens(query)
	if len(terms) == 0 || len(candidates) == 0 {
		return nil
	}
	documents := make(map[string]map[string]bool, len(candidates))
	frequency := map[string]int{}
	for _, candidate := range candidates {
		words := map[string]bool{}
		for _, word := range search.Tokenize(candidate.Memory.Content) {
			if !words[word] {
				frequency[word]++
				words[word] = true
			}
		}
		documents[candidate.Memory.MemoryID] = words
	}
	weights := map[string]float64{}
	for _, word := range terms {
		// Reporting verbs, temporal operators and question grammar express
		// how to retrieve a fact. They are not the fact's subject.
		switch word {
		case "最近", "最新", "一次", "上次", "之前", "刚才", "提到", "说起", "说过", "放在", "在哪", "哪里", "何处", "关于", "什么", "这个", "那个", "是放":
			continue
		}
		count := frequency[word]
		if count > 0 && count < len(candidates) {
			weights[word] = math.Log(1 + (float64(len(candidates)-count)+0.5)/(float64(count)+0.5))
		}
	}
	scores := map[string]float64{}
	maximum := 0.0
	for id, words := range documents {
		for word, weight := range weights {
			if words[word] {
				scores[id] += weight
			}
		}
		maximum = math.Max(maximum, scores[id])
	}
	if maximum == 0 {
		return nil
	}
	for id := range scores {
		scores[id] /= maximum
	}
	return scores
}
