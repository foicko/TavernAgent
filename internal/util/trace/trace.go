// Package trace 提供进程内的追踪实现：环形缓冲保存最近若干次请求的耗时与分段打点。
//
// 定位：回答"刚才那次请求慢在哪一段"，而不是替代完整的分布式追踪。
// 没有外部依赖、没有后台 goroutine、容量固定，因此在热路径上只多一次互斥锁。
package trace

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"tavernagent/internal/ports"
)

// DefaultCapacity 是环形缓冲的记录条数：够看清"最近的毛刺"，又不会无限增长。
const DefaultCapacity = 64

// DefaultSlowThreshold 是"慢请求"的判定阈值，用于在状态端点里给出慢请求计数。
const DefaultSlowThreshold = 500 * time.Millisecond

// recorder 是 span 的共享状态。
type recorder struct {
	mu        sync.Mutex
	ring      []ports.TraceRecord
	next      int
	filled    bool
	capacity  int
	threshold time.Duration
}

// Recorder 是进程内追踪记录器：实现 ports.Tracer，并可按需读出快照。
type Recorder struct {
	rec   *recorder
	total atomic.Int64
	slow  atomic.Int64
}

// NewRecorder 创建记录器；capacity <= 0 时使用 DefaultCapacity。
func NewRecorder(capacity int) *Recorder {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Recorder{rec: &recorder{ring: make([]ports.TraceRecord, capacity), capacity: capacity, threshold: DefaultSlowThreshold}}
}

// SetSlowThreshold 调整慢请求阈值（测试与后续调参用）。
func (r *Recorder) SetSlowThreshold(d time.Duration) {
	if r == nil || d <= 0 {
		return
	}
	r.rec.mu.Lock()
	r.rec.threshold = d
	r.rec.mu.Unlock()
}

// Start 实现 ports.Tracer。
func (r *Recorder) Start(ctx context.Context, name string) (context.Context, ports.Span) {
	if r == nil {
		return ctx, ports.NoopSpan{}
	}
	return ctx, &span{owner: r, name: name, started: time.Now(), marks: map[string]float64{}}
}

// TraceSnapshot 实现 ports.TraceSnapshotter。
func (r *Recorder) TraceSnapshot() ports.TraceSummary {
	if r == nil {
		return ports.TraceSummary{}
	}
	r.rec.mu.Lock()
	capacity, threshold := r.rec.capacity, r.rec.threshold
	records := make([]ports.TraceRecord, 0, capacity)
	if r.rec.filled {
		// 环形缓冲里 next 指向最旧的一条。
		records = append(records, r.rec.ring[r.rec.next:]...)
		records = append(records, r.rec.ring[:r.rec.next]...)
	} else {
		records = append(records, r.rec.ring[:r.rec.next]...)
	}
	r.rec.mu.Unlock()

	summary := ports.TraceSummary{
		Total:           int(r.total.Load()),
		Slow:            int(r.slow.Load()),
		SlowThresholdMs: int(threshold / time.Millisecond),
		Recent:          lastN(records, 8),
		Slowest:         slowestN(records, 5),
	}
	return summary
}

// finish 把一次完成的 span 写进环形缓冲。
func (r *Recorder) finish(record ports.TraceRecord, duration time.Duration) {
	r.total.Add(1)
	r.rec.mu.Lock()
	if duration >= r.rec.threshold {
		r.slow.Add(1)
	}
	r.rec.ring[r.rec.next] = record
	r.rec.next = (r.rec.next + 1) % r.rec.capacity
	if r.rec.next == 0 {
		r.rec.filled = true
	}
	r.rec.mu.Unlock()
}

// lastN 取按时间倒序的最近 n 条。
func lastN(records []ports.TraceRecord, n int) []ports.TraceRecord {
	if len(records) <= n {
		out := make([]ports.TraceRecord, len(records))
		for i := range records {
			out[i] = records[len(records)-1-i]
		}
		return out
	}
	out := make([]ports.TraceRecord, 0, n)
	for i := len(records) - 1; i >= len(records)-n; i-- {
		out = append(out, records[i])
	}
	return out
}

// slowestN 取耗时最长的 n 条（并列时保持时间倒序，避免读数抖动）。
func slowestN(records []ports.TraceRecord, n int) []ports.TraceRecord {
	ordered := lastN(records, len(records))
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Ms > ordered[j].Ms })
	if len(ordered) > n {
		ordered = ordered[:n]
	}
	return ordered
}

// span 是一次请求的写入端。End 之后不可再用（幂等：第二次调用直接返回）。
type span struct {
	owner   *Recorder
	name    string
	started time.Time
	marks   map[string]float64
	attrs   map[string]any
	ended   bool
}

// End 实现 ports.Span。
func (s *span) End(err error) {
	if s.ended {
		return
	}
	s.ended = true
	duration := time.Since(s.started)
	record := ports.TraceRecord{
		Name: s.name,
		Ms:   float64(duration) / float64(time.Millisecond),
	}
	if len(s.marks) > 0 {
		record.Marks = s.marks
	}
	for key, value := range s.attrs {
		switch key {
		case "requestId":
			record.RequestID, _ = value.(string)
		case "method":
			record.Method, _ = value.(string)
		case "path":
			record.Path, _ = value.(string)
		case "status":
			record.Status, _ = value.(int)
		}
	}
	if err != nil {
		record.Marks = withMark(record.Marks, "error")
	}
	s.owner.finish(record, duration)
}

// SetAttr 实现 ports.Span。
func (s *span) SetAttr(key string, value any) {
	if s.ended {
		return
	}
	if s.attrs == nil {
		s.attrs = map[string]any{}
	}
	s.attrs[key] = value
}

// Mark 实现 ports.Span：记录相对开始的毫秒数。
func (s *span) Mark(name string) {
	if s.ended {
		return
	}
	if s.marks == nil {
		s.marks = map[string]float64{}
	}
	s.marks[name] = float64(time.Since(s.started)) / float64(time.Millisecond)
}

// withMark 复制一份 marks 并追加一个零值打点（用于 error 这类"有没有发生"的标记）。
func withMark(marks map[string]float64, name string) map[string]float64 {
	out := make(map[string]float64, len(marks)+1)
	for key, value := range marks {
		out[key] = value
	}
	if _, exists := out[name]; !exists {
		out[name] = 0
	}
	return out
}
