package trace

import (
	"context"
	"errors"
	"testing"
	"time"

	"tavernagent/internal/ports"
)

func TestNilRecorderIsSafe(t *testing.T) {
	var recorder *Recorder
	ctx, span := recorder.Start(context.Background(), "noop")
	if ctx == nil {
		t.Fatal("nil 记录器也要返回可用的 context")
	}
	// no-op span 必须能被调用而不会 panic。
	span.SetAttr("k", "v")
	span.Mark("m")
	span.End(nil)
	if summary := recorder.TraceSnapshot(); summary.Total != 0 {
		t.Fatalf("nil 记录器不应有读数，得到 %+v", summary)
	}
}

func TestSpanRecordsAttrsMarksAndCounts(t *testing.T) {
	recorder := NewRecorder(4)
	_, span := recorder.Start(context.Background(), "http.request")
	span.SetAttr("requestId", "req-1")
	span.SetAttr("method", "GET")
	span.SetAttr("path", "/api/status")
	span.Mark("compile.lorebook")
	span.SetAttr("status", 200)
	span.End(nil)

	summary := recorder.TraceSnapshot()
	if summary.Total != 1 {
		t.Fatalf("total = %d, want 1", summary.Total)
	}
	if summary.Slow != 0 {
		t.Fatalf("未超阈值的请求不应计入 slow，得到 %d", summary.Slow)
	}
	if len(summary.Recent) != 1 {
		t.Fatalf("recent 应有 1 条，得到 %d", len(summary.Recent))
	}
	record := summary.Recent[0]
	if record.RequestID != "req-1" || record.Method != "GET" || record.Path != "/api/status" || record.Status != 200 {
		t.Fatalf("属性未落到记录上: %+v", record)
	}
	if _, ok := record.Marks["compile.lorebook"]; !ok {
		t.Fatalf("分段打点丢失: %+v", record.Marks)
	}
}

func TestEndIsIdempotentAndAttrsAfterEndAreIgnored(t *testing.T) {
	recorder := NewRecorder(4)
	_, span := recorder.Start(context.Background(), "http.request")
	span.End(nil)
	span.End(nil)
	span.SetAttr("path", "/late")
	span.Mark("late")

	summary := recorder.TraceSnapshot()
	if summary.Total != 1 {
		t.Fatalf("End 必须幂等：total = %d, want 1", summary.Total)
	}
	if summary.Recent[0].Path != "" {
		t.Fatalf("End 之后的 SetAttr 不应生效: %+v", summary.Recent[0])
	}
	if _, ok := summary.Recent[0].Marks["late"]; ok {
		t.Fatalf("End 之后的 Mark 不应生效: %+v", summary.Recent[0].Marks)
	}
}

func TestFailureMarksTheSpan(t *testing.T) {
	recorder := NewRecorder(4)
	_, span := recorder.Start(context.Background(), "http.request")
	span.End(errors.New("boom"))

	if _, ok := recorder.TraceSnapshot().Recent[0].Marks["error"]; !ok {
		t.Fatal("失败的 span 应带上 error 标记")
	}
}

func TestRingKeepsNewestAndSlowest(t *testing.T) {
	recorder := NewRecorder(2)
	for _, name := range []string{"first", "second", "third"} {
		_, span := recorder.Start(context.Background(), name)
		span.End(nil)
	}

	summary := recorder.TraceSnapshot()
	if summary.Total != 3 {
		t.Fatalf("total 应统计全部 3 次，得到 %d", summary.Total)
	}
	if len(summary.Recent) != 2 {
		t.Fatalf("容量为 2 的环形缓冲只该留 2 条，得到 %d", len(summary.Recent))
	}
	if summary.Recent[0].Name != "third" || summary.Recent[1].Name != "second" {
		t.Fatalf("recent 应按时间倒序: %+v", summary.Recent)
	}

	// 慢请求单独排一次序：sleep 过的那次必须排第一。
	recorder2 := NewRecorder(4)
	_, fast := recorder2.Start(context.Background(), "fast")
	fast.End(nil)
	_, slow := recorder2.Start(context.Background(), "slow")
	time.Sleep(5 * time.Millisecond)
	slow.End(nil)

	slowest := recorder2.TraceSnapshot().Slowest
	if len(slowest) == 0 || slowest[0].Name != "slow" {
		t.Fatalf("slowest 首位应是慢的那次: %+v", slowest)
	}
}

func TestSlowThresholdCounts(t *testing.T) {
	recorder := NewRecorder(4)
	recorder.SetSlowThreshold(time.Nanosecond)
	_, span := recorder.Start(context.Background(), "http.request")
	// 睡一小会儿：Windows 上极短的操作可能测出 0，阈值判定会变成掷硬币。
	time.Sleep(time.Millisecond)
	span.End(nil)

	summary := recorder.TraceSnapshot()
	if summary.Slow != 1 {
		t.Fatalf("超过阈值的请求应计入 slow，得到 %d", summary.Slow)
	}
	if summary.SlowThresholdMs != 0 { // 1ns 取整后是 0ms，这里只要求字段被回传
		t.Fatalf("阈值应被回传，得到 %d", summary.SlowThresholdMs)
	}
}

// 记录器必须满足端口契约：换实现时状态端点靠这个断言决定"有没有 trace 段"。
var _ ports.Tracer = (*Recorder)(nil)
var _ ports.TraceSnapshotter = (*Recorder)(nil)
