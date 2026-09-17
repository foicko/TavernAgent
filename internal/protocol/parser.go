package protocol

import (
	"bytes"
	"errors"
	"strings"
	"unicode/utf8"
)

// 解析错误类别。
//
// 只保留真的会被返回的哨兵：此前的 ErrTruncated 与 ErrDuplicateSeq 从未被
// 返回（截断尾帧走 ValidationError，序号不连续也走 ValidationError），
// 却被应用层当成有效判断写进了 errors.Is 列表——"看着像有机制、其实没有"。
var (
	ErrMissingFinal = errors.New("structured turn missing final frame")
	ErrTooLarge     = errors.New("turn exceeds total byte budget")
	ErrAfterFinal   = errors.New("frames received after final")
)

// 解析模式（技术契约 §4）：结构化帧协议，或纯叙事兼容模式。
const (
	ModeStructured = "structured"
	ModeNarrative  = "narrative"
)

// StreamParser 增量解析逐行 JSON 帧协议 v1。
// 只发布已完整解析的帧；残片不是历史（技术契约 §4）。
//
// 兼容模式降级规则（契约 §4「纯叙事兼容模式」）：首个非空行即不是合法帧、
// 且此前尚未发布任何帧时，整条流降级为兼容模式——全量原文作为叙述正文，
// 提议与选项为空。降级是「整体」而非逐行，避免半结构化污染；
// 一旦已发布过帧再出现非法帧，仍按协议错误硬拒绝。
type StreamParser struct {
	buf            []byte
	raw            []byte // 全量输入（兼容模式重建正文用）
	total          int
	draft          TurnDraft
	lastSeq        int
	sawFinal       bool
	lineNo         int
	published      int // 已发布帧数（用于判定降级时机）
	mode           string
	tailErr        error // Finish 时被废弃的尾行解析错误（用于诊断）
	onFrame        func(frame any) error
	onDelta        func(seq int, kind BlockKind, speakerID *string, delta string) error
	curTextEmitted int
}

// NewStreamParser 构造增量解析器。
func NewStreamParser() *StreamParser {
	return &StreamParser{mode: ModeStructured}
}

// OnFrame 注册每帧发布后的回调（持久化或推送）。
func (p *StreamParser) OnFrame(sink func(frame any) error) {
	p.onFrame = sink
}

// OnDelta 注册行内实时正文增量回调（渐进式打字机流式）。
func (p *StreamParser) OnDelta(sink func(seq int, kind BlockKind, speakerID *string, delta string) error) {
	p.onDelta = sink
}

// Mode 返回当前解析模式：structured（结构化）或 narrative（纯叙事兼容）。
func (p *StreamParser) Mode() string {
	if p.mode == "" {
		return ModeStructured
	}
	return p.mode
}

// TailError 返回 Finish 阶段被废弃的尾行解析错误（如有）。
// 用于把「未收到 final」细化为可诊断的原因，例如字段名不合规。
func (p *StreamParser) TailError() error { return p.tailErr }

// Frames 返回已发布的块帧。
func (p *StreamParser) Blocks() []BlockFrame { return p.draft.Blocks }

// Draft 返回当前已解析产物（不含 final 时也允许预览块）。
func (p *StreamParser) Draft() TurnDraft { return p.draft }

// Feed 喂入任意 UTF-8 字节块；可跨 chunk 拼接行。
// 返回第一个校验错误；已发布帧保留在解析器中。
func (p *StreamParser) Feed(chunk []byte) error {
	p.total += len(chunk)
	if p.total > MaxTurnBytes {
		return ErrTooLarge
	}
	p.raw = append(p.raw, chunk...)
	if p.Mode() == ModeNarrative {
		// 兼容模式：不再逐行解析，正文直接作为增量输出，在 Finish 时统一成形。
		if p.onDelta != nil && len(chunk) > 0 {
			_ = p.onDelta(1, BlockNarration, nil, string(chunk))
		}
		return nil
	}
	p.buf = append(p.buf, chunk...)
	for {
		if p.Mode() == ModeNarrative {
			// 已降级：丢弃剩余行缓冲（原文已完整保留在 raw 中）。
			p.buf = p.buf[:0]
			return nil
		}
		idx := bytes.IndexByte(p.buf, '\n')
		if idx < 0 {
			// 未完成行：等待后续 chunk；用单帧上限防内存失控
			if len(p.buf) > MaxFrameBytes*8 {
				return ErrTooLarge
			}
			// 行内实时正文增量提取
			if p.onDelta != nil && len(p.buf) > 0 {
				if seq, kind, sp, text, _ := extractInFlightBlock(p.buf); len(text) > p.curTextEmitted {
					_ = p.onDelta(seq, kind, sp, text[p.curTextEmitted:])
					p.curTextEmitted = len(text)
				}
			}
			return nil
		}
		var line []byte
		if idx > 0 && p.buf[idx-1] == '\r' {
			line = p.buf[:idx-1]
		} else {
			line = p.buf[:idx]
		}
		p.buf = p.buf[idx+1:]
		p.lineNo++
		if len(bytes.TrimSpace(line)) == 0 {
			continue // 空行跳过（心跳/空帧无业务语义）
		}
		// 完整行到达：补齐该行所有未发出的增量字符，再重置行游标
		if p.onDelta != nil {
			if seq, kind, sp, text, _ := extractInFlightBlock(line); len(text) > p.curTextEmitted {
				_ = p.onDelta(seq, kind, sp, text[p.curTextEmitted:])
			}
		}
		p.curTextEmitted = 0

		if err := p.acceptLine(line); err != nil {
			return err
		}
	}
}

// extractInFlightBlock 提取未闭合 JSON 块帧中的行内正文与元数据。
func extractInFlightBlock(line []byte) (seq int, kind BlockKind, speakerID *string, text string, closed bool) {
	if !bytes.Contains(line, []byte(`"block"`)) {
		return 0, "", nil, "", false
	}

	if seqIdx := bytes.Index(line, []byte(`"seq":`)); seqIdx >= 0 {
		rest := line[seqIdx+6:]
		num := 0
		for _, b := range rest {
			if b >= '0' && b <= '9' {
				num = num*10 + int(b-'0')
			} else if num > 0 {
				break
			}
		}
		seq = num
	}

	if kindIdx := bytes.Index(line, []byte(`"kind":`)); kindIdx >= 0 {
		rest := bytes.TrimSpace(line[kindIdx+7:])
		if len(rest) > 0 && rest[0] == '"' {
			rest = rest[1:]
			if qIdx := bytes.IndexByte(rest, '"'); qIdx >= 0 {
				kind = BlockKind(rest[:qIdx])
			}
		}
	}
	if kind == "" {
		kind = BlockNarration
	}

	if spIdx := bytes.Index(line, []byte(`"speakerId":`)); spIdx >= 0 {
		rest := bytes.TrimSpace(line[spIdx+12:])
		if len(rest) > 0 && rest[0] == '"' {
			rest = rest[1:]
			if qIdx := bytes.IndexByte(rest, '"'); qIdx >= 0 {
				sp := string(rest[:qIdx])
				speakerID = &sp
			}
		}
	}

	textIdx := bytes.Index(line, []byte(`"text":`))
	if textIdx < 0 {
		return seq, kind, speakerID, "", false
	}

	valPart := line[textIdx+7:]
	valStart := bytes.IndexByte(valPart, '"')
	if valStart < 0 {
		return seq, kind, speakerID, "", false
	}

	strBytes := valPart[valStart+1:]
	var buf strings.Builder
	escaped := false

	for i := 0; i < len(strBytes); i++ {
		b := strBytes[i]
		if escaped {
			switch b {
			case '"':
				buf.WriteByte('"')
			case '\\':
				buf.WriteByte('\\')
			case '/':
				buf.WriteByte('/')
			case 'n':
				buf.WriteByte('\n')
			case 'r':
				buf.WriteByte('\r')
			case 't':
				buf.WriteByte('\t')
			default:
				buf.WriteByte('\\')
				buf.WriteByte(b)
			}
			escaped = false
			continue
		}

		if b == '\\' {
			escaped = true
			continue
		}

		if b == '"' {
			return seq, kind, speakerID, buf.String(), true
		}

		buf.WriteByte(b)
	}

	res := buf.String()
	for len(res) > 0 && !utf8.ValidString(res) {
		res = res[:len(res)-1]
	}

	return seq, kind, speakerID, res, false
}

// InFlight 返回当前未闭合块帧的实时正文（可能为空）。
//
// 用途只有一个：断线或刷新后的客户端补齐"正在写的这一段"。已完成的块走
// durable 事件回放，行内增量是 Sequence 0 的临时事件、不会重放，
// 因此缺字只会发生在当前块，而这里正是它的权威文本。
func (p *StreamParser) InFlight() (seq int, kind BlockKind, speakerID *string, text string, ok bool) {
	if len(p.buf) == 0 {
		return 0, "", nil, "", false
	}
	seq, kind, speakerID, text, _ = extractInFlightBlock(p.buf)
	if text == "" {
		return 0, "", nil, "", false
	}
	return seq, kind, speakerID, text, true
}

// Finish 结束流：结构化模式必须包含 final，且丢弃未完成尾行；
// 兼容模式把全量原文成形为正文块，不要求 final（契约 §4）。
// 返回去掉未完成尾行后的总字节与解析错误。
func (p *StreamParser) Finish() (TurnDraft, error) {
	if p.total <= 0 {
		return TurnDraft{}, ErrMissingFinal
	}
	// 补解析未以换行结尾的尾行：供应商的最后一个分片通常不带 \n，
	// 若直接丢弃会把合法的 final 帧当成残片（表现为「已输出正文但未收到 final」）。
	// 契约要求丢弃的是「未完成尾帧」，完整帧必须先尝试解析。
	// 解析失败时保持沉默——该尾行按未完成尾帧废弃，由下方分支决定终态。
	if tail := bytes.TrimSpace(p.buf); len(tail) > 0 && !p.sawFinal {
		if bytes.HasPrefix(tail, []byte("```")) {
			// 尾部 markdown 围栏直接忽略
		} else {
			if p.onDelta != nil {
				if seq, kind, sp, text, _ := extractInFlightBlock(tail); len(text) > p.curTextEmitted {
					_ = p.onDelta(seq, kind, sp, text[p.curTextEmitted:])
				}
			}
			p.curTextEmitted = 0
			if err := p.acceptLine(tail); err != nil {
				p.tailErr = err
			}
		}
	}
	p.buf = p.buf[:0]

	if p.Mode() == ModeNarrative {
		return p.narrativeDraft()
	}
	if !p.sawFinal {
		// 未发布任何帧：整条流可能就是一段没有结尾换行的纯文本（此时永远不会走到
		// acceptLine，无法在 Feed 阶段降级）。用与 acceptLine 相同的判据处理：
		// 不以 "{" 开头视为纯叙事，"{" 开头视为非法帧尝试仍拒绝。
		if p.published == 0 {
			text := strings.TrimSpace(string(p.raw))
			cleaned := strings.TrimPrefix(text, "```json")
			cleaned = strings.TrimPrefix(cleaned, "```")
			cleaned = strings.TrimSpace(cleaned)
			if cleaned != "" && !strings.HasPrefix(cleaned, "{") {
				p.mode = ModeNarrative
				return p.narrativeDraft()
			}
		}
		return p.draft, ErrMissingFinal
	}
	return p.draft, nil
}

// narrativeDraft 把已累积的原文成形为兼容模式草稿：叙述块 + 空提议 + 空选项。
func (p *StreamParser) narrativeDraft() (TurnDraft, error) {
	text := strings.TrimSpace(string(p.raw))
	if text == "" {
		return TurnDraft{}, ErrMissingFinal
	}
	blocks := make([]BlockFrame, 0, 1)
	for i, seg := range splitNarration(text) {
		blocks = append(blocks, BlockFrame{
			V: Version, Seq: i + 1, Type: FrameBlock,
			Kind: BlockNarration, Text: seg,
		})
	}
	p.draft = TurnDraft{Blocks: blocks, Proposals: []Proposal{}, Options: []OptionFrame{}}
	return p.draft, nil
}

// splitNarration 把兼容模式原文切成不超过 MaxBlockRunes 的块，
// 优先在换行处断开，保持段落完整。
func splitNarration(text string) []string {
	var out []string
	for _, para := range strings.Split(text, "\n") {
		if strings.TrimSpace(para) == "" {
			if len(out) > 0 {
				out[len(out)-1] += "\n" // 保留段落间空行，交给渲染层处理
			}
			continue
		}
		r := []rune(para)
		if len(r) <= MaxBlockRunes {
			out = append(out, para)
			continue
		}
		for len(r) > MaxBlockRunes {
			out = append(out, string(r[:MaxBlockRunes]))
			r = r[MaxBlockRunes:]
		}
		if len(r) > 0 {
			out = append(out, string(r))
		}
	}
	if len(out) == 0 {
		out = []string{strings.TrimSpace(text)}
	}
	return out
}

func (p *StreamParser) acceptLine(line []byte) error {
	trimmed := bytes.TrimSpace(line)
	if bytes.HasPrefix(trimmed, []byte("```")) {
		// 跳过代码围栏（``` 或 ```json），不计入序列也不报错
		return nil
	}
	if p.sawFinal {
		return ErrAfterFinal
	}
	has, frame, err := parseFrame(line)
	if err != nil {
		// 兼容模式降级：首个非空行是普通文本（不以 "{" 开头，不是帧尝试）且尚未发布任何帧
		// → 整条流按纯叙事处理。
		// 以 "{" 开头说明模型在尝试输出帧，只是格式非法：仍按协议错误硬拒绝，
		// 避免用宽松解析掩盖残缺操作（契约 §4）。
		if p.published == 0 && !bytes.HasPrefix(trimmed, []byte("{")) {
			p.mode = ModeNarrative
			return nil
		}
		return err
	}
	if !has {
		return Invalid("empty frame line", 0)
	}
	switch f := frame.(type) {
	case BlockFrame:
		if f.Seq != p.lastSeq+1 {
			return Invalid("non-contiguous block seq", f.Seq)
		}
		p.lastSeq = f.Seq
		p.draft.Blocks = append(p.draft.Blocks, f)
	case FinalFrame:
		if f.Seq != p.lastSeq+1 {
			return Invalid("non-contiguous final seq", f.Seq)
		}
		p.lastSeq = f.Seq
		p.draft.Proposals = f.Proposals
		p.draft.Options = f.Options
		p.draft.Director = f.Director
		p.sawFinal = true
	default:
		return Invalid("unknown frame kind", 0)
	}
	p.published++
	if p.onFrame != nil {
		return p.onFrame(frame)
	}
	return nil
}
