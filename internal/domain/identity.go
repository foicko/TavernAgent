package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CardIdentity is shared by card import and legacy-session migration. Identity
// must not be guessed from a display name, which users are free to change.
func CardIdentity(raw string) string {
	var card struct {
		CardID     string `json:"cardId"`
		Name       string `json:"name"`
		Characters []struct {
			Name string `json:"name"`
		} `json:"characters"`
	}
	if json.Unmarshal([]byte(raw), &card) != nil {
		return ""
	}
	if card.CardID != "" {
		return card.CardID
	}
	if card.Name == "" && len(card.Characters) > 0 {
		card.Name = card.Characters[0].Name
	}
	sum := sha256.Sum256([]byte(raw + ":" + card.Name))
	return hex.EncodeToString(sum[:])[:16]
}
