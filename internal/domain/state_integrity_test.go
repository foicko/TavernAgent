package domain

import (
	"encoding/json"
	"testing"
)

func integrityWorld() *WorldState {
	s := NewWorldState()
	s.Characters["player"] = CharacterInfo{CharacterID: "player", Name: "Player"}
	s.Characters["guide"] = CharacterInfo{CharacterID: "guide", Name: "Guide", Aliases: []string{"Keeper"}, Attributes: map[string]int{"strength": 12}}
	s.Scene = &Scene{SceneID: "inn", Title: "Inn", LocationID: "town"}
	s.Items["potion"] = ItemInstance{InstanceID: "potion", Name: "Potion", OwnerID: "player", Quantity: 5, Aliases: []string{"Medicine"}}
	s.Promises["promise"] = Promise{PromiseID: "promise", ParticipantIDs: []string{"player", "guide"}, Content: "Return", State: PromiseActive}
	s.Moods["guide"] = CharacterMood{MoodCode: "calm", Text: "Waiting"}
	return s
}

func TestStateCloneDoesNotShareMutableData(t *testing.T) {
	for name, change := range map[string]func(*WorldState){
		"scene":                func(s *WorldState) { s.Scene.Title = "Harbor" },
		"item_alias":           func(s *WorldState) { s.Items["potion"].Aliases[0] = "Changed" },
		"promise_participants": func(s *WorldState) { s.Promises["promise"].ParticipantIDs[0] = "guide" },
		"character_alias":      func(s *WorldState) { s.Characters["guide"].Aliases[0] = "Changed" },
		"character_attributes": func(s *WorldState) { s.Characters["guide"].Attributes["strength"] = 99 },
	} {
		t.Run(name, func(t *testing.T) {
			original := integrityWorld()
			before, _ := original.Marshal()
			change(original.Clone())
			after, _ := original.Marshal()
			if before != after {
				t.Fatal("mutating a state copy changed its source")
			}
		})
	}
}

func TestStateHashCoversInventoryAndCharacterState(t *testing.T) {
	for name, change := range map[string]func(*WorldState){
		"scene_title":          func(s *WorldState) { s.Scene.Title = "Harbor" },
		"item_location":        func(s *WorldState) { i := s.Items["potion"]; i.LocationID = "harbor"; s.Items["potion"] = i },
		"item_protection":      func(s *WorldState) { i := s.Items["potion"]; i.Protection = 2; s.Items["potion"] = i },
		"item_name":            func(s *WorldState) { i := s.Items["potion"]; i.Name = "Poison"; s.Items["potion"] = i },
		"promise_text":         func(s *WorldState) { p := s.Promises["promise"]; p.Content = "Never return"; s.Promises["promise"] = p },
		"mood_text":            func(s *WorldState) { s.Moods["guide"] = CharacterMood{MoodCode: "calm", Text: "Leaving"} },
		"character_attributes": func(s *WorldState) { s.Characters["guide"].Attributes["strength"] = 99 },
	} {
		t.Run(name, func(t *testing.T) {
			before := integrityWorld().HashID()
			changed := integrityWorld()
			change(changed)
			if changed.HashID() == before {
				t.Fatal("different authoritative states have the same state hash")
			}
		})
	}
}

func itemEvent(kind EventType, payload ItemTransferPayload) *DomainEvent {
	raw, _ := json.Marshal(payload)
	return &DomainEvent{Type: kind, PayloadJSON: string(raw)}
}

func TestConsumeOnlyRemovesRequestedQuantity(t *testing.T) {
	s := integrityWorld()
	for _, qty := range []int{2, 3} {
		if _, err := ApplyEvent(s, itemEvent(EventItemConsume, ItemTransferPayload{ItemID: "potion", From: "player", To: "consumed", Quantity: qty})); err != nil {
			t.Fatal(err)
		}
		item := s.Items["potion"]
		if qty == 2 && (item.Quantity != 3 || item.OwnerID != "player") {
			t.Fatalf("partial consumption lost the remaining inventory: %+v", item)
		}
		if qty == 3 && (item.Quantity != 0 || item.OwnerID == "player") {
			t.Fatalf("fully consumed item remains in inventory: %+v", item)
		}
	}
	if _, err := ApplyEvent(s, itemEvent(EventItemConsume, ItemTransferPayload{ItemID: "potion", From: "player", Quantity: 1})); err == nil {
		t.Fatal("consumed inventory was usable twice")
	}
}

func TestInventoryRejectsInvalidTransferWithoutChangingState(t *testing.T) {
	for name, payload := range map[string]ItemTransferPayload{
		"wrong_owner":       {ItemID: "potion", From: "guide", To: "player", Quantity: 5},
		"too_many":          {ItemID: "potion", From: "player", To: "guide", Quantity: 6},
		"negative":          {ItemID: "potion", From: "player", To: "guide", Quantity: -1},
		"unknown_recipient": {ItemID: "potion", From: "player", To: "ghost", Quantity: 5},
		"partial_instance":  {ItemID: "potion", From: "player", To: "guide", Quantity: 2},
	} {
		t.Run(name, func(t *testing.T) {
			s := integrityWorld()
			before, _ := s.Marshal()
			if _, err := ApplyEvent(s, itemEvent(EventItemTransfer, payload)); err == nil {
				t.Fatal("invalid transfer accepted")
			}
			after, _ := s.Marshal()
			if after != before {
				t.Fatal("rejected transfer changed inventory")
			}
		})
	}
	s := integrityWorld()
	if _, err := ApplyEvent(s, itemEvent(EventItemTransfer, ItemTransferPayload{ItemID: "potion", From: "player", To: "guide", Quantity: 5})); err != nil {
		t.Fatal(err)
	}
	if s.Items["potion"].OwnerID != "guide" || s.Items["potion"].Quantity != 5 {
		t.Fatal("whole-instance transfer did not conserve quantity")
	}
}
