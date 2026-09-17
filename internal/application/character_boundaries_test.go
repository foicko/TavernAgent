package application

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"tavernagent/internal/domain"
	"tavernagent/internal/protocol"
)

func TestCharacterProposalsRespectIdentityAndBounds(t *testing.T) {
	base := domain.NewWorldState()
	base.Characters["player"] = domain.CharacterInfo{CharacterID: "player", Name: "旅人"}
	base.Characters["npc"] = domain.CharacterInfo{CharacterID: "npc", Name: "店主"}
	base.Relationships["npc"] = domain.RelationValue{Affection: 95, Trust: 3, Alertness: 99}

	for _, tc := range []struct {
		name, character, field string
		deltas                 []int
		want                   domain.RelationValue
	}{
		{"positive cap", "npc", "affection", []int{9, 9}, domain.RelationValue{Affection: 100, Trust: 3, Alertness: 99}},
		{"negative floor", "npc", "trust", []int{-9, -9}, domain.RelationValue{Affection: 95, Trust: 0, Alertness: 99}},
		{"overflow stays positive", "npc", "alertness", []int{math.MaxInt, math.MaxInt}, domain.RelationValue{Affection: 95, Trust: 3, Alertness: 100}},
		{"exact cancellation before clamp", "npc", "trust", []int{math.MaxInt, math.MaxInt, -math.MaxInt, -math.MaxInt, 2}, domain.RelationValue{Affection: 95, Trust: 5, Alertness: 99}},
		{"unknown character ignored", "ghost", "trust", []int{10}, base.Relationships["npc"]},
		{"player ignored", "player", "trust", []int{10}, base.Relationships["npc"]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draft := protocol.TurnDraft{}
			for _, delta := range tc.deltas {
				draft.Proposals = append(draft.Proposals, protocol.Proposal{Type: "relationship_delta", CharacterID: tc.character, Field: tc.field, Delta: delta})
			}
			draft.Proposals = append(draft.Proposals,
				protocol.Proposal{Type: "mood_set", CharacterID: "ghost", MoodCode: "angry", Text: "不存在的人物"},
				protocol.Proposal{Type: "goal_set", CharacterID: "ghost", Text: "不存在的目标"},
				protocol.Proposal{Type: "mood_set", CharacterID: "npc", MoodCode: "calm", Text: "终于放松下来"},
				protocol.Proposal{Type: "goal_set", CharacterID: "npc", Text: "等待旅人回来"})
			before := base.HashID()
			plan, err := buildPlan(base, draft, domain.TurnInput{}, "structured", "node", planContext{ruleset: domain.DefaultRuleset(), RulesetVersion: RulesetVersion})
			if err != nil {
				t.Fatal(err)
			}
			if got := plan.NewState.Relationships["npc"]; got != tc.want {
				t.Fatalf("relationship=%+v, want %+v", got, tc.want)
			}
			if len(plan.NewState.Relationships) != 1 || len(plan.NewState.Moods) != 1 || len(plan.NewState.Goals) != 1 {
				t.Fatalf("unknown/player state leaked: %+v", plan.NewState)
			}
			if plan.NewState.Moods["npc"].Text != "终于放松下来" || plan.NewState.Goals["goal_npc"].Text != "等待旅人回来" {
				t.Fatal("valid character state was lost")
			}
			replayed := base.Clone()
			if _, err := domain.ApplyEvents(replayed, plan.Events); err != nil || replayed.HashID() != plan.NewState.HashID() || base.HashID() != before {
				t.Fatalf("projection differs or base mutated: %v", err)
			}
		})
	}
}

func TestInvalidCardStateDoesNotCreateSession(t *testing.T) {
	st, _, sessions, _, _, _ := newTestServices(t, happyScript())
	for _, tc := range []struct {
		name string
		edit func(*CharacterCard)
	}{
		{"duplicate character", func(c *CharacterCard) { c.Characters = append(c.Characters, c.Characters[0]) }},
		{"reserved player", func(c *CharacterCard) { c.Characters[0].CharacterID = "player" }},
		{"reserved consumed", func(c *CharacterCard) { c.Characters[0].CharacterID = "consumed" }},
		{"blank character", func(c *CharacterCard) { c.Characters[0].CharacterID = "  " }},
		{"affection outside range", func(c *CharacterCard) {
			c.InitialState = &CharacterInitialState{Relationships: map[string]domain.RelationValue{c.Characters[0].CharacterID: {Affection: 101}}}
		}},
		{"trust below zero", func(c *CharacterCard) {
			c.InitialState = &CharacterInitialState{Relationships: map[string]domain.RelationValue{c.Characters[0].CharacterID: {Trust: -1}}}
		}},
		{"alertness outside range", func(c *CharacterCard) {
			c.InitialState = &CharacterInitialState{Relationships: map[string]domain.RelationValue{c.Characters[0].CharacterID: {Alertness: 101}}}
		}},
		{"unknown relationship", func(c *CharacterCard) {
			c.InitialState = &CharacterInitialState{Relationships: map[string]domain.RelationValue{"ghost": {Trust: 5}}}
		}},
		{"unknown mood", func(c *CharacterCard) {
			c.InitialState = &CharacterInitialState{Moods: map[string]domain.CharacterMood{"ghost": {MoodCode: "calm"}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			card, err := ParseCharacterCard(testCard)
			if err != nil {
				t.Fatal(err)
			}
			tc.edit(card)
			raw, _ := json.Marshal(card)
			before, _ := st.ListSessions()
			if _, err := sessions.Setup(context.Background(), &SessionSetupRequest{CharacterJSON: string(raw), Player: Player{Name: "旅人"}, OpeningText: "开场"}); err == nil {
				t.Fatal("invalid initial state was accepted")
			}
			after, _ := st.ListSessions()
			if len(after) != len(before) {
				t.Fatal("invalid card left a session")
			}
		})
	}
}
