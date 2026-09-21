// Package ports 的追踪端口：上层只依赖接口，不依赖任何具体追踪实现。
//
// 为什么不直接接 OpenTelemetry SDK：它会给单二进制带来一整棵依赖树，而本项目的
// 交付形态恰恰是"一个可执行文件 + 零外部服务"。这里先把**缝**留出来——
// 调用点按 Span 打点，组合根决定背后是 no-op、进程内记录器，还是将来的 OTLP 导出器。
// 换实现时调用点一行都不用改。
package ports

import "context"

// Span 是一次可观测操作的句柄。实现必须保证 End 幂等。
type Span interface {
	// End 结束一次追踪；err 非空表示这次操作失败。
	End(err error)
	// SetAttr 附加结构化上下文（请求 ID、状态码、模型槽位、节点数等）。
	SetAttr(key string, value any)
	// Mark 在时间轴上打一个点，用于分段耗时（如 compile.lorebook）。
	Mark(name string)
}

// Tracer 开启一次追踪，并把 Span 挂到返回的 context 上供下游取用。
type Tracer interface {
	Start(ctx context.Context, name string) (context.Context, Span)
}

// NoopTracer 是默认实现：不记录任何东西，但让调用点不必判空。
type NoopTracer struct{}

// Start 实现 Tracer。
func (NoopTracer) Start(ctx context.Context, _ string) (context.Context, Span) {
	return ctx, NoopSpan{}
}

// NoopSpan 是 NoopTracer 产出的空 Span。
type NoopSpan struct{}

// End 实现 Span。
func (NoopSpan) End(error) {}

// SetAttr 实现 Span。
func (NoopSpan) SetAttr(string, any) {}

// Mark 实现 Span。
func (NoopSpan) Mark(string) {}

// TraceRecord 是一次已完成请求的读数（形状即 /api/status 的契约）。
type TraceRecord struct {
	RequestID string  `json:"requestId,omitempty"`
	Name      string  `json:"name"`
	Method    string  `json:"method,omitempty"`
	Path      string  `json:"path,omitempty"`
	Status    int     `json:"status,omitempty"`
	Ms        float64 `json:"ms"`
	// Marks 是分段耗时（相对请求开始的毫秒），键为 Mark 的名字。
	Marks map[string]float64 `json:"marks,omitempty"`
}

// TraceSummary 是追踪读数的稳定形状：只回答"最近发生过什么、慢在哪"。
//
// 设计约束与 /api/status 一致：只读、失败降级、绝不因为观测失败而返回 5xx。
type TraceSummary struct {
	Total           int `json:"total"`
	Slow            int `json:"slow"`
	SlowThresholdMs int `json:"slowThresholdMs"`
	// Recent 按时间倒序（最近发生了什么）。
	Recent []TraceRecord `json:"recent,omitempty"`
	// Slowest 按耗时倒序（最慢的几次卡在哪一段）。
	Slowest []TraceRecord `json:"slowest,omitempty"`
}

// TraceSnapshotter 是"能读出历史记录"的 Tracer（进程内记录器实现它）。
//
// /api/status 用它取值：换成 OTLP 导出器时该断言失败即可，状态端点自动少一段，
// 而不是报错——观测不得影响服务可用性。
type TraceSnapshotter interface {
	TraceSnapshot() TraceSummary
}
