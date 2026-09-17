package application

import "tavernagent/internal/protocol"

// liveStream 是在途回合的解析器引用：它的缓冲区就是当前块尚未收尾的文本。
type liveStream struct {
	attemptID string
	parser    *protocol.StreamParser
}

// PartialBlock 是当前未完成块的实时快照。
type PartialBlock struct {
	Seq       int
	Kind      string
	SpeakerID *string
	Text      string
}

// PartialTurn 是"到目前为止"的正文快照：已落盘帧由 durable 事件回放，
// 这里只负责当前块——它的行内增量是 Sequence 0 的临时事件，断线后永不重放。
type PartialTurn struct {
	AttemptID string
	InFlight  *PartialBlock
}

func (s *TurnService) registerLive(turnID, attemptID string, parser *protocol.StreamParser) func() {
	stream := &liveStream{attemptID: attemptID, parser: parser}
	s.liveMu.Lock()
	if s.live == nil {
		s.live = map[string]*liveStream{}
	}
	s.live[turnID] = stream
	s.liveMu.Unlock()
	return func() {
		s.liveMu.Lock()
		if current := s.live[turnID]; current == stream {
			delete(s.live, turnID)
		}
		s.liveMu.Unlock()
	}
}

// PartialDraft 返回在途回合的实时正文；回合不在途（未开始/已结束/换了解析器）时 ok=false。
func (s *TurnService) PartialDraft(turnID string) (*PartialTurn, bool) {
	s.liveMu.Lock()
	stream := s.live[turnID]
	s.liveMu.Unlock()
	if stream == nil || stream.parser == nil {
		return nil, false
	}
	draft := &PartialTurn{AttemptID: stream.attemptID}
	if seq, kind, speaker, text, ok := stream.parser.InFlight(); ok {
		draft.InFlight = &PartialBlock{Seq: seq, Kind: string(kind), SpeakerID: speaker, Text: text}
	}
	return draft, true
}
