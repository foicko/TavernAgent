package ports

import "tavernagent/internal/domain"

// TurnTransition fences delayed workers and atomically persists status, branch
// ownership, receipt abandonment and the durable lifecycle event.
type TurnTransition struct {
	TurnID, AttemptID                         string
	Expected                                  []domain.TurnStatus
	Status                                    domain.TurnStatus
	ResultNodeID, FailureCode, FailureMessage string
	Retryable                                 bool
}
