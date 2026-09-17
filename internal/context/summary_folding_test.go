package context

import (
	"fmt"
	"reflect"
	"testing"

	"tavernagent/internal/domain"
)

func TestSummaryFoldingPreservesGapsAndConfiguredTail(t *testing.T) {
	history := []*domain.PlotNode{}
	for i := 1; i <= 10; i++ {
		history = append(history, &domain.PlotNode{NodeID: fmt.Sprint(i), Depth: i, Kind: domain.NodeKindTurn})
	}
	summaries := []*domain.SummaryArtifact{{FromNodeID: "3", ToNodeID: "4"}, {FromNodeID: "7", ToNodeID: "10"}}
	c := &Compiler{options: CompilerOptions{CompactionPolicy: CompactionPolicy{TailWindowTurns: 2}}}
	got, err := c.foldSummarizedHistory(history, summaries)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, node := range got {
		ids = append(ids, node.NodeID)
	}
	if !reflect.DeepEqual(ids, []string{"1", "2", "5", "6", "9", "10"}) {
		t.Fatalf("uncovered gaps or protected tail were lost: %v", ids)
	}
}
