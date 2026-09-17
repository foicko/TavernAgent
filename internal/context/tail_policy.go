package context

import (
	"context"
	"tavernagent/internal/domain"
)

// ResolveTailPolicy protects a contiguous suffix of complete turns. Half the
// space left after mandatory context is the target, not a license to truncate
// the latest turn. Budget fitting reports an error when that turn cannot fit.
func ResolveTailPolicy(nodes []*domain.PlotNode, policy CompactionPolicy, available int, enabled bool, calibration float64) CompactionPolicy {
	if policy.TailWindowTurns <= 0 {
		policy.TailWindowTurns = 20
	}
	if policy.MinUncompactedTurns <= 0 {
		policy.MinUncompactedTurns = 8
	}
	if !enabled {
		return policy
	}
	count, used := 0, 0
	for i := len(nodes) - 1; i >= 0 && count < policy.TailWindowTurns; i-- {
		if nodes[i].Kind != domain.NodeKindTurn {
			continue
		}
		content, err := parseTurnContent(nodes[i].ContentJSON)
		if err != nil {
			continue
		}
		cost := calibrateTokens(EstimateTokens(content.InputText)+EstimateTokens(renderBlocks(content.Blocks))+8, calibration)
		if count > 0 && used+cost > max(0, available)/2 {
			break
		}
		count++
		used += cost
	}
	policy.TailWindowTurns = max(1, count)
	policy.PruneThoughtDepth = policy.TailWindowTurns
	return policy
}

// CompactionPolicyAt uses the generation model's window and the same mandatory
// material as Compile. Background model limits only bound summary input/output.
func (c *Compiler) CompactionPolicyAt(ctx context.Context, sessionID, nodeID string, nodes []*domain.PlotNode, state *domain.WorldState) (CompactionPolicy, error) {
	if err := ctx.Err(); err != nil {
		return CompactionPolicy{}, err
	}
	sc := c.loadSessionContext(sessionID)
	var err error
	sc.Director, err = c.store.DirectorAt(nodeID)
	if err != nil {
		return CompactionPolicy{}, err
	}
	ledger, _, err := c.compileLedger(nodeID, state, sc)
	if err != nil {
		return CompactionPolicy{}, err
	}
	mandatory := c.EstimateMessages(c.buildMessages(nil, nil, nil, nil, nil, sc, state, "", ledger, 0))
	return ResolveTailPolicy(nodes, c.options.CompactionPolicy, c.options.InputBudget()-mandatory, c.options.ContextWindow > 0, c.options.TokenCalibration), nil
}
