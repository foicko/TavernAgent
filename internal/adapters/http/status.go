package http

import (
	"net/http"

	ctxpkg "tavernagent/internal/context"
)

// runtimeStatus 暴露运行读数（T0.2）：回合与用量计数、最近若干次调用的
// 真实/估算 token 对照、上下文编译各阶段耗时、提示词版本指纹与消融开关状态。
//
// 设计约束：这是**只读观测端点**。任何一项取不到就降级为空，绝不因为
// 观测失败而返回 5xx——观测不得影响服务可用性。
//
// 单独成文件而不是留在 server.go：server.go 已被架构门禁的文件行数上限
// 与 baseline 棘轮钉住，往里加行会让门禁失败。
func (s *Server) runtimeStatus(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"counters": s.metrics.Snapshot(),
		// 提示词版本与指纹（ADS-7.8-04）：评估报告必须能说明"这一轮跑的是哪份提示词"，
		// 否则成功率变化无法区分来自模型、Harness 还是提示词措辞。
		"prompt": ctxpkg.PromptManifest(),
	}
	// 消融开关（ADS-7.8-01）：基线评估必须能从产物里确认"这一轮确实关了哪些特性"，
	// 而不是靠运行者的记忆。
	out["ablation"] = s.ablation.String()
	if s.usage != nil {
		if totals, err := s.usage.TurnUsageTotals(); err == nil {
			out["usage"] = totals
		}
		if recent, err := s.usage.RecentTurnUsage(10); err == nil {
			out["recent"] = recent
		}
	}
	writeJSON(w, 200, out)
}
