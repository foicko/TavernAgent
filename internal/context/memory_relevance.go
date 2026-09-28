package context

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/textutil"
)

// currentMemoryRelevance measures distinctive content in the current request.
// RRF is useful for merging candidate channels, but loses the distance between
// a precise topic match and a common history word. Query-local IDF restores
// that distinction before importance and recency are applied. Names are scored
// separately through the entity channel; they cannot stand in for a topic.
func currentMemoryRelevance(candidates []*ports.MemoryCandidate, input string, state *domain.WorldState) map[string]float64 {
	query := strings.ToLower(input)
	if state != nil {
		// 名字按"长的先剥、同长按字典序"固定顺序剔除：map 遍历顺序随机，
		// 重叠的名字（"莉尔"/"莉尔娜"）先剥短的会留下"娜"之类残片，排名随之漂移。
		var names []string
		seen := map[string]bool{}
		addName := func(v string) {
			v = strings.ToLower(v)
			if v != "" && !seen[v] {
				seen[v] = true
				names = append(names, v)
			}
		}
		for id, ch := range state.Characters {
			addName(id)
			for _, name := range ch.SearchTerms() {
				addName(name)
			}
		}
		sort.Slice(names, func(i, j int) bool {
			li, lj := utf8.RuneCountInString(names[i]), utf8.RuneCountInString(names[j])
			if li != lj {
				return li > lj
			}
			return names[i] < names[j]
		})
		for _, name := range names {
			query = strings.ReplaceAll(query, name, " ")
		}
	}
	query += " " + strings.Join(textutil.ConceptTerms(input), " ")
	terms := textutil.Tokens(query)
	if len(terms) == 0 || len(candidates) == 0 {
		return nil
	}
	documents := make(map[string]map[string]bool, len(candidates))
	frequency := map[string]int{}
	for _, candidate := range candidates {
		words := map[string]bool{}
		for _, word := range textutil.Tokenize(candidate.Memory.Content) {
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
	// 浮点加法不满足结合律：按固定的词序累加，避免 map 顺序让同分候选的排名抖动。
	weightTerms := make([]string, 0, len(weights))
	for word := range weights {
		weightTerms = append(weightTerms, word)
	}
	sort.Strings(weightTerms)
	scores := map[string]float64{}
	maximum := 0.0
	for id, words := range documents {
		for _, word := range weightTerms {
			if words[word] {
				scores[id] += weights[word]
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
