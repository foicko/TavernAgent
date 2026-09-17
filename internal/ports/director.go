package ports

import (
	"context"
	"tavernagent/internal/domain"
)

type DirectorConflict struct{ Code string }

func (e *DirectorConflict) Error() string { return e.Code }

type DirectorCommit struct {
	SessionID, BranchID, ExpectedHeadID, IdempotencyKey, PayloadHash string
	ExpectedVersion                                                  int64
	DraftVersion                                                     *int64
	Node                                                             *domain.PlotNode
	Change                                                           domain.DirectorChange
}

type DirectorStore interface {
	GetDirectorDraft(branchID string) (*domain.DirectorDraft, error)
	SaveDirectorDraft(draft *domain.DirectorDraft, expectedVersion int64) error
	FindDirectorCommand(sessionID, branchID, key, hash string) (*CommitResult, error)
	CommitDirector(ctx context.Context, command *DirectorCommit) (*CommitResult, error)
	CreateDirectorRequest(request *domain.DirectorRequest) (*domain.DirectorRequest, error)
	GetDirectorRequest(requestID string) (*domain.DirectorRequest, error)
	ListDirectorRequests(branchID string, limit int) ([]*domain.DirectorRequest, error)
	FinishDirectorRequest(requestID, status, reply, failure string, candidate *domain.DirectorPlan) (*domain.DirectorRequest, error)
	RecoverDirectorRequests() error
}
