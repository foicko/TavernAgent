package context

import (
	"strings"

	"tavernagent/internal/domain"
)

// ---- 世界书（关键词命中注入，技术契约 §4「世界书」与路线图 M3）----

// lorebookHit 是一条命中的世界书条目及其所属世界书。
type lorebookHit struct {
	Book  string
	Entry domain.LorebookEntry
}

// sessionContext 是编译所需、来自会话根节点的稳定信息（T20：会话内版本固定）。
type sessionContext struct {
	Director    *domain.DirectorState
	OpeningText string
	PlayerName  string
	PlayerRole  string
	Lorebooks   []domain.Lorebook
	Rules       domain.Ruleset
	// Secrets 是会话的**全部**秘密定义（来自卡片模板）。
	// 是否注入由 WorldState.UnlockedSecrets 决定——未揭示的内容绝不进入上下文（T25）。
	Secrets []secretEntry
}

// secretEntry 是一条秘密定义（编译层只需要这三项；揭示条件不在编译期求值）。
type secretEntry struct {
	SecretID   string
	Title      string
	Content    string
	UnlockTurn int
}

// matchLorebook 按关键词在检索文本中命中世界书条目。
// 命中顺序稳定（世界书声明顺序 → 条目声明顺序）；受条数与字符预算约束。
// 只读：不写库、不改状态、不因命中次数提升任何权重（T18）。
func (c *Compiler) matchLorebook(sc sessionContext, haystack string) []lorebookHit {
	if len(sc.Lorebooks) == 0 {
		return nil
	}
	hay := strings.ToLower(haystack)

	var hits []lorebookHit
	used, usedRunes := 0, 0
	for _, book := range sc.Lorebooks {
		seen := map[string]bool{}
		for _, e := range book.EnabledEntries() {
			if e.EntryID != "" && seen[e.EntryID] {
				continue
			}
			if !entryMatches(e, hay, c.options.MinLorebookKeyRunes) {
				continue
			}
			n := len([]rune(e.Content))
			if used >= c.options.MaxLorebookEntries || usedRunes+n > c.options.MaxLorebookRunes {
				continue // 超预算：跳过该条，后续更短的条目仍有机会命中
			}
			seen[e.EntryID] = true
			hits = append(hits, lorebookHit{Book: book.Name, Entry: e})
			used++
			usedRunes += n
		}
	}
	return hits
}

// buildHaystack 拼出检索文本：开场白 + 最近正文历史 + 当前输入。
// 世界书命中与记忆召回共用这一段，避免两处各拼一遍导致行为漂移。
func buildHaystack(opening string, recent []*domain.PlotNode, inputText string) string {
	var sb strings.Builder
	sb.WriteString(opening)
	sb.WriteString("\n")
	sb.WriteString(inputText)
	for _, n := range recent {
		if n.Kind != domain.NodeKindTurn {
			continue
		}
		tc, err := parseTurnContent(n.ContentJSON)
		if err != nil {
			continue
		}
		sb.WriteString("\n")
		sb.WriteString(tc.InputText)
		sb.WriteString("\n")
		sb.WriteString(renderBlocks(tc.Blocks))
	}
	return sb.String()
}

// entryMatches 判断条目是否命中：任一 key 作为子串出现在给文本中。
// 若配置了 SecondaryKeys，则要求主关键词命中的同时，至少一个次要关键词也必须命中。
// 短于 minRunes 的 key 被忽略——单字 key（如「门」）会命中几乎所有文本，噪声大于收益。
func entryMatches(e domain.LorebookEntry, haystack string, minRunes int) bool {
	matchedPrimary := false
	for _, k := range e.Keys {
		k = strings.TrimSpace(k)
		if len([]rune(k)) < minRunes {
			continue
		}
		if strings.Contains(haystack, strings.ToLower(k)) {
			matchedPrimary = true
			break
		}
	}
	if !matchedPrimary {
		return false
	}
	if len(e.SecondaryKeys) == 0 {
		return true
	}
	for _, sk := range e.SecondaryKeys {
		sk = strings.TrimSpace(sk)
		if len([]rune(sk)) < minRunes {
			continue
		}
		if strings.Contains(haystack, strings.ToLower(sk)) {
			return true
		}
	}
	return false
}
