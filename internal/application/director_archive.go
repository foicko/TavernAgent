package application

import (
	"encoding/json"
	"fmt"
	"tavernagent/internal/domain"
)

func validateBundleDirector(bundle *domain.SessionBundle, nodes map[string]*domain.PlotNode) error {
	events := map[string]*domain.DomainEvent{}
	worldEvents := map[string]bool{}
	for _, e := range bundle.Events {
		if e.Type != domain.EventDirectorChange {
			worldEvents[e.NodeID] = true
			continue
		}
		if bundle.FormatVersion != domain.DirectorPackFormatVersion {
			return Err("INVALID_PACK", "导演事件需要剧情包格式 v2", 400)
		}
		if events[e.NodeID] != nil {
			return Err("INVALID_PACK", "每个节点只能包含一个导演变更", 400)
		}
		events[e.NodeID] = e
	}
	children := map[string][]*domain.PlotNode{}
	for _, n := range nodes {
		children[n.ParentID] = append(children[n.ParentID], n)
		if n.Kind == domain.NodeKindDirectorEvent && events[n.NodeID] == nil {
			return Err("INVALID_PACK", "导演节点缺少事件", 400)
		}
	}
	if len(events) == 0 {
		return nil
	}
	states := map[string]*domain.DirectorState{}
	queue := []*domain.PlotNode{nodes[bundle.RootNodeID]}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		state := states[n.ParentID]
		if ev := events[n.NodeID]; ev != nil {
			var c domain.DirectorChange
			if err := json.Unmarshal([]byte(ev.PayloadJSON), &c); err != nil {
				return Err("INVALID_PACK", "导演事件无效", 400)
			}
			if c.Action == "report" {
				if n.Kind != domain.NodeKindTurn || c.Report == nil {
					return Err("INVALID_PACK", "导演报告必须属于剧情回合", 400)
				}
				var tc domain.TurnContent
				if err := json.Unmarshal([]byte(n.ContentJSON), &tc); err != nil {
					return Err("INVALID_PACK", "报告正文无效", 400)
				}
				if err := domain.ValidateDirectorEvidence(*c.Report, tc.Blocks); err != nil {
					return Err("INVALID_PACK", err.Error(), 400)
				}
			} else {
				parent := nodes[n.ParentID]
				if n.Kind != domain.NodeKindDirectorEvent || parent == nil || n.TurnNumber != parent.TurnNumber || n.Depth != parent.Depth+1 {
					return Err("INVALID_PACK", "导演节点轮次或深度无效", 400)
				}
				if worldEvents[n.NodeID] {
					return Err("INVALID_PACK", "导演节点不能修改世界事实", 400)
				}
			}
			var err error
			state, err = domain.ApplyDirectorEvent(state, ev)
			if err != nil {
				return Err("INVALID_PACK", fmt.Sprintf("导演状态无法重放: %v", err), 400)
			}
		}
		states[n.NodeID] = state
		queue = append(queue, children[n.NodeID]...)
	}
	return nil
}
