package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// 契约测试：SSE/outbox 事件名的唯一来源是 testdata/contract/sse-events.json。
// 1) 常量清单必须与契约一致；
// 2) 扫描实际发出事件的服务端源码，任何不在契约里的事件名都会失败（防漏改名）。
type sseContract struct {
	TurnEvents         []string `json:"turnEvents"`
	TerminalTurnEvents []string `json:"terminalTurnEvents"`
	DirectorEvents     []string `json:"directorEvents"`
	InternalEvents     []string `json:"internalEvents"`
}

var eventLiteralRe = regexp.MustCompile(`"(turn|block|director)\.[a-z_.]+"`)

func TestSSEEventContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contract", "sse-events.json"))
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var doc sseContract
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse contract: %v", err)
	}

	if !reflect.DeepEqual(TurnEvents, doc.TurnEvents) {
		t.Fatalf("TurnEvents = %v, want %v", TurnEvents, doc.TurnEvents)
	}
	if !reflect.DeepEqual(TerminalTurnEvents, doc.TerminalTurnEvents) {
		t.Fatalf("TerminalTurnEvents = %v, want %v", TerminalTurnEvents, doc.TerminalTurnEvents)
	}
	if !reflect.DeepEqual(DirectorEvents, doc.DirectorEvents) {
		t.Fatalf("DirectorEvents = %v, want %v", DirectorEvents, doc.DirectorEvents)
	}
	if !reflect.DeepEqual(InternalEvents, doc.InternalEvents) {
		t.Fatalf("InternalEvents = %v, want %v", InternalEvents, doc.InternalEvents)
	}

	known := map[string]bool{}
	for _, group := range [][]string{doc.TurnEvents, doc.DirectorEvents, doc.InternalEvents} {
		for _, name := range group {
			known[name] = true
		}
	}
	// 扫描真实发出事件的包；adapters/tts 的 "Path:turn.end" 是供应商协议串，不在范围内。
	for _, dir := range []string{"../application", "../adapters/sqlite", "../adapters/http"} {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			for _, match := range eventLiteralRe.FindAllString(string(content), -1) {
				name := strings.Trim(match, `"`)
				if !known[name] {
					t.Errorf("源码出现契约外事件名 %q（%s）；请更新 sse-events.json 或改回契约名", name, path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
}
