package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"testing"
)

func TestExactCheckpointFastPathStillValidatesIntegrity(t *testing.T) {
	s := newTestStore(t)
	_, root, _ := seedSession(t, s)
	state := domain.NewWorldState()
	if err := s.SaveSnapshot(&domain.StateSnapshot{NodeID: root.NodeID, StateJSON: MarshalState(state), StateHash: "tampered", SnapshotVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StateAt(root.NodeID); err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("corrupt exact checkpoint accepted: %v", err)
	}
	if _, err := s.StateAt("missing-node"); err == nil {
		t.Fatal("missing node accepted")
	}
}

// Additive deltas commute and concealed a tie in the recursive path's depth.
// Replacement state exposes it: the last mood must survive read and checkpoint.
func TestStateReplayOrdersReplacementEventsBeforeMemoryCheckpoint(t *testing.T) {
	s := newTestStore(t)
	session, root, branch := seedSession(t, s)
	state := domain.NewWorldState()
	state.Characters["npc_x"] = domain.CharacterInfo{CharacterID: "npc_x", Name: "记录员", Participant: true}
	if err := s.SaveSnapshot(&domain.StateSnapshot{NodeID: root.NodeID, StateJSON: MarshalState(state), StateHash: state.HashID(), SnapshotVersion: 1}); err != nil {
		t.Fatal(err)
	}
	head := root.NodeID
	for i, mood := range []string{"calm", "concerned", "relieved"} {
		node, turn := fmt.Sprintf("n%d", i), fmt.Sprintf("t%d", i)
		payload, _ := json.Marshal(map[string]any{"characterId": "npc_x", "mood": domain.CharacterMood{MoodCode: mood, Text: mood}})
		event := &domain.DomainEvent{EventID: "e" + turn, NodeID: node, Type: domain.EventMoodSet, PayloadJSON: string(payload)}
		before := state.HashID()
		if _, err := domain.ApplyEvent(state, event); err != nil {
			t.Fatal(err)
		}
		seedTurn(t, s, session.SessionID, branch.BranchID, turn, head, int64(i))
		plan := makeCommitPlan(session.SessionID, turn, branch.BranchID, node, head, int64(i), before, []*domain.DomainEvent{event}, state)
		plan.Node.Depth, plan.Node.TurnNumber = i+1, i+1
		result, err := s.CommitTurn(plan)
		if err != nil || !result.Committed {
			t.Fatalf("commit: %+v %v", result, err)
		}
		head = node
		got, err := s.StateAt(head)
		if err != nil || got.StateHash != state.HashID() {
			t.Fatalf("replacement order lost at %s: %+v %v", node, got, err)
		}
	}
	result, err := s.CommitMemoryBatch(context.Background(), &ports.MemoryBatch{BatchID: "batch", NodeID: "memory-node", SessionID: session.SessionID, BranchID: branch.BranchID, ExpectedHeadID: head, ExpectedVersion: 3, PayloadHash: "batch", Memories: []*domain.MemoryRecord{{MemoryID: "memory", SourceNodeID: "memory-node", Content: "记录员终于放心了。", Kind: domain.MemoryObserved}}})
	if err != nil || !result.Committed {
		t.Fatalf("memory commit: %+v %v", result, err)
	}
	got, err := s.StateAt(result.NewHeadID)
	if err != nil || got.StateHash != state.HashID() {
		t.Fatalf("memory checkpoint disagrees with ordered replay: %+v %v", got, err)
	}
}
