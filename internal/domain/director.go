package domain

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

// Director plans describe intentions, never world facts or character knowledge.
const EventDirectorChange EventType = "director_change"

type DirectorBeat struct {
	BeatID             string `json:"beatId"`
	Title              string `json:"title"`
	Instruction        string `json:"instruction"`
	CompletionCriteria string `json:"completionCriteria"`
}

type DirectorPlan struct {
	PlanID     string         `json:"planId,omitempty"`
	RevisionID string         `json:"revisionId,omitempty"`
	Title      string         `json:"title"`
	Guidance   string         `json:"guidance"`
	Beats      []DirectorBeat `json:"beats"`
}

type DirectorEvidence struct {
	BlockSeq int    `json:"blockSeq"`
	Quote    string `json:"quote"`
}

type DirectorReport struct {
	RevisionID string             `json:"revisionId"`
	BeatID     string             `json:"beatId"`
	Status     string             `json:"status"` // continue | completed | blocked
	Evidence   []DirectorEvidence `json:"evidence,omitempty"`
	Reason     string             `json:"reason,omitempty"`
}

type DirectorBeatProgress struct {
	Status       string             `json:"status"` // completed | skipped
	SourceNodeID string             `json:"sourceNodeId"`
	Evidence     []DirectorEvidence `json:"evidence,omitempty"`
}

type DirectorState struct {
	Plan          DirectorPlan                    `json:"plan"`
	Status        string                          `json:"status"` // active | paused | completed
	CurrentBeatID string                          `json:"currentBeatId"`
	Progress      map[string]DirectorBeatProgress `json:"progress"`
	Warning       string                          `json:"warning,omitempty"`
}

type DirectorSummary struct {
	Title            string `json:"title"`
	RevisionID       string `json:"revisionId"`
	Status           string `json:"status"`
	CurrentBeatID    string `json:"currentBeatId"`
	CurrentBeatTitle string `json:"currentBeatTitle"`
	Completed        int    `json:"completed"`
	Total            int    `json:"total"`
	Warning          string `json:"warning,omitempty"`
}

func (s *DirectorState) Summary() *DirectorSummary {
	if s == nil {
		return nil
	}
	out := &DirectorSummary{Title: s.Plan.Title, RevisionID: s.Plan.RevisionID, Status: s.Status, CurrentBeatID: s.CurrentBeatID, Completed: len(s.Progress), Total: len(s.Plan.Beats), Warning: s.Warning}
	if beat := s.CurrentBeat(); beat != nil {
		out.CurrentBeatTitle = beat.Title
	}
	return out
}

type DirectorChange struct {
	Action         string          `json:"action"`
	Plan           *DirectorPlan   `json:"plan,omitempty"`
	BaseRevisionID string          `json:"baseRevisionId,omitempty"`
	Replace        bool            `json:"replace,omitempty"`
	BeatID         string          `json:"beatId,omitempty"`
	Report         *DirectorReport `json:"report,omitempty"`
}

type DirectorDraft struct {
	SessionID      string       `json:"sessionId"`
	BranchID       string       `json:"branchId"`
	Version        int64        `json:"version"`
	BaseRevisionID string       `json:"baseRevisionId"`
	Plan           DirectorPlan `json:"plan"`
	UpdatedAt      time.Time    `json:"updatedAt"`
}

// Requests also form the durable, branch-local discussion transcript.
type DirectorRequest struct {
	RequestID      string        `json:"requestId"`
	SessionID      string        `json:"sessionId"`
	BranchID       string        `json:"branchId"`
	IdempotencyKey string        `json:"-"`
	PayloadHash    string        `json:"-"`
	BaseNodeID     string        `json:"baseNodeId"`
	DraftVersion   int64         `json:"draftVersion"`
	DraftJSON      string        `json:"-"`
	Text           string        `json:"text"`
	Status         string        `json:"status"` // generating | completed | failed | cancelled | interrupted
	Reply          string        `json:"reply,omitempty"`
	Candidate      *DirectorPlan `json:"candidate,omitempty"`
	DraftApplied   bool          `json:"draftApplied"`
	Error          string        `json:"error,omitempty"`
	CreatedAt      time.Time     `json:"createdAt"`
}

func (s *DirectorState) CurrentBeat() *DirectorBeat {
	if s == nil {
		return nil
	}
	for i := range s.Plan.Beats {
		if s.Plan.Beats[i].BeatID == s.CurrentBeatID {
			return &s.Plan.Beats[i]
		}
	}
	return nil
}

// ReportReferences are compact, deterministic model-facing identifiers. They
// remain bound to this immutable revision and beat; persisted events always
// contain the canonical IDs. This avoids asking models to transcribe UUIDs.
func (s *DirectorState) ReportReferences() (revision, beat string) {
	revisionHash := sha256.Sum256([]byte(s.Plan.RevisionID))
	beatHash := sha256.Sum256([]byte(s.Plan.RevisionID + "\x00" + s.CurrentBeatID))
	return fmt.Sprintf("rev_%x", revisionHash[:12]), fmt.Sprintf("beat_%x", beatHash[:12])
}

func ValidateDirectorPlan(p DirectorPlan, complete bool) error {
	if len(p.Beats) > 32 || utf8.RuneCountInString(p.Title) > 200 || utf8.RuneCountInString(p.Guidance) > 4000 {
		return fmt.Errorf("大纲最多 32 个阶段，标题最多 200 字，全局要求最多 4000 字")
	}
	if complete && (strings.TrimSpace(p.Title) == "" || len(p.Beats) == 0) {
		return fmt.Errorf("请填写大纲标题和至少一个阶段")
	}
	seen := map[string]bool{}
	for _, b := range p.Beats {
		if b.BeatID == "" || len(b.BeatID) > 100 || seen[b.BeatID] {
			return fmt.Errorf("阶段标识缺失或重复")
		}
		seen[b.BeatID] = true
		if utf8.RuneCountInString(b.Title) > 200 || utf8.RuneCountInString(b.Instruction) > 4000 || utf8.RuneCountInString(b.CompletionCriteria) > 2000 {
			return fmt.Errorf("阶段内容过长：名称最多 200 字、安排最多 4000 字、完成条件最多 2000 字")
		}
		if complete && (strings.TrimSpace(b.Title) == "" || strings.TrimSpace(b.Instruction) == "" || strings.TrimSpace(b.CompletionCriteria) == "") {
			return fmt.Errorf("每个阶段都需要名称、剧情安排和完成条件")
		}
	}
	return nil
}

// Evidence must be a literal, public story block. This deliberately errs on the
// conservative side for intentions; semantic completion is still model judged.
func ValidateDirectorEvidence(r DirectorReport, blocks []TextBlock) error {
	if r.Status != "completed" {
		return nil
	}
	if len(r.Evidence) == 0 || len(r.Evidence) > 8 {
		return fmt.Errorf("完成阶段需要正文依据")
	}
	for _, e := range r.Evidence {
		if e.BlockSeq < 1 || e.BlockSeq > len(blocks) || utf8.RuneCountInString(strings.TrimSpace(e.Quote)) < 6 {
			return fmt.Errorf("阶段完成依据无效")
		}
		b := blocks[e.BlockSeq-1]
		if (b.Kind != "narration" && b.Kind != "dialogue") || !strings.Contains(b.Text, e.Quote) {
			return fmt.Errorf("完成依据必须来自实际叙述或对白")
		}
		if !hasFactualDirectorQuote(b.Text, e.Quote) {
			return fmt.Errorf("意愿、假设或内心设想不能标记阶段完成")
		}
	}
	return nil
}

// Check the enclosing sentence as well as the selected quote so a fragment of
// "如果……" cannot evade the intention guard by leaving out its conditional.
func hasFactualDirectorQuote(block, quote string) bool {
	const boundaries = "。！？!?；;\n."
	for offset := 0; offset < len(block); {
		match := strings.Index(block[offset:], quote)
		if match < 0 {
			return false
		}
		start, end := offset+match, offset+match+len(quote)
		left := strings.LastIndexAny(block[:start], boundaries)
		if left < 0 {
			left = 0
		}
		right := strings.IndexAny(block[end:], boundaries)
		if right < 0 {
			right = len(block) - end
		}
		sentence := strings.ToLower(block[left : end+right])
		factual := true
		for _, word := range []string{"打算", "计划", "希望", "假如", "如果", "要是", "想要", "准备要", "将会", "可能会", "想象", "设想", "幻想", "梦见", "想道", "心想", "will ", "would ", "could ", "plans to", "wants to", "imagine", "dreamed"} {
			if strings.Contains(sentence, word) {
				factual = false
				break
			}
		}
		if factual {
			return true
		}
		offset = end
	}
	return false
}

// ApplyDirectorEvent is the single reducer for live writes and imported history.
// Its result is a deep copy; sibling branches never share mutable progress.
func ApplyDirectorEvent(base *DirectorState, ev *DomainEvent) (*DirectorState, error) {
	if ev.Type != EventDirectorChange {
		return base, nil
	}
	var c DirectorChange
	if err := json.Unmarshal([]byte(ev.PayloadJSON), &c); err != nil {
		return nil, err
	}
	var s *DirectorState
	if base != nil {
		raw, _ := json.Marshal(base)
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
	}
	if c.Action == "activate" {
		if c.Plan == nil {
			return nil, fmt.Errorf("缺少导演大纲")
		}
		if err := ValidateDirectorPlan(*c.Plan, true); err != nil {
			return nil, err
		}
		if c.Plan.RevisionID != ev.NodeID {
			return nil, fmt.Errorf("大纲版本必须指向启用节点")
		}
		if s == nil || c.Replace {
			if c.Plan.PlanID != ev.NodeID {
				return nil, fmt.Errorf("大纲标识无效")
			}
			s = &DirectorState{Plan: *c.Plan, Status: "active", CurrentBeatID: c.Plan.Beats[0].BeatID, Progress: map[string]DirectorBeatProgress{}}
		} else {
			if c.BaseRevisionID != s.Plan.RevisionID || c.Plan.PlanID != s.Plan.PlanID {
				return nil, fmt.Errorf("大纲已更新，请重新加载")
			}
			// Settled stages and the current stage's position/identity are retained.
			for i, old := range s.Plan.Beats {
				if _, done := s.Progress[old.BeatID]; done {
					if i >= len(c.Plan.Beats) || !reflect.DeepEqual(old, c.Plan.Beats[i]) {
						return nil, fmt.Errorf("已完成或跳过的阶段不可修改，请先回退进度或替换大纲")
					}
				} else if old.BeatID == s.CurrentBeatID {
					if i >= len(c.Plan.Beats) || c.Plan.Beats[i].BeatID != old.BeatID {
						return nil, fmt.Errorf("请保留当前阶段，使用跳过操作进入下一阶段")
					}
				}
			}
			s.Plan = *c.Plan
			if s.Status == "completed" {
				for _, b := range s.Plan.Beats {
					if _, ok := s.Progress[b.BeatID]; !ok {
						s.CurrentBeatID = b.BeatID
						break
					}
				}
			}
			s.Status = "active"
			if s.CurrentBeatID == "" {
				s.Status = "completed"
			}
			s.Warning = ""
		}
		return s, nil
	}
	if s == nil || c.BaseRevisionID != s.Plan.RevisionID {
		return nil, fmt.Errorf("活动大纲版本不匹配")
	}
	switch c.Action {
	case "pause":
		if s.Status != "active" {
			return nil, fmt.Errorf("只有执行中的大纲可以暂停")
		}
		s.Status = "paused"
	case "resume":
		if s.Status != "paused" {
			return nil, fmt.Errorf("只有暂停的大纲可以恢复")
		}
		s.Status = "active"
	case "rewind":
		if _, settled := s.Progress[c.BeatID]; !settled {
			return nil, fmt.Errorf("只能回退到已完成或跳过的阶段")
		}
		found := false
		for _, b := range s.Plan.Beats {
			if b.BeatID == c.BeatID {
				found = true
			}
			if found {
				delete(s.Progress, b.BeatID)
			}
		}
		if !found {
			return nil, fmt.Errorf("阶段不存在")
		}
		s.CurrentBeatID, s.Warning = c.BeatID, ""
		if s.Status == "completed" {
			s.Status = "active"
		}
	case "complete", "skip", "report":
		if s.CurrentBeat() == nil || (s.Status != "active" && c.Action == "report") {
			return nil, fmt.Errorf("当前没有执行中的阶段")
		}
		status := "completed"
		var evidence []DirectorEvidence
		if c.Action == "report" {
			r := c.Report
			if r == nil || r.RevisionID != s.Plan.RevisionID || r.BeatID != s.CurrentBeatID || len(r.Reason) > 6000 {
				return nil, fmt.Errorf("导演报告版本或阶段不匹配")
			}
			switch r.Status {
			case "continue", "blocked":
				s.Warning = r.Reason
				return s, nil
			case "completed":
				if len(r.Evidence) == 0 {
					return nil, fmt.Errorf("完成报告缺少依据")
				}
				evidence = r.Evidence
			default:
				return nil, fmt.Errorf("未知导演报告状态")
			}
		} else {
			if c.BeatID != s.CurrentBeatID {
				return nil, fmt.Errorf("只能完成或跳过当前阶段")
			}
			if c.Action == "skip" {
				status = "skipped"
			}
		}
		s.Progress[s.CurrentBeatID] = DirectorBeatProgress{Status: status, SourceNodeID: ev.NodeID, Evidence: evidence}
		s.CurrentBeatID, s.Warning = "", ""
		for _, b := range s.Plan.Beats {
			if _, done := s.Progress[b.BeatID]; !done {
				s.CurrentBeatID = b.BeatID
				break
			}
		}
		if s.CurrentBeatID == "" {
			s.Status = "completed"
		}
	default:
		return nil, fmt.Errorf("未知导演操作")
	}
	return s, nil
}
