package application

import (
	"encoding/json"
	"tavernagent/internal/domain"
	"tavernagent/internal/protocol"
)

func directorProgressEvent(state *domain.DirectorState, raw json.RawMessage, blocks []domain.TextBlock, nodeID string) *domain.DomainEvent {
	if state == nil || state.Status != "active" || state.CurrentBeat() == nil {
		return nil
	}
	r := domain.DirectorReport{RevisionID: state.Plan.RevisionID, BeatID: state.CurrentBeatID, Status: "continue"}
	var supplied domain.DirectorReport
	parsed := len(raw) > 0 && protocol.DecodeStrictJSON(raw, &supplied) == nil
	if parsed {
		revisionRef, beatRef := state.ReportReferences()
		if supplied.RevisionID == revisionRef {
			supplied.RevisionID = state.Plan.RevisionID
		}
		if supplied.BeatID == beatRef {
			supplied.BeatID = state.CurrentBeatID
		}
	}
	valid := parsed && supplied.RevisionID == state.Plan.RevisionID && supplied.BeatID == state.CurrentBeatID && len(supplied.Reason) <= 6000
	valid = valid && (supplied.Status == "continue" || supplied.Status == "blocked" || supplied.Status == "completed")
	evidenceErr := domain.ValidateDirectorEvidence(supplied, blocks)
	if valid && evidenceErr == nil {
		r = supplied
		if r.Status == "blocked" && r.Reason == "" {
			r.Reason = "当前剧情与阶段安排存在冲突，可在导演模式中调整大纲。"
		}
	} else {
		r.Reason = "本轮没有有效的阶段完成判断，已保留当前阶段；可在导演模式中手动纠正。"
		if parsed && (supplied.RevisionID != state.Plan.RevisionID || supplied.BeatID != state.CurrentBeatID) {
			r.Reason = "本轮导演报告的大纲版本或阶段不匹配，已保留当前阶段；可重试或手动纠正。"
		} else if valid && evidenceErr != nil {
			r.Reason = evidenceErr.Error() + "，已保留当前阶段；可在导演模式中手动纠正。"
		}
	}
	change := domain.DirectorChange{Action: "report", BaseRevisionID: state.Plan.RevisionID, Report: &r}
	payload, _ := json.Marshal(change)
	return &domain.DomainEvent{NodeID: nodeID, Type: domain.EventDirectorChange, PayloadJSON: string(payload)}
}
