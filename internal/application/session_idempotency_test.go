package application

import (
	"context"
	"errors"
	"sync"
	"testing"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

func TestSessionSetupConcurrentRetriesAndConflictingIntent(t *testing.T) {
	st, _, sessions, _, _, _ := newTestServices(t, happyScript())
	req := &SessionSetupRequest{IdempotencyKey: "one-adventure", CharacterJSON: testCard, OpeningText: "开场", Player: Player{Name: "林舟"}}
	results := make(chan *SetupResult, 6)
	errorsCh := make(chan error, 6)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := sessions.Setup(context.Background(), req)
			results <- res
			errorsCh <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first *SetupResult
	for result := range results {
		if first == nil {
			first = result
		}
		if result.Session.SessionID != first.Session.SessionID || result.State.HashID() != first.State.HashID() {
			t.Fatal("creation retries produced different adventures")
		}
	}
	again, err := NewSessionService(st).Setup(context.Background(), req)
	if err != nil || again.Session.SessionID != first.Session.SessionID {
		t.Fatalf("recreated service lost idempotency: %v", err)
	}
	changed := *req
	changed.Player.Name = "另一位玩家"
	_, err = sessions.Setup(context.Background(), &changed)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("different intent reused the same story: %v", err)
	}
	all, _ := st.ListSessions()
	if len(all) != 2 { // original fixture + one idempotent creation
		t.Fatalf("unexpected session count %d", len(all))
	}
}

type ambiguousSetupStore struct{ ports.Store }

func (s ambiguousSetupStore) CreateSessionWithSnapshot(session *domain.Session, root *domain.PlotNode, branch *domain.Branch, templates []*domain.TemplateVersion, snapshot *domain.StateSnapshot) error {
	if err := s.Store.CreateSessionWithSnapshot(session, root, branch, templates, snapshot); err != nil {
		return err
	}
	return errors.New("injected failure after durable commit")
}

func TestSessionSetupRecoversAmbiguousCommit(t *testing.T) {
	st, _, _, _, _, _ := newTestServices(t, happyScript())
	svc := NewSessionService(ambiguousSetupStore{st})
	result, err := svc.Setup(context.Background(), &SessionSetupRequest{IdempotencyKey: "ambiguous", CharacterJSON: testCard, OpeningText: "开始"})
	if err != nil || result == nil || result.State == nil {
		t.Fatalf("durable creation was reported as failed: %+v %v", result, err)
	}
}
