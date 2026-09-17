package protocol

import (
	"reflect"
	"testing"
)

// 种子覆盖三类真实输入：合法帧、兼容模式纯文本、截断/非法/二进制残片。
func fuzzSeeds() [][]byte {
	return [][]byte{
		nil,
		[]byte("hello"),
		[]byte(`{"v":1,"seq":1,"type":"block","kind":"narration","speakerId":null,"text":"hi"}` + "\n"),
		[]byte(`{"v":1,"seq":1,"type":"final","proposals":[],"options":[]}`),
		[]byte("```json\n不是 JSON"),
		[]byte(`{"v":1,"seq":1,"type":"block","kind":"narration","text":"尾部残缺`),
		[]byte("\xff\xfe\x00binary"),
		[]byte(`{"v":1,"seq":2,"type":"block","kind":"narration","text":"x"}` + "\n"),
	}
}

// FuzzStreamParserNoPanic 对任意字节逐字节喂入并 Finish，断言解析器永不 panic。
func FuzzStreamParserNoPanic(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<14 {
			data = data[:1<<14]
		}
		p := NewStreamParser()
		for i := 0; i < len(data); i++ {
			if err := p.Feed(data[i : i+1]); err != nil {
				break
			}
		}
		_, _ = p.Finish()
	})
}

// runParser 按 chunk 大小喂入数据；遇到 Feed 错误即停止（模拟真实消费方），
// 最后 Finish。返回终态草稿、解析模式、是否发生 Feed 错误、是否发生 Finish 错误。
func runParser(data []byte, chunk int) (draft TurnDraft, mode string, feedErr, finishErr bool) {
	p := NewStreamParser()
	if chunk < 1 {
		chunk = len(data)
		if chunk < 1 {
			chunk = 1
		}
	}
	for i := 0; i < len(data); {
		end := i + chunk
		if end > len(data) {
			end = len(data)
		}
		if err := p.Feed(data[i:end]); err != nil {
			feedErr = true
			break
		}
		i = end
	}
	draft, err := p.Finish()
	return draft, p.Mode(), feedErr, err != nil
}

// FuzzStreamParserChunkingInvariant 断言：对任何**不产生解析错误**的输入，
// 「整块喂」与「任意分块喂」得到完全相同的终态（模式、草稿、Finish 错误）。
// 对应验收 T03：UTF-8、JSON 转义与标签文本在任意 chunk 处切断都要完整还原。
//
// 错误路径不在此断言：Feed 报错后解析器不再接收数据，其残态取决于分块位置，
// 与调用方是否继续喂入有关——那由 NoPanic 目标覆盖，不做等价要求。
func FuzzStreamParserChunkingInvariant(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed, 1)
	}
	f.Fuzz(func(t *testing.T, data []byte, chunk int) {
		if len(data) > 1<<14 {
			data = data[:1<<14]
		}
		whole, wholeMode, wholeFeedErr, wholeFinishErr := runParser(data, len(data))
		split, splitMode, splitFeedErr, splitFinishErr := runParser(data, chunk)
		if wholeFeedErr || splitFeedErr {
			return
		}
		if wholeMode != splitMode || wholeFinishErr != splitFinishErr || !reflect.DeepEqual(whole, split) {
			t.Fatalf("分块改变了终态 (chunk=%d len=%d):\n  整块 mode=%s finishErr=%v draft=%+v\n  分块 mode=%s finishErr=%v draft=%+v",
				chunk, len(data), wholeMode, wholeFinishErr, whole, splitMode, splitFinishErr, split)
		}
	})
}
