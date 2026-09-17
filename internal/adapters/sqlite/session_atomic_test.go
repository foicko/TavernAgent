package sqlite

import (
	"context"
	"errors"
	"testing"

	"tavernagent/internal/application"
)

func TestSessionSetupRollsBackInitialSnapshotFailure(t *testing.T) {
	st := newTestStore(t)
	_, err := st.db.Exec("CREATE TRIGGER reject_initial_snapshot BEFORE INSERT ON state_snapshots BEGIN SELECT RAISE(ABORT, 'snapshot unavailable'); END")
	if err != nil {
		t.Fatal(err)
	}
	const card = "{\"schemaVersion\":2,\"cardId\":\"atomic-card\",\"name\":\"Guide\",\"characters\":[{\"characterId\":\"guide\",\"name\":\"Guide\"}],\"openingVariants\":[{\"variantId\":\"start\",\"text\":\"A new journey.\"}]}"
	svc := application.NewSessionService(st)
	for attempt := 0; attempt < 2; attempt++ {
		_, err := svc.Setup(context.Background(), &application.SessionSetupRequest{CharacterJSON: card})
		var apiErr *application.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != "STORAGE_UNAVAILABLE" {
			t.Fatalf("expected injected storage failure, got %v", err)
		}
		for _, table := range []string{"sessions", "plot_nodes", "branches", "template_versions", "state_snapshots"} {
			var count int
			if err := st.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Errorf("failed attempt %d left %d rows in %s", attempt, count, table)
			}
		}
	}
	if _, err := st.db.Exec("DROP TRIGGER reject_initial_snapshot"); err != nil {
		t.Fatal(err)
	}
	result, err := svc.Setup(context.Background(), &application.SessionSetupRequest{CharacterJSON: card})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := st.StateAt(result.RootNode.NodeID)
	if err != nil || snapshot.StateHash != result.State.HashID() {
		t.Fatalf("successful setup did not create a readable initial state: %v", err)
	}
	sessions, err := st.ListSessions()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("recovery created duplicate sessions: %d, %v", len(sessions), err)
	}
}
