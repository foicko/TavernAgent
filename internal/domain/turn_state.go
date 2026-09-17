package domain

// Terminal states are immutable. A retry is a new request, never a rewrite of
// the old result. Continuation is the only transition out of a paused draft.
func (s TurnStatus) Terminal() bool {
	return s == TurnCommitted || s == TurnCancelled || s == TurnFailed || s == TurnConflicted
}

func CanTransitionTurn(from, to TurnStatus) bool {
	if from.Terminal() || from == to {
		return false
	}
	if to == TurnCancelled || to == TurnFailed || to == TurnConflicted {
		return true
	}
	switch to {
	case TurnPreparing:
		return from == TurnAccepted || from == TurnQueued || from == TurnAwaitingContinuation
	case TurnGenerating:
		return from == TurnPreparing
	case TurnValidating:
		return from == TurnQueued || from == TurnPreparing || from == TurnGenerating
	case TurnAwaitingContinuation:
		return from == TurnPreparing || from == TurnGenerating || from == TurnValidating
	case TurnCommitted:
		return from == TurnValidating
	}
	return false
}
