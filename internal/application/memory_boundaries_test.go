package application

import (
	"errors"
	"strings"
	"testing"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

func TestMemoryCorrectionLengthIsBoundedAndAtomic(t *testing.T) {
	f := newMemoryFixture(t)
	head := f.commitOn(t, f.main.BranchID, "root", memRec("original", "The guide lives near the lighthouse."))
	for _, content := range []string{" \n ", strings.Repeat("记", 1201)} {
		_, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "original", MemoryPatch{Content: &content})
		if err == nil {
			t.Fatal("invalid correction accepted")
		}
		branch, _ := f.store.GetBranch(f.main.BranchID)
		records, _ := f.store.ListMemories(f.sess.SessionID)
		if branch.HeadNodeID != head || len(records) != 1 {
			t.Fatal("invalid correction changed the branch or memory history")
		}
	}
}

type memoryReadFailure struct{ ports.Store }

func (s memoryReadFailure) GetBranch(string) (*domain.Branch, error) {
	return nil, errors.New("temporary read failure")
}

func TestMemoryReadFailureIsNotReportedAsMissingData(t *testing.T) {
	f := newMemoryFixture(t)
	_, _, err := NewMemoryService(memoryReadFailure{f.store}).ListAt(f.sess.SessionID, f.main.BranchID, "")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 503 {
		t.Fatalf("storage failure should be retryable, got %v", err)
	}
}
