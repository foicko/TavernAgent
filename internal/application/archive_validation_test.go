package application

import (
	"testing"

	"tavernagent/internal/domain"
)

// memory_change 负载形状必须在导入期校验，避免后台压缩时 panic（S1/M13）。
func TestValidateNodePayloadsRejectsNullMemoryChange(t *testing.T) {
	nodes := map[string]*domain.PlotNode{
		"n1": {NodeID: "n1", Kind: domain.NodeKindMemoryChange, ContentJSON: `{"memoryChanges":[null]}`},
	}
	if err := validateNodePayloads(nodes); err == nil {
		t.Fatal("expected error for null memory change")
	}
}

func TestValidateNodePayloadsRejectsMissingMemoryID(t *testing.T) {
	nodes := map[string]*domain.PlotNode{
		"n1": {NodeID: "n1", Kind: domain.NodeKindMemoryChange, ContentJSON: `{"memoryChanges":[{}]}`},
	}
	if err := validateNodePayloads(nodes); err == nil {
		t.Fatal("expected error for memory change without id")
	}
}

func TestValidateNodePayloadsAcceptsValidAndOtherKinds(t *testing.T) {
	nodes := map[string]*domain.PlotNode{
		"n1": {NodeID: "n1", Kind: domain.NodeKindMemoryChange, ContentJSON: `{"memoryChanges":[{"memoryId":"m1","content":"x"}]}`},
		"n2": {NodeID: "n2", Kind: domain.NodeKindTurn, ContentJSON: `{}`},
	}
	if err := validateNodePayloads(nodes); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
