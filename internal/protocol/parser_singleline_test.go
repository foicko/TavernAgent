package protocol

import (
	"testing"
)

// 复现：模型把整段输出挤在一行（无换行）时，当前解析器会把它当成单个 JSON
// 对象去解析——json.Unmarshal 因尾随数据报错，尾行错误被吞掉，最终表现为
// ErrMissingFinal（应用层显示「结构化回合缺少 final 帧」，且不可重试）。
//
// 真实数据佐证：用户库里 3 条 PROTOCOL_INVALID 失败回合，provider=openai-responses、
// model=deepseek-flash，completion_tokens 719~1076，但 draft_frames 为 0——
// 即模型确实产出了内容，却没有一个完整帧被解析出来。
func TestWholeTurnOnSingleLineIsAccepted(t *testing.T) {
	p := NewStreamParser()
	// 注意：两帧之间没有换行。
	body := mkBlock(1, BlockNarration, "门轴发出一声干涩的声响。") + mkFinal(2, "", "")

	if err := p.Feed([]byte(body)); err != nil {
		t.Fatalf("feed: %v", err)
	}
	draft, err := p.Finish()
	if err != nil {
		t.Fatalf("finish: %v (tailErr=%v)", err, p.TailError())
	}
	if len(draft.Blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(draft.Blocks))
	}
	if draft.Blocks[0].Text != "门轴发出一声干涩的声响。" {
		t.Fatalf("text = %q", draft.Blocks[0].Text)
	}
	if p.Mode() != ModeStructured {
		t.Fatalf("mode = %q, want structured", p.Mode())
	}
}

// 首帧带换行、后续多帧挤在末行（无换行）时也要能完整收尾。
func TestMultipleFramesOnUnterminatedLastLine(t *testing.T) {
	p := NewStreamParser()
	body := mkBlock(1, BlockNarration, "a") + "\n" +
		mkBlock(2, BlockDialogue, "b") + mkFinal(3, "", "")

	if err := p.Feed([]byte(body)); err != nil {
		t.Fatalf("feed: %v", err)
	}
	draft, err := p.Finish()
	if err != nil {
		t.Fatalf("finish: %v (tailErr=%v)", err, p.TailError())
	}
	if len(draft.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(draft.Blocks))
	}
}
