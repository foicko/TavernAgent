package domain

import "testing"

func TestLegacyItemReplayAndNewConsumptionHaveExplicitSemantics(t *testing.T) {
	base := NewWorldState()
	base.Items["tea"] = ItemInstance{InstanceID: "tea", Quantity: 5, OwnerID: "player", LocationID: "tavern"}
	event := &DomainEvent{Type: EventItemConsume, PayloadJSON: `{"itemId":"tea","from":"player","to":"consumed","quantity":2}`}
	old := base.Clone()
	if err := ReplayEvents(old, []*DomainEvent{event}); err != nil {
		t.Fatal(err)
	}
	if old.Items["tea"].OwnerID != "consumed" || old.Items["tea"].Quantity != 5 || old.Items["tea"].LocationID != "tavern" {
		t.Fatal("legacy history was reinterpreted")
	}
	if err := VersionItemEvent(event); err != nil {
		t.Fatal(err)
	}
	current := base.Clone()
	if err := ReplayEvents(current, []*DomainEvent{event}); err != nil {
		t.Fatal(err)
	}
	if current.Items["tea"].Quantity != 3 || current.Items["tea"].OwnerID != "player" {
		t.Fatal("new consumption did not reduce the player's stack")
	}
	projected := base.Clone()
	if _, err := ApplyEvents(projected, []*DomainEvent{event}); err != nil || projected.HashID() != current.HashID() {
		t.Fatalf("new-event validation and stored replay diverged: %v", err)
	}
}

func TestFutureItemProjectionIsRejected(t *testing.T) {
	if err := ReplayEvents(NewWorldState(), []*DomainEvent{{Type: EventItemGrant, PayloadJSON: `{"stateProjectionVersion":99,"item":{"instanceId":"x","quantity":1}}`}}); err == nil {
		t.Fatal("unknown future semantics were accepted")
	}
}
