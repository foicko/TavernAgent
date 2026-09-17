package domain

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxMemoryContentRunes = 1200

type MemoryEvidence struct {
	SourceNodeID   string `json:"sourceNodeId,omitempty"`
	SourceQuote    string `json:"sourceQuote,omitempty"`
	Reasoning      string `json:"reasoning,omitempty"`
	Confidence     string `json:"confidence"`
	AutoDowngraded bool   `json:"autoDowngraded,omitempty"`
}

// EvidenceExcerpt is one original input or block, never a concatenation that
// could manufacture a quote across unrelated speakers or turns.
type EvidenceExcerpt struct {
	NodeID string `json:"nodeId"`
	Text   string `json:"text"`
}

func MemoryVisibleTo(m *MemoryRecord, state *WorldState) bool {
	if m.Hidden {
		return false
	}
	if m.SecretID != "" || m.Kind == MemorySecret {
		if state == nil || m.SecretID == "" || !state.IsSecretUnlocked(m.SecretID) {
			return false
		}
	}
	if len(m.OwnerIDs) == 0 {
		return true
	}
	if state == nil {
		return false
	}
	for _, oid := range m.OwnerIDs {
		if c, ok := state.Characters[oid]; ok && (c.Participant || oid == "player") {
			return true
		}
	}
	return false
}

type CognitiveMemory struct {
	Content     string   `json:"content"`
	EntityIDs   []string `json:"entityIds,omitempty"`
	OwnerIDs    []string `json:"ownerIds,omitempty"`
	SourceQuote string   `json:"sourceQuote,omitempty"`
	Reasoning   string   `json:"reasoning,omitempty"`
	Confidence  string   `json:"confidence"`
	Importance  int      `json:"importance,omitempty"`
	SubjectKey  string   `json:"subjectKey,omitempty"`
}

type CognitiveRevision struct {
	OldMemoryID string `json:"oldMemoryId"`
	NewContent  string `json:"newContent"`
	Reason      string `json:"reason"`
	SourceQuote string `json:"sourceQuote"`
}

type CognitiveRelationship struct {
	CharacterID string            `json:"characterId"`
	Dimension   RelationshipField `json:"dimension"`
	Delta       int               `json:"delta"`
	Reason      string            `json:"reason"`
	SourceQuote string            `json:"sourceQuote"`
}

type CognitivePlan struct {
	PlanID             string                  `json:"planId"`
	TurnID             string                  `json:"turnId"`
	WriteObserved      []CognitiveMemory       `json:"writeObserved"`
	InferBelief        []CognitiveMemory       `json:"inferBelief"`
	SupersedeMemory    []CognitiveRevision     `json:"supersedeMemory"`
	AdjustRelationship []CognitiveRelationship `json:"adjustRelationship"`
}

// ResolveEntityID maps aliases, character names, player keywords, and prefixes (e.g. "liel" -> "npc_liel")
// to canonical entity IDs recognized by the WorldState.
func ResolveEntityID(raw string, state *WorldState) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || state == nil {
		return raw
	}
	// 1. Direct match in state
	if _, ok := state.Characters[raw]; ok {
		return raw
	}
	if _, ok := state.Items[raw]; ok {
		return raw
	}
	if _, ok := state.Promises[raw]; ok {
		return raw
	}

	lower := strings.ToLower(raw)
	// 2. Player aliases
	if lower == "player" || lower == "user" || lower == "self" || raw == "玩家" || raw == "旅人" || raw == "主角" || raw == "我" {
		if _, ok := state.Characters["player"]; ok {
			return "player"
		}
	}
	if p, ok := state.Characters["player"]; ok {
		for _, term := range p.SearchTerms() {
			if strings.EqualFold(raw, term) {
				return "player"
			}
		}
	}

	// 3. Prefix "npc_" (e.g. "liel" -> "npc_liel")
	npcPrefixed := "npc_" + strings.TrimPrefix(lower, "npc_")
	if _, ok := state.Characters[npcPrefixed]; ok {
		return npcPrefixed
	}

	// 4. Match character Name, Aliases, or stripped ID
	for cid, c := range state.Characters {
		for _, term := range c.SearchTerms() {
			if strings.EqualFold(term, raw) {
				return cid
			}
		}
		strippedCID := strings.TrimPrefix(cid, "npc_")
		if strings.EqualFold(strippedCID, raw) || strings.EqualFold(strippedCID, lower) {
			return cid
		}
	}

	// 5. Match items by Name and Aliases
	for iid, it := range state.Items {
		if strings.EqualFold(it.Name, raw) {
			return iid
		}
		for _, alias := range it.Aliases {
			if strings.EqualFold(alias, raw) {
				return iid
			}
		}
	}

	return raw
}

// trimQuoteBoundaries strips wrapping quotation marks, markdown formatting, brackets, and whitespace.
func trimQuoteBoundaries(s string) string {
	return strings.Trim(s, "\"'“”‘「」『』《》【】()（）[]*`~。，,!?！？…—– \t\r\n")
}

// stripSpeakerPrefix removes dialogue attribution like "莉尔：「" or "莉尔说：" from the start of a quote.
func stripSpeakerPrefix(s string) string {
	s = strings.TrimSpace(s)
	for _, sep := range []string{"：", ":"} {
		if idx := strings.Index(s, sep); idx > 0 && idx <= 24 {
			prefix := s[:idx]
			if utf8.RuneCountInString(prefix) <= 8 {
				rest := strings.TrimSpace(s[idx+len(sep):])
				rest = trimQuoteBoundaries(rest)
				if rest != "" {
					return rest
				}
			}
		}
	}
	return s
}

// normalizePunctuation replaces various punctuation forms with standard equivalents.
func normalizePunctuation(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '…':
			b.WriteRune('…')
		case '“', '”', '‘', '’', '「', '」', '『', '』', '"', '\'':
			b.WriteRune('"')
		case '，', '、':
			b.WriteRune(',')
		case '。':
			b.WriteRune('.')
		case '！':
			b.WriteRune('!')
		case '？':
			b.WriteRune('?')
		case '：':
			b.WriteRune(':')
		case '；':
			b.WriteRune(';')
		case '—', '–':
			b.WriteRune('-')
		default:
			if unicode.IsSpace(r) {
				b.WriteRune(' ')
			} else {
				b.WriteRune(unicode.ToLower(r))
			}
		}
	}
	res := b.String()
	res = strings.ReplaceAll(res, "...", "…")
	res = strings.ReplaceAll(res, "……", "…")
	return res
}

// cleanRunesOnly returns only letters, digits, and CJK characters in lowercase.
func cleanRunesOnly(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Han, r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func ValidateMemoryEvidence(kind MemoryKind, evidence MemoryEvidence, excerpts []EvidenceExcerpt) (MemoryEvidence, float64, error) {
	evidence.SourceQuote = strings.TrimSpace(evidence.SourceQuote)
	evidence.Reasoning = strings.TrimSpace(evidence.Reasoning)
	evidence.SourceNodeID = "" // only the verifier chooses the source
	evidence.AutoDowngraded = false
	if kind == MemoryInferred && evidence.Reasoning == "" {
		return evidence, 0, fmt.Errorf("inferred memory requires reasoning")
	}
	if utf8.RuneCountInString(evidence.Reasoning) > 1200 || utf8.RuneCountInString(evidence.SourceQuote) > 2048 {
		return evidence, 0, fmt.Errorf("memory evidence is too long")
	}
	if evidence.SourceQuote != "" {
		rawQuote := evidence.SourceQuote
		trimmedQuote := trimQuoteBoundaries(rawQuote)
		speakerStripped := stripSpeakerPrefix(rawQuote)
		normQuote := normalizePunctuation(rawQuote)
		cleanQuote := cleanRunesOnly(rawQuote)
		cleanQuoteRunes := utf8.RuneCountInString(cleanQuote)

		for _, x := range excerpts {
			// Tier 1: Direct exact match
			if strings.Contains(x.Text, rawQuote) {
				evidence.SourceNodeID = x.NodeID
				break
			}
			// Tier 2: Trimmed quote boundary match
			if trimmedQuote != "" && trimmedQuote != rawQuote && strings.Contains(x.Text, trimmedQuote) {
				evidence.SourceNodeID = x.NodeID
				evidence.SourceQuote = trimmedQuote
				break
			}
			// Tier 3: Speaker stripped match
			if speakerStripped != "" && speakerStripped != rawQuote && speakerStripped != trimmedQuote && strings.Contains(x.Text, speakerStripped) {
				evidence.SourceNodeID = x.NodeID
				evidence.SourceQuote = speakerStripped
				break
			}
			// Tier 4: Punctuation-normalized match
			if normQuote != "" && strings.Contains(normalizePunctuation(x.Text), normQuote) {
				evidence.SourceNodeID = x.NodeID
				if trimmedQuote != "" {
					evidence.SourceQuote = trimmedQuote
				}
				break
			}
			// Tier 5: Clean runes match (requires >= 8 runes to avoid false positive collisions, T3.3)
			if cleanQuoteRunes >= 8 && strings.Contains(cleanRunesOnly(x.Text), cleanQuote) {
				evidence.SourceNodeID = x.NodeID
				if trimmedQuote != "" {
					evidence.SourceQuote = trimmedQuote
				}
				break
			}
		}
		if evidence.SourceNodeID == "" {
			return evidence, 0, fmt.Errorf("sourceQuote is not verbatim in the recent turns")
		}
	}
	if evidence.Confidence == "" {
		evidence.Confidence = "medium"
	}
	if evidence.Confidence == "high" && utf8.RuneCountInString(evidence.SourceQuote) < 6 {
		evidence.Confidence = "medium"
		evidence.AutoDowngraded = true
	}
	if evidence.Confidence == "medium" && evidence.SourceQuote == "" {
		evidence.Confidence = "low"
		evidence.AutoDowngraded = true
	}
	switch evidence.Confidence {
	case "high":
		return evidence, 0.95, nil
	case "medium":
		return evidence, 0.75, nil
	case "low":
		return evidence, 0.4, nil
	default:
		return evidence, 0, fmt.Errorf("unknown confidence %q", evidence.Confidence)
	}
}

func ValidateCognitiveMemory(m CognitiveMemory, kind MemoryKind, state *WorldState, excerpts []EvidenceExcerpt) (*MemoryRecord, error) {
	content := strings.TrimSpace(m.Content)
	if content == "" || utf8.RuneCountInString(content) > MaxMemoryContentRunes {
		return nil, fmt.Errorf("memory content must contain 1..1200 characters")
	}
	if len(m.EntityIDs) > 16 || len(m.OwnerIDs) > 8 {
		return nil, fmt.Errorf("too many memory entities")
	}
	resolvedEntities := make([]string, 0, len(m.EntityIDs))
	for _, eid := range m.EntityIDs {
		resolved := ResolveEntityID(eid, state)
		_, char := state.Characters[resolved]
		_, item := state.Items[resolved]
		_, promise := state.Promises[resolved]
		if !char && !item && !promise {
			return nil, fmt.Errorf("unknown memory entity %q", eid)
		}
		resolvedEntities = append(resolvedEntities, resolved)
	}
	resolvedOwners := make([]string, 0, len(m.OwnerIDs))
	for _, oid := range m.OwnerIDs {
		resolved := ResolveEntityID(oid, state)
		if _, ok := state.Characters[resolved]; !ok {
			return nil, fmt.Errorf("unknown memory owner %q", oid)
		}
		resolvedOwners = append(resolvedOwners, resolved)
	}
	evidence, confidence, err := ValidateMemoryEvidence(kind, MemoryEvidence{SourceQuote: m.SourceQuote, Reasoning: m.Reasoning, Confidence: m.Confidence}, excerpts)
	if err != nil {
		return nil, err
	}
	return &MemoryRecord{
		Kind:       kind,
		Content:    content,
		EntityIDs:  resolvedEntities,
		OwnerIDs:   resolvedOwners,
		Importance: ClampMemoryImportance(m.Importance),
		Confidence: confidence,
		Evidence:   &evidence,
		SubjectKey: NormalizeSubjectKey(m.SubjectKey),
	}, nil
}
