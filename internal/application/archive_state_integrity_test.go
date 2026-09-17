package application

import (
	"bytes"
	"context"
	"testing"
	"time"

	"tavernagent/internal/domain"
	"tavernagent/internal/pack"
)

func TestArchiveRejectsRootStateDisagreement(t *testing.T) {
	st, _, _, sid, _, _ := newTestServices(t, happyScript())
	bundle, _ := st.ExportSession(sid, "")
	state, _ := domain.UnmarshalWorld(bundle.Snapshots[0].StateJSON)
	item := state.Items["item_pocketwatch"]
	item.Protection = 99
	state.Items[item.InstanceID] = item
	bundle.Snapshots[0].StateJSON, _ = state.Marshal()
	bundle.Snapshots[0].StateHash = state.HashID()
	var archive bytes.Buffer
	if _, err := pack.Write(&archive, bundle, "test", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := NewArchiveService(st, "test").Import(context.Background(), archive.Bytes()); err == nil {
		t.Fatal("forged root state replaced the declared initial state")
	}
	sessions, _ := st.ListSessions()
	if len(sessions) != 1 {
		t.Fatal("rejected root snapshot left an imported session")
	}
}

func TestLegacyArchiveItemReplayIndependentOfCheckpoints(t *testing.T) {
	for _, checkpoint := range []bool{false, true} {
		st, _, _, sid, _, root := newTestServices(t, happyScript())
		bundle, _ := st.ExportSession(sid, "")
		state, _ := domain.UnmarshalWorld(bundle.Snapshots[0].StateJSON)
		bundle.Snapshots[0].StateHash = state.HashID()
		bundle.Nodes = append(bundle.Nodes, &domain.PlotNode{NodeID: "legacy-use", SessionID: sid, ParentID: root, Kind: domain.NodeKindTurn, Depth: 1, TurnNumber: 1, SchemaVersion: 1, ContentJSON: `{"blocks":[{"kind":"narration","text":"旧版物品消耗"}]}`})
		bundle.Events = append(bundle.Events, &domain.DomainEvent{EventID: "legacy-event", NodeID: "legacy-use", Type: domain.EventItemConsume, PayloadJSON: `{"itemId":"item_pocketwatch","from":"player","to":"consumed","quantity":1}`})
		bundle.Branches[0].HeadNodeID = "legacy-use"
		item := state.Items["item_pocketwatch"]
		item.OwnerID = "consumed" // v1 retained quantity; replay must not reinterpret it.
		state.Items[item.InstanceID] = item
		if checkpoint {
			raw, _ := state.Marshal()
			bundle.Snapshots = append(bundle.Snapshots, &domain.StateSnapshot{NodeID: "legacy-use", SnapshotVersion: 1, StateJSON: raw, StateHash: state.HashID()})
		}
		var archive bytes.Buffer
		if _, err := pack.Write(&archive, bundle, "legacy-test", time.Now()); err != nil {
			t.Fatal(err)
		}
		imported, err := NewArchiveService(st, "test").Import(context.Background(), archive.Bytes())
		if err != nil {
			t.Fatalf("checkpoint=%v: %v", checkpoint, err)
		}
		branches, _ := st.ListBranches(imported.Session.SessionID)
		snapshot, err := st.StateAt(branches[0].HeadNodeID)
		if err != nil || snapshot.StateHash != state.HashID() {
			t.Fatalf("checkpoint=%v changed historical state: %+v %v", checkpoint, snapshot, err)
		}
	}
}
