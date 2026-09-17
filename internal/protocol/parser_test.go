package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func mkBlock(seq int, kind BlockKind, text string) string {
	txt, _ := json.Marshal(text)
	return fmt.Sprintf(`{"v":1,"seq":%d,"type":"block","kind":"%s","text":%s}`, seq, kind, txt)
}

func mkFinal(seq int, proposals, opts string) string {
	return fmt.Sprintf(`{"v":1,"seq":%d,"type":"final","proposals":[%s],"options":[%s]}`, seq, proposals, opts)
}

// T03：UTF-8、JSON 转义与标签文本在任意 chunk 处切断 → 完整帧正确还原。
func TestFeedSplitsChunksArbitrarily(t *testing.T) {
	text := "雨水沿着酒馆的窗框流下。\n她说：“我来自北方”\n\t<secret>标签</secret> & <script>不应执行</script>"
	line1 := mkBlock(1, BlockNarration, text)
	line2 := mkFinal(2, "", "")
	full := line1 + "\n" + line2 + "\n"

	p := NewStreamParser()
	// 每个 rune 边界切一刀（中文 >1 字节，强制覆盖中间切分）
	runes := []rune(full)
	for i := 0; i < len(runes); i++ {
		seg := string(runes[i : i+1])
		if err := p.Feed([]byte(seg)); err != nil {
			t.Fatalf("feed at rune %d: %v", i, err)
		}
	}
	draft, err := p.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(draft.Blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(draft.Blocks))
	}
	if draft.Blocks[0].Text != text {
		t.Fatalf("text mismatch:\n got %q\nwant %q", draft.Blocks[0].Text, text)
	}
}

// T04a 缺 final → 结构化回合不提交。
func TestMissingFinalRejected(t *testing.T) {
	p := NewStreamParser()
	if err := p.Feed([]byte(mkBlock(1, BlockNarration, "前半段") + "\n")); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if _, err := p.Finish(); err == nil {
		t.Fatalf("expected ErrMissingFinal, got nil")
	}
}

// T04b 跳号 → 拒绝。
func TestNonContiguousSeqRejected(t *testing.T) {
	p := NewStreamParser()
	if err := p.Feed([]byte(mkBlock(1, BlockNarration, "a") + "\n" + mkBlock(3, BlockDialogue, "b") + "\n")); err == nil {
		t.Fatalf("expected seq gap rejection, got nil")
	}
}

// T04c 重复 JSON 键 → 拒绝。
func TestDuplicateJSONKeysRejected(t *testing.T) {
	p := NewStreamParser()
	line := `{"v":1,"seq":1,"type":"block","kind":"narration","text":"a","text":"b"}`
	if err := p.Feed([]byte(line + "\n")); err == nil {
		t.Fatalf("expected duplicate key rejection, got nil")
	}
}

// T04d 非法帧（未知 frame type / kind / intent / 版本）→ 拒绝。
func TestInvalidFramesRejected(t *testing.T) {
	cases := []string{
		mkBlock(1, BlockNarration, "a") + `{"v":1,"seq":2,"type":"weird"}`,
		`{"v":1,"seq":1,"type":"block","kind":"theater","text":"x"}`,
		`{"v":1,"seq":1,"type":"final","proposals":[],"options":[{"optionId":"o1","intent":"sneaky","text":"t"}]}`,
		`{"v":2,"seq":1,"type":"block","kind":"narration","text":"x"}`,
	}
	for i, c := range cases {
		p := NewStreamParser()
		if err := p.Feed([]byte(c + "\n")); err == nil {
			t.Fatalf("case %d: expected rejection, got nil", i)
		}
	}
}

// T04e 深度越界 → 拒绝。
func TestDepthLimitRejected(t *testing.T) {
	deep := `{"v":1,"seq":1,"type":"block","kind":"narration","text":"a","extra":{"a":{"a":{"a":{"a":{"a":{"a":{"a":{"a":{"a":{"a":{"a":{"a":{"a":{"a":{"a":{}}}}}}}}}}}}}}}}}`
	p := NewStreamParser()
	if err := p.Feed([]byte(deep + "\n")); err == nil {
		t.Fatalf("expected depth rejection, got nil")
	}
}

// T05 无心声、空选项 → final 正常（解析层接受空 options/proposals，界面可收起）。
func TestFinalWithEmptyArraysOK(t *testing.T) {
	p := NewStreamParser()
	body := mkBlock(1, BlockNarration, "完成。") + "\n" + mkFinal(2, "", "") + "\n"
	if err := p.Feed([]byte(body)); err != nil {
		t.Fatalf("feed: %v", err)
	}
	draft, err := p.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(draft.Options) != 0 || len(draft.Proposals) != 0 {
		t.Fatalf("expected empty options/proposals")
	}
}

// 选项超过 4 个 → 拒绝。
func TestMaxOptionsRejected(t *testing.T) {
	var opts []string
	for i := 0; i < 5; i++ {
		opts = append(opts, `{"optionId":"o`+strconv.Itoa(i)+`","intent":"clever","text":"t"}`)
	}
	line := fmt.Sprintf(`{"v":1,"seq":1,"type":"final","proposals":[],"options":[%s]}`, strings.Join(opts, ","))
	p := NewStreamParser()
	if err := p.Feed([]byte(line + "\n")); err == nil {
		t.Fatalf("expected >4 options rejection, got nil")
	}
}

// T06：本地小模型以纯文本兼容模式回复 → 保留阅读体验，零状态变化且不猜测硬操作。
func TestNarrativeCompatModeOnPlainText(t *testing.T) {
	prose := "雨水沿着酒馆的窗框流下。\n「你终于回来了。」她说。\n壁炉的火光在墙上摇曳。"
	p := NewStreamParser()
	for _, r := range prose { // 逐 rune 喂入，覆盖任意 chunk 切分
		if err := p.Feed([]byte(string(r))); err != nil {
			t.Fatalf("feed: %v", err)
		}
	}
	if p.Mode() != ModeNarrative {
		t.Fatalf("mode = %q, want %q", p.Mode(), ModeNarrative)
	}
	draft, err := p.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(draft.Blocks) == 0 {
		t.Fatalf("expected narration blocks, got none")
	}
	// 正文必须完整还原（段落换行允许保留）。
	got := strings.Join(blockTexts(draft.Blocks), "\n")
	if !strings.Contains(got, "你终于回来了") || !strings.Contains(got, "壁炉的火光") {
		t.Fatalf("narrative text lost: %q", got)
	}
	for _, b := range draft.Blocks {
		if b.Kind != BlockNarration {
			t.Fatalf("compat mode block kind = %q, want narration", b.Kind)
		}
	}
	// 零状态变化：兼容模式不得凭空产生提议或选项。
	if len(draft.Proposals) != 0 || len(draft.Options) != 0 {
		t.Fatalf("compat mode must not guess operations: proposals=%d options=%d",
			len(draft.Proposals), len(draft.Options))
	}
}

// T06b：兼容模式下超长正文仍受单块上限约束。
func TestNarrativeCompatSplitsLongText(t *testing.T) {
	long := strings.Repeat("字", MaxBlockRunes*2+10)
	p := NewStreamParser()
	if err := p.Feed([]byte(long + "\n")); err != nil {
		t.Fatalf("feed: %v", err)
	}
	draft, err := p.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(draft.Blocks) != 3 {
		t.Fatalf("blocks = %d, want 3", len(draft.Blocks))
	}
	for i, b := range draft.Blocks {
		if len([]rune(b.Text)) > MaxBlockRunes {
			t.Fatalf("block %d exceeds MaxBlockRunes", i)
		}
		if b.Seq != i+1 {
			t.Fatalf("block %d seq = %d, want %d", i, b.Seq, i+1)
		}
	}
}

// T06c：已发布过合法帧之后再出现非法行 → 不得降级，仍按协议错误拒绝（防半结构化污染）。
func TestNoDegradeAfterPublishedFrame(t *testing.T) {
	p := NewStreamParser()
	body := mkBlock(1, BlockNarration, "第一段") + "\n" + "这行是模型跑偏的散文\n"
	if err := p.Feed([]byte(body)); err == nil {
		t.Fatalf("expected rejection after published frame, got nil")
	}
	if p.Mode() != ModeStructured {
		t.Fatalf("mode = %q, want %q", p.Mode(), ModeStructured)
	}
}

// T06d：以 "{" 开头的非法帧是「帧尝试失败」，不得降级为兼容模式。
func TestNoDegradeOnMalformedFrameAttempt(t *testing.T) {
	p := NewStreamParser()
	if err := p.Feed([]byte(`{"v":1,"seq":1,"type":"block"` + "\n")); err == nil {
		t.Fatalf("expected rejection for malformed frame attempt, got nil")
	}
	if p.Mode() != ModeStructured {
		t.Fatalf("mode = %q, want %q", p.Mode(), ModeStructured)
	}
}

func blockTexts(blocks []BlockFrame) []string {
	out := make([]string, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, b.Text)
	}
	return out
}

// T06e：纯文本不带结尾换行（最后一行永远不完整）→ Finish 阶段仍应降级为兼容模式。
func TestNarrativeCompatWithoutTrailingNewline(t *testing.T) {
	p := NewStreamParser()
	if err := p.Feed([]byte("门轴发出一声干涩的声响。")); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if p.Mode() != ModeStructured {
		t.Fatalf("Feed 阶段不应降级")
	}
	draft, err := p.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if p.Mode() != ModeNarrative {
		t.Fatalf("mode = %q, want %q", p.Mode(), ModeNarrative)
	}
	if len(draft.Blocks) != 1 || draft.Blocks[0].Text != "门轴发出一声干涩的声响。" {
		t.Fatalf("blocks = %+v", draft.Blocks)
	}
}

// T06f：不带换行的残缺帧（以 "{" 开头）→ 仍按协议错误拒绝，不得当成散文。
func TestTruncatedFrameAttemptNotDegraded(t *testing.T) {
	p := NewStreamParser()
	if err := p.Feed([]byte(`{"v":1,"seq":1,"type":"block","kind":"narr`)); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if _, err := p.Finish(); err == nil {
		t.Fatalf("expected ErrMissingFinal for truncated frame attempt, got nil")
	}
	if p.Mode() != ModeStructured {
		t.Fatalf("mode = %q, want %q", p.Mode(), ModeStructured)
	}
}

// 回归：供应商最后一个分片不带结尾换行时，final 帧不能被当作残片丢弃。
// 缺陷表现：回合停在 awaiting_continuation（「已输出正文但未收到 final」）。
func TestFinalWithoutTrailingNewline(t *testing.T) {
	p := NewStreamParser()
	body := mkBlock(1, BlockNarration, "门轴发出轻响。") + "\n" + mkFinal(2, "", "")
	if err := p.Feed([]byte(body)); err != nil { // 注意：结尾无 \n
		t.Fatalf("feed: %v", err)
	}
	draft, err := p.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(draft.Blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(draft.Blocks))
	}
	if draft.Blocks[0].Text != "门轴发出轻响。" {
		t.Fatalf("text = %q", draft.Blocks[0].Text)
	}
	if p.Mode() != ModeStructured {
		t.Fatalf("mode = %q, want structured", p.Mode())
	}
}

// 回归：按 SSE 分片喂入（首行带换行、末行不带）时 final 仍能收尾。
func TestFinalArrivesAsLastUnterminatedChunk(t *testing.T) {
	p := NewStreamParser()
	chunks := []string{
		mkBlock(1, BlockNarration, "a") + "\n",
		mkBlock(2, BlockDialogue, "b") + "\n",
		mkFinal(3, "", ""), // 末行无换行
	}
	for _, c := range chunks {
		if err := p.Feed([]byte(c)); err != nil {
			t.Fatalf("feed %q: %v", c, err)
		}
	}
	draft, err := p.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(draft.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(draft.Blocks))
	}
}

// 回归：末行是真正的残片（流在中途断开）→ 仍然报缺 final，不误判为完整。
func TestTruncatedTailStillMissingFinal(t *testing.T) {
	p := NewStreamParser()
	body := mkBlock(1, BlockNarration, "前半段") + "\n" + `{"v":1,"seq":2,"type":"fin`
	if err := p.Feed([]byte(body)); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if _, err := p.Finish(); err == nil {
		t.Fatalf("expected ErrMissingFinal, got nil")
	}
}

// 渐进式流式：行内字符实时触发 onDelta，最终累积与完整帧一致。
func TestStreamParserDeltaProgressive(t *testing.T) {
	p := NewStreamParser()
	var deltas []string
	var kinds []BlockKind
	p.OnDelta(func(seq int, kind BlockKind, speakerID *string, delta string) error {
		deltas = append(deltas, delta)
		kinds = append(kinds, kind)
		return nil
	})

	chunks := []string{
		`{"v":1,"seq":1,"type":"block","kind":"narration","speakerId":null,"text":"在昏`,
		`暗的烛`,
		`光下，艾`,
		"莲娜微笑着说：\\\"日落了。\\\"",
		"\"}\n",
		mkFinal(2, "", "") + "\n",
	}

	for _, c := range chunks {
		if err := p.Feed([]byte(c)); err != nil {
			t.Fatalf("feed %q: %v", c, err)
		}
	}

	draft, err := p.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(draft.Blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(draft.Blocks))
	}

	accumulated := strings.Join(deltas, "")
	if accumulated != draft.Blocks[0].Text {
		t.Fatalf("delta accumulated %q != block text %q", accumulated, draft.Blocks[0].Text)
	}
	if len(deltas) < 4 {
		t.Fatalf("expected multiple delta callbacks, got %d", len(deltas))
	}
}

// 验证模型在前后输出 markdown 代码围栏（```json 与 ```）时不发生降级且正常解析结构化帧。
func TestMarkdownCodeFenceIgnored(t *testing.T) {
	p := NewStreamParser()
	raw := "```json\n" +
		mkBlock(1, BlockNarration, "雨夜中的酒馆很温暖。") + "\n" +
		mkFinal(2, "", "") + "\n" +
		"```\n"

	if err := p.Feed([]byte(raw)); err != nil {
		t.Fatalf("feed markdown fenced frames: %v", err)
	}
	draft, err := p.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if p.Mode() != ModeStructured {
		t.Fatalf("expected ModeStructured, got %s", p.Mode())
	}
	if len(draft.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(draft.Blocks))
	}
	if draft.Blocks[0].Text != "雨夜中的酒馆很温暖。" {
		t.Fatalf("text mismatch: %q", draft.Blocks[0].Text)
	}
}

// 真实模型偶尔把块类型直接写进 type（"type":"dialogue"），kind 位置缺失。
// 这三个值都是本协议已定义的块类型，语义无歧义；不兼容会让整个回合失败
// （实测 DeepSeek 约 7% 的回合踩到，用户看到"生成受阻"）。
func TestBlockKindInTypeFieldIsAccepted(t *testing.T) {
	for _, kind := range []string{"dialogue", "inner_monologue", "narration"} {
		line := `{"v":1,"seq":1,"type":"` + kind + `","text":"她把缆绳绕回船桩。"}`
		present, frame, err := parseFrame([]byte(line))
		if err != nil || !present {
			t.Fatalf("%s: 应被接受，得到 present=%v err=%v", kind, present, err)
		}
		block, ok := frame.(BlockFrame)
		if !ok {
			t.Fatalf("%s: 帧类型 = %T，应为 BlockFrame", kind, frame)
		}
		if string(block.Kind) != kind {
			t.Fatalf("%s: kind = %q", kind, block.Kind)
		}
	}
}

// 未知 type 仍然拒绝：兼容不能变成放行。
func TestUnknownFrameTypeIsStillRejected(t *testing.T) {
	line := `{"v":1,"seq":1,"type":"monologue","text":"x"}`
	present, _, err := parseFrame([]byte(line))
	if err == nil || present {
		t.Fatalf("未知 type 应被拒绝，得到 present=%v err=%v", present, err)
	}
}

// 一行两个 JSON 对象必须带 ValidationError 身份：应用层据此判 PROTOCOL_INVALID
// （模型输出不合协议，重试整轮无意义），而不是当成供应商故障去整轮重试。
// 这是真实出现过的失败形态（DeepSeek 偶发把两个帧挤到一行）。
func TestDoubleJSONObjectLineIsProtocolInvalid(t *testing.T) {
	p := NewStreamParser()
	line := mkBlock(1, BlockNarration, "a") + mkBlock(2, BlockNarration, "b")
	err := p.Feed([]byte(line + "\n"))
	if err == nil {
		t.Fatal("一行两个对象应被拒绝")
	}
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("错误必须带 ValidationError 身份（否则会被误判为供应商故障），实际 %T: %v", err, err)
	}
	if !strings.Contains(validation.Reason, "malformed JSON") {
		t.Fatalf("错误原因应说明 JSON 结构问题，实际：%s", validation.Reason)
	}
}

// final 之后再来的任何非空行都是硬失败：回合已经收尾，模型还在输出说明它
// 没有遵守协议，继续拼装只会产生半结构化污染。此前这条路径零测试覆盖。
func TestContentAfterFinalRejected(t *testing.T) {
	p := NewStreamParser()
	if err := p.Feed([]byte(mkFinal(1, "", "") + "\n")); err != nil {
		t.Fatalf("final 本身应被接受：%v", err)
	}
	if err := p.Feed([]byte("这句话不应出现\n")); !errors.Is(err, ErrAfterFinal) {
		t.Fatalf("final 之后的文本应返回 ErrAfterFinal，实际 %v", err)
	}
}

// 断线或刷新后，客户端只能靠 InFlight 补齐"正在写的这一段"：
// 已完成的块有 durable 的 block.appended 回放，未完成块的行内增量（Sequence 0）
// 永远不会重放，所以这里必须能拿到服务端解析器的实时文本。
func TestInFlightExposesPartialBlockText(t *testing.T) {
	p := NewStreamParser()
	if err := p.Feed([]byte(mkBlock(1, BlockNarration, "潮水漫过甲板。") + "\n")); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if _, _, _, _, ok := p.InFlight(); ok {
		t.Fatal("完整行结束后不应报告在途块")
	}

	full := mkBlock(2, BlockDialogue, "「你听，海在说话。」")
	// 切在正文值内部（每个汉字 3 字节，9 字节正好是 3 个字的边界）。
	cut := strings.Index(full, `"text":"`) + len(`"text":"`) + 9
	if err := p.Feed([]byte(full[:cut])); err != nil {
		t.Fatalf("feed partial: %v", err)
	}
	seq, kind, speaker, text, ok := p.InFlight()
	if !ok {
		t.Fatal("未闭合行应当报告在途块")
	}
	if seq != 2 || kind != BlockDialogue {
		t.Fatalf("在途块 = seq:%d kind:%s", seq, kind)
	}
	if !strings.HasPrefix("「你听，海在说话。」", text) || len([]rune(text)) < 3 {
		t.Fatalf("在途文本应是该块已收到的前缀，实际 %q", text)
	}
	if speaker != nil {
		t.Fatalf("无 speakerId 时应为 nil，实际 %v", *speaker)
	}
	if err := p.Feed([]byte(full[cut:] + "\n")); err != nil {
		t.Fatalf("补完该行: %v", err)
	}
	if _, _, _, _, ok := p.InFlight(); ok {
		t.Fatal("补完后不应再报告在途块")
	}
}
