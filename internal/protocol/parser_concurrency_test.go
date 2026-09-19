package protocol

import (
	"strings"
	"sync"
	"testing"
)

// 生成 goroutine 在 Feed，HTTP 轮询 goroutine 在读 InFlight（GET /turns 取在途正文）。
// 两者是完全并行的两件事——客户端每 5 秒轮询一次，正好落在生成过程中间。
//
// 本用例的牙齿在 `go test -race`：把 InFlight 改回直接读 p.buf，race detector 会报
// "Previous read at ... by StreamParser.InFlight / Write at ... by StreamParser.Feed"，
// 并指名 parser.go 的这两个方法。不跑 -race 时它仍验证并发下的快照完整性。
func TestStreamParserInFlightIsSafeDuringConcurrentFeed(t *testing.T) {
	p := NewStreamParser()
	var wg sync.WaitGroup
	stop := make(chan struct{})
	rr := strings.NewReader("")

	reader := func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			seq, kind, _, text, ok := p.InFlight()
			if ok && (seq <= 0 || kind == "" || text == "") {
				// 快照是整体替换的指针：读侧不允许看到半成品（否则正文会缺字）。
				t.Errorf("快照不完整: seq=%d kind=%q text=%q", seq, kind, text)
				return
			}
		}
	}
	wg.Add(1)
	go reader()

	// 逐字节喂入（最坏切分），同时让读侧一直并发地读。
	for seq := 1; seq <= 6; seq++ {
		line := mkBlock(seq, BlockNarration, "潮水漫过甲板，远处的灯塔亮了一次。") + "\n"
		rr = strings.NewReader(line)
		for {
			b, err := rr.ReadByte()
			if err != nil {
				break
			}
			if err := p.Feed([]byte{b}); err != nil {
				t.Fatalf("feed seq %d: %v", seq, err)
			}
		}
	}
	close(stop)
	wg.Wait()

	// 最后一整行已被消费：不再有"正在写的块"。
	if _, _, _, text, ok := p.InFlight(); ok || text != "" {
		t.Fatalf("行已闭合后不应再报告在途块: ok=%v text=%q", ok, text)
	}
	if len(p.Blocks()) != 6 {
		t.Fatalf("块数 = %d, want 6", len(p.Blocks()))
	}
}

// 纯叙事兼容模式不产出块帧，因此也不该报告在途块（此时正文由 raw + 增量回调承载）。
// 这条同时钉住"降级时要清空快照"：否则降级前的残行会一直挂在读侧。
func TestStreamParserInFlightEmptyInNarrativeMode(t *testing.T) {
	p := NewStreamParser()
	if err := p.Feed([]byte("这是一段没有帧协议的纯文本开头。")); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if _, _, _, _, ok := p.InFlight(); ok {
		t.Fatal("兼容模式不应报告在途块")
	}
}
