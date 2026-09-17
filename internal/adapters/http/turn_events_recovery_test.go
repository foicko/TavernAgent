package http

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// Signal the first completed read so the test appends after the initial replay,
// rather than accidentally passing by reading events that predate the stream.
type observedOutbox struct {
	ports.Store
	firstRead chan struct{}
	once      sync.Once
}

func (s *observedOutbox) PollOutbox(id string, after int64, limit int) ([]*domain.OutboxEvent, error) {
	events, err := s.Store.PollOutbox(id, after, limit)
	s.once.Do(func() { close(s.firstRead) })
	return events, err
}

func TestTurnStreamRecoversDurableCompletion(t *testing.T) {
	for _, notify := range []bool{false, true} {
		name := "missing_notification"
		if notify {
			name = "zero_sequence_notification"
		}
		t.Run(name, func(t *testing.T) {
			store, err := sqlite.Open(t.TempDir(), ports.RealClock{})
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			observed := &observedOutbox{Store: store, firstRead: make(chan struct{})}
			bus := application.NewEventBus(observed)
			server := mustNew(t, Deps{Bus: bus})
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				server.eventStream(w, r, "recovery")
			}))
			defer ts.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, "GET", ts.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := ts.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			select {
			case <-observed.firstRead:
			case <-ctx.Done():
				t.Fatal("stream never performed its initial replay")
			}

			events := []*domain.OutboxEvent{
				{EventID: "block", AggregateID: "recovery", Type: "block.appended", PayloadJSON: `{}`},
				{EventID: "commit", AggregateID: "recovery", Type: "turn.committed", PayloadJSON: `{}`},
			}
			if err := store.AppendOutbox(events); err != nil {
				t.Fatal(err)
			}
			if notify {
				// Commit transactions may broadcast a fresh object without the
				// persisted sequence. Delivering it directly would produce id :0.
				bus.BroadcastOnly(&domain.OutboxEvent{AggregateID: "recovery", Type: "turn.committed", PayloadJSON: `{}`})
			}

			scanner := bufio.NewScanner(response.Body)
			var eventType, eventID string
			seen := 0
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "event: ") {
					eventType = strings.TrimPrefix(line, "event: ")
				} else if strings.HasPrefix(line, "id: ") {
					eventID = strings.TrimPrefix(line, "id: ")
				} else if line == "" && eventType != "" {
					if eventType != events[seen].Type || eventID != fmt.Sprintf("recovery:%d", seen+1) {
						t.Fatalf("event %d = %s / %s; want durable %s / recovery:%d", seen, eventType, eventID, events[seen].Type, seen+1)
					}
					seen++
					if seen == len(events) {
						return
					}
					eventType, eventID = "", ""
				}
			}
			t.Fatalf("received %d/%d durable events: %v", seen, len(events), scanner.Err())
		})
	}
}
