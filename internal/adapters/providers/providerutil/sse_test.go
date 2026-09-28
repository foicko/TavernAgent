package providerutil

import (
	"context"
	"strings"
	"testing"
)

func collectSSE(t *testing.T, input string) []SSEEvent {
	t.Helper()
	events, errs := ReadSSE(context.Background(), strings.NewReader(input))
	var out []SSEEvent
	for ev := range events {
		out = append(out, ev)
	}
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}
	default:
	}
	return out
}

func TestReadSSEJoinsMultilineDataAndKeepsEventNames(t *testing.T) {
	got := collectSSE(t, "event: block.delta\ndata: {\"a\":\ndata: 1}\n\n")
	if len(got) != 1 {
		t.Fatalf("want 1 event, got %d", len(got))
	}
	if got[0].Event != "block.delta" {
		t.Fatalf("event = %q", got[0].Event)
	}
	if string(got[0].Data) != "{\"a\":\n1}" {
		t.Fatalf("data = %q", got[0].Data)
	}
}

func TestReadSSEHandlesFinalEventWithoutTrailingNewline(t *testing.T) {
	got := collectSSE(t, "data: one\n\ndata: two")
	if len(got) != 2 {
		t.Fatalf("want 2 events, got %d", len(got))
	}
	if string(got[1].Data) != "two" {
		t.Fatalf("last data = %q", got[1].Data)
	}
}

func TestReadSSEFlushesValidJSONBeforeNextEventWithoutBlankLine(t *testing.T) {
	// 网关漏发空行：上一段已是完整 JSON，遇到新 data 应先派发而不是拼成一段。
	got := collectSSE(t, "data: {\"a\":1}\ndata: {\"b\":2}\n\n")
	if len(got) != 2 {
		t.Fatalf("want 2 events, got %d: %#v", len(got), got)
	}
	if string(got[0].Data) != "{\"a\":1}" || string(got[1].Data) != "{\"b\":2}" {
		t.Fatalf("bad split: %#v", got)
	}
}

func TestReadSSEIgnoresCommentsAndEmptyData(t *testing.T) {
	got := collectSSE(t, ": ping\n\ndata: \n\ndata: real\n\n")
	if len(got) != 1 || string(got[0].Data) != "real" {
		t.Fatalf("got %#v", got)
	}
}
