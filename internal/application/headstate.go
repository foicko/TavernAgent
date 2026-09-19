package application

import (
	"tavernagent/internal/domain"
)

// nodeReader 是回溯父链所需的最小读取面（ports.Store 与各服务的依赖接口都满足它）。
type nodeReader interface {
	GetNode(nodeID string) (*domain.PlotNode, error)
}

// isMaintenanceNode 判断"不推进叙事位置"的非对话节点。
//
// 后台认知（memory_change）与导演事件（director_event）只更新状态与记忆，
// 它们会追加为分支新头、把 branch.version 往前推，但故事仍然停在最近一轮对话上
// （domain.PlotNode.TurnNumber 的注释即是此约定：非对话事件不增加回合数）。
func isMaintenanceNode(kind domain.NodeKind) bool {
	return kind == domain.NodeKindMemoryChange || kind == domain.NodeKindDirectorEvent
}

// headMatchesClient 判断客户端缓存的 head/version 是否仍指向同一个叙事位置。
//
// 为什么不能只做严格相等：维护节点由后台写入，客户端**根本来不及知道**——
// 它读到视图之后、提交之前，认知节点就可能已经落地并抬高了版本。严格相等会把
// 这种"只多写了记忆"的情况误报成"分支已变化"，读者看到的是一个要求刷新的死胡同，
// 而他的选择其实完全有效。选项来源的校验（validateTurnInput）早已按同一规则沿父链
// 回溯，这里只是把口径统一起来。
//
// 容忍范围被刻意收得很窄：沿父链回溯时**必须一路都是维护节点**，遇到对话节点立即
// 判定为过期。因此"另一个回合已经走完"依旧会被拒绝——那才是真正需要刷新重选的情况。
func headMatchesClient(store nodeReader, branch *domain.Branch, expectedHeadID string, expectedVersion int64) bool {
	if expectedHeadID == "" || branch.HeadNodeID == expectedHeadID {
		return branch.Version == expectedVersion
	}
	// 版本只会随新头推进而增加；比客户端还小的版本说明对不上号，不必回溯。
	if branch.Version < expectedVersion {
		return false
	}
	cursor := branch.HeadNodeID
	for cursor != "" && cursor != expectedHeadID {
		node, err := store.GetNode(cursor)
		if err != nil || node == nil || node.ParentID == "" || !isMaintenanceNode(node.Kind) {
			return false
		}
		cursor = node.ParentID
	}
	return cursor == expectedHeadID
}
