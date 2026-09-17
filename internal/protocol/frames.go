// Package protocol 实现模型输出协议 v1：逐行 JSON 帧的增量解析与合法性校验。
// 供应商的 SSE 格式不等于应用的 SSE 格式，二者由后端隔离（技术契约 §4）。
package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Protocol 常量（技术契约 §4）。
const (
	Version = 1

	MaxFrameBytes = 64 * 1024       // 单帧 64 KiB
	MaxTurnBytes  = 2 * 1024 * 1024 // 单次尝试正文与元数据总量 2 MiB
	MaxJSONDepth  = 16
	MaxBlockRunes = 2048 // 单块最大 2048 个 Unicode 字符
	MaxOptions    = 4
)

type FrameType string

const (
	FrameBlock FrameType = "block"
	FrameFinal FrameType = "final"
)

type BlockKind string

const (
	BlockNarration      BlockKind = "narration"
	BlockDialogue       BlockKind = "dialogue"
	BlockInnerMonologue BlockKind = "inner_monologue"
)

// 各种合法性校验失败的错误类别（应用层映射到 PROTOCOL_INVALID）。
type ValidationError struct {
	Reason string
	Seq    int
}

func (e *ValidationError) Error() string {
	if e.Seq > 0 {
		return fmt.Sprintf("protocol invalid at seq %d: %s", e.Seq, e.Reason)
	}
	return "protocol invalid: " + e.Reason
}

func Invalid(reason string, seq int) error {
	return &ValidationError{Reason: reason, Seq: seq}
}

// BlockFrame 是正文块帧。
type BlockFrame struct {
	V         int       `json:"v"`
	Seq       int       `json:"seq"`
	Type      FrameType `json:"type"`
	Kind      BlockKind `json:"kind"`
	SpeakerID *string   `json:"speakerId"`
	Text      string    `json:"text"`
}

// Proposal 是 final 帧中的操作提议。
type Proposal struct {
	ProposalID         string   `json:"proposalId"`
	Type               string   `json:"type"`
	CharacterID        string   `json:"characterId,omitempty"`
	Field              string   `json:"field,omitempty"`
	Delta              int      `json:"delta,omitempty"`
	EvidenceBlockSeqs  []int    `json:"evidenceBlockSeqs,omitempty"`
	ItemID             string   `json:"itemId,omitempty"`
	ActionRef          string   `json:"actionRef,omitempty"`
	From               string   `json:"from,omitempty"`
	To                 string   `json:"to,omitempty"`
	Quantity           int      `json:"quantity,omitempty"`
	Text               string   `json:"text,omitempty"` // memory/promise 内容等
	Participants       []string `json:"participants,omitempty"`
	MoodCode           string   `json:"moodCode,omitempty"`
	Confidence         string   `json:"confidence,omitempty"` // observed|reported|inferred
	MemoryKind         string   `json:"memoryKind,omitempty"`
	SourceQuote        string   `json:"sourceQuote,omitempty"`
	Reasoning          string   `json:"reasoning,omitempty"`
	EvidenceConfidence string   `json:"evidenceConfidence,omitempty"`
	EntityIDs          []string `json:"entityIds,omitempty"`
	RuleID             string   `json:"ruleId,omitempty"`
	ReceiptRef         string   `json:"receiptRef,omitempty"`
	SubjectKey         string   `json:"subjectKey,omitempty"`
}

// OptionFrame 是 final 帧中的候选选项。
type OptionFrame struct {
	OptionID  string `json:"optionId"`
	Intent    string `json:"intent"`
	Text      string `json:"text"`
	ActionRef string `json:"actionRef,omitempty"`
}

// FinalFrame 是最后一帧，携带操作提议与候选选项。
type FinalFrame struct {
	Director  json.RawMessage `json:"director,omitempty"`
	V         int             `json:"v"`
	Seq       int             `json:"seq"`
	Type      FrameType       `json:"type"`
	Proposals []Proposal      `json:"proposals"`
	Options   []OptionFrame   `json:"options"`
}

// TurnDraft 是一次尝试的完整产物。
type TurnDraft struct {
	Director  json.RawMessage
	Blocks    []BlockFrame
	Proposals []Proposal
	Options   []OptionFrame
}

// ---- 帧解码与校验收敛 ----

// parseFrame 解析一行内嵌的帧对象；返回 (存在帧, 帧, 错误)。
// 任何校验失败都使本行非法。
func parseFrame(line []byte) (bool, any, error) {
	var raw struct {
		V    int    `json:"v"`
		Seq  int    `json:"seq"`
		Type string `json:"type"`
	}
	// 先做严格 JSON 结构校验（深度、重复键、类型安全）。
	if err := strictDecode(line, &raw); err != nil {
		return false, nil, err
	}
	if raw.V != Version {
		return false, nil, Invalid(fmt.Sprintf("unsupported protocol version %d", raw.V), raw.Seq)
	}
	if raw.Seq < 1 {
		return false, nil, Invalid("non-positive seq", raw.Seq)
	}
	switch FrameType(raw.Type) {
	case FrameBlock:
		return decodeBlockFrame(line, raw.Seq, "")
	case FrameFinal:
		var f FinalFrame
		if err := strictDecode(line, &f); err != nil {
			return false, nil, err
		}
		if len(f.Options) > MaxOptions {
			return false, nil, Invalid(fmt.Sprintf("options exceed %d", MaxOptions), f.Seq)
		}
		seen := map[string]bool{}
		for _, o := range f.Options {
			switch o.Intent {
			case "aggressive", "clever", "emotional", "chaotic":
			default:
				return false, nil, Invalid(fmt.Sprintf("unknown intent %q", o.Intent), f.Seq)
			}
			if o.OptionID == "" {
				return false, nil, Invalid("option missing optionId", f.Seq)
			}
			if seen[o.OptionID] {
				return false, nil, Invalid(fmt.Sprintf("duplicate optionId %q", o.OptionID), f.Seq)
			}
			seen[o.OptionID] = true
		}
		propSeen := map[string]bool{}
		for _, p := range f.Proposals {
			if p.ProposalID == "" {
				return false, nil, Invalid("proposal missing proposalId", f.Seq)
			}
			if propSeen[p.ProposalID] {
				return false, nil, Invalid(fmt.Sprintf("duplicate proposalId %q", p.ProposalID), f.Seq)
			}
			propSeen[p.ProposalID] = true
		}
		return true, f, nil
	default:
		// 兼容写法：模型偶尔把块类型直接写进 type（"type":"dialogue" / "inner_monologue"），
		// kind 位置反而缺失。这三个值都是本协议已定义的块类型，语义无歧义，按 block 收下；
		// 其它未知 type 仍然拒绝。
		if kind := BlockKind(raw.Type); isBlockKind(kind) {
			return decodeBlockFrame(line, raw.Seq, kind)
		}
		return false, nil, Invalid(fmt.Sprintf("unknown frame type %q", raw.Type), raw.Seq)
	}
}

func isBlockKind(kind BlockKind) bool {
	switch kind {
	case BlockNarration, BlockDialogue, BlockInnerMonologue:
		return true
	}
	return false
}

// decodeBlockFrame 校验块帧。fallbackKind 非空表示调用方已从 type 位置识别出块类型。
func decodeBlockFrame(line []byte, seq int, fallbackKind BlockKind) (bool, any, error) {
	var f BlockFrame
	if err := strictDecode(line, &f); err != nil {
		return false, nil, err
	}
	if len([]rune(f.Text)) > MaxBlockRunes {
		return false, nil, Invalid("block text exceeds 2048 runes", f.Seq)
	}
	if f.Kind == "" {
		f.Kind = fallbackKind
	}
	if !isBlockKind(f.Kind) {
		return false, nil, Invalid(fmt.Sprintf("unknown block kind %q", f.Kind), f.Seq)
	}
	return true, f, nil
}

// strictDecode 解码并同时校验 JSON 嵌套深度与重复键。
//
// 两处校验都必须是 ValidationError：应用层按错误身份区分"模型输出不合协议"
// （PROTOCOL_INVALID，重试整轮没有意义）与"供应商故障"（PROVIDER_UNAVAILABLE，
// 值得重试）。此前 json.Unmarshal 的语法错误是裸 *json.SyntaxError——
// 同一行里写两个 JSON 对象会被当成供应商故障，白花一次整轮重试。
func strictDecode(data []byte, dst any) error {
	if err := validateJSON(data); err != nil {
		return err
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return Invalid("malformed JSON frame: "+err.Error(), 0)
	}
	return nil
}

// validateJSON 使用 Token 遍历检查深度 ≤ MaxJSONDepth 且无重复对象键。
func validateJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	type objState struct {
		keys      map[string]bool
		expectKey bool
	}
	var stack []objState
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil // 正常结束
		}
		if err != nil {
			return Invalid("malformed JSON frame", 0)
		}
		switch tv := tok.(type) {
		case json.Delim:
			switch tv {
			case '{':
				if len(stack) >= MaxJSONDepth {
					return Invalid(fmt.Sprintf("JSON depth exceeds %d", MaxJSONDepth), 0)
				}
				stack = append(stack, objState{keys: map[string]bool{}, expectKey: true})
			case '[':
				if len(stack) >= MaxJSONDepth {
					return Invalid(fmt.Sprintf("JSON depth exceeds %d", MaxJSONDepth), 0)
				}
				stack = append(stack, objState{})
			case '}', ']':
				if len(stack) == 0 {
					return Invalid("unbalanced JSON delimiters", 0)
				}
				stack = stack[:len(stack)-1]
			}
		case string:
			sv := string(tv)
			if len(stack) == 0 {
				break
			}
			top := &stack[len(stack)-1]
			if top.expectKey && top.keys != nil {
				if top.keys[sv] {
					return Invalid(fmt.Sprintf("duplicate JSON key %q", sv), 0)
				}
				top.keys[sv] = true
				top.expectKey = false
			} else {
				if top.keys != nil {
					top.expectKey = true // 对象内值之后的下一个字符串是键
				}
			}
		default:
			if len(stack) > 0 {
				top := &stack[len(stack)-1]
				if top.keys != nil {
					top.expectKey = true
				}
			}
		}
	}
}

// DecodeStrictJSON also rejects unknown fields for bounded background protocols.
func DecodeStrictJSON(data []byte, dst any) error {
	if len(data) > MaxFrameBytes {
		return Invalid("JSON payload too large", 0)
	}
	if err := validateJSON(data); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return Invalid("multiple JSON values", 0)
	}
	return nil
}
