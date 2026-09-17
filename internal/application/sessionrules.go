package application

import (
	"encoding/json"
	"sort"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

func (s *TurnService) sessionRules(sessionID string) (domain.Ruleset, error) {
	return rulesForSession(s.store, sessionID, s.ruleset)
}

func rulesForSession(store interface {
	ports.SessionStore
	ports.StoryStore
}, sessionID string, fallback domain.Ruleset) (domain.Ruleset, error) {
	sess, err := store.GetSession(sessionID)
	if err != nil {
		return domain.Ruleset{}, err
	}
	root, err := store.GetNode(sess.RootNodeID)
	if err != nil {
		return domain.Ruleset{}, err
	}
	var refs struct {
		Templates map[string]struct {
			ID string `json:"templateVersionId"`
		} `json:"templates"`
	}
	if err := json.Unmarshal([]byte(root.ContentJSON), &refs); err != nil {
		return domain.Ruleset{}, err
	}
	if ref := refs.Templates["rules"]; ref.ID != "" {
		tpl, err := store.GetTemplateVersion(ref.ID)
		if err != nil {
			return domain.Ruleset{}, err
		}
		var rules domain.Ruleset
		if err := json.Unmarshal([]byte(tpl.Content), &rules); err != nil {
			return domain.Ruleset{}, err
		}
		return rules, domain.ValidateRuleset(rules)
	}
	return fallback, nil
}

type ActionView struct {
	ActionID  string   `json:"actionId"`
	Label     string   `json:"label"`
	Attribute string   `json:"attribute"`
	DC        int      `json:"dc"`
	ItemIDs   []string `json:"itemIds,omitempty"`
}

func actionViews(rs domain.Ruleset) []ActionView {
	out := make([]ActionView, 0, len(rs.Actions))
	for _, a := range rs.Actions {
		view := ActionView{ActionID: a.ActionID, Label: a.Label, Attribute: a.Attribute, DC: a.DC}
		items := map[string]bool{}
		for _, effects := range a.Consequences {
			for _, effect := range effects {
				if effect.Type != domain.EventItemConsume && effect.Type != domain.EventItemTransfer {
					continue
				}
				var item domain.ItemTransferPayload
				if json.Unmarshal(effect.Payload, &item) == nil && item.ItemID != "" {
					items[item.ItemID] = true
				}
			}
		}
		for id := range items {
			view.ItemIDs = append(view.ItemIDs, id)
		}
		sort.Strings(view.ItemIDs)
		out = append(out, view)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ActionID < out[j].ActionID })
	return out
}
