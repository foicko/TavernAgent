package context

import (
	"fmt"
	"strings"
	"tavernagent/internal/domain"
	"testing"
)

func TestTailPolicyAdaptsToWindowAndKeepsCompleteLatestTurn(t *testing.T) {
	var nodes []*domain.PlotNode
	for i := 0; i < 40; i++ {
		nodes = append(nodes, mustTurnID(fmt.Sprint(i), strings.Repeat("故事", 250)))
	}
	previous := 0
	for _, window := range []int{8192, 32768, 131072} {
		policy := ResolveTailPolicy(nodes, DefaultCompactionPolicy(), window-4096-2500, true, 0)
		if policy.TailWindowTurns < 1 || policy.TailWindowTurns > 20 || policy.TailWindowTurns < previous {
			t.Fatalf("window %d: %+v", window, policy)
		}
		previous = policy.TailWindowTurns
	}
	if got := ResolveTailPolicy(nodes, DefaultCompactionPolicy(), 1, true, 0); got.TailWindowTurns != 1 {
		t.Fatalf("latest complete turn must survive: %+v", got)
	}
}
