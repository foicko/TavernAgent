package http

import (
	"encoding/json"
	"net/http"
)

// 回合读取端点：终态返回结果节点，生成中额外返回**在途正文快照**。
// 快照是"正在写的这一段"的权威文本——它的行内增量是 Sequence 0 的临时事件，
// 断线或刷新后不会重放，客户端只能靠这里补齐。
func (s *Server) getTurn(w http.ResponseWriter, r *http.Request) {
	turn, err := s.turns.Get(r.PathValue("turnId"))
	if err != nil {
		writeError(w, 404, "NOT_FOUND", "回合不存在", false, "")
		return
	}
	resp := map[string]any{
		"turnId":          turn.TurnID,
		"sessionId":       turn.SessionID,
		"branchId":        turn.BranchID,
		"status":          turn.Status,
		"mode":            turn.Mode,
		"input":           json.RawMessage(turn.InputJSON),
		"expectedHeadId":  turn.ExpectedHeadID,
		"expectedVersion": turn.ExpectedVersion,
		"failureCode":     turn.FailureCode,
		"failureMessage":  turn.FailureMessage,
	}
	if turn.ResultNodeID != "" {
		resp["resultNodeId"] = turn.ResultNodeID
		if nv, nerr := s.sessions.NodeView(turn.ResultNodeID); nerr == nil {
			resp["resultNode"] = nv.Node
		}
	}
	// 在途正文快照：当前块的行内增量是 Sequence 0 的临时事件，断线/刷新后不会重放，
	// 客户端靠这个字段把"正在写的这一段"补齐。已完成的块仍走 durable 事件回放。
	if partial, ok := s.turns.PartialDraft(turn.TurnID); ok {
		draft := map[string]any{"attemptId": partial.AttemptID}
		if partial.InFlight != nil {
			draft["inFlight"] = map[string]any{
				"seq":       partial.InFlight.Seq,
				"kind":      partial.InFlight.Kind,
				"speakerId": partial.InFlight.SpeakerID,
				"text":      partial.InFlight.Text,
			}
		}
		resp["draft"] = draft
	}
	writeJSON(w, 200, resp)
}
