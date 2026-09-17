package application_test

import (
	"testing"

	"tavernagent/internal/context"
	"tavernagent/internal/protocol"
)

// TestProductionDefaultsLock 锁定全仓关键业务与工程默认值。
//
// 背景（技术评审 2026-09-12）：
// "机制层已建成，缺口集中在无度量、默认配置不正确、无门禁"。
// 若缺乏集中断言，生产默认值易在后续重构中被"设计对了但默认关闭"（如 SplitDynamicContext 此前默认 false、
// MinUncompactedTurns 默认 20 导致折叠步长过大、裁剪下限未设导致原文被裁）。
//
// 本测试作为 T5.5 生产默认值总关卡，任何项的回退都会导致测试失败。
func TestProductionDefaultsLock(t *testing.T) {
	t.Run("协议版本锁定", func(t *testing.T) {
		if protocol.Version != 1 {
			t.Fatalf("protocol.Version = %d, want 1", protocol.Version)
		}
	})

	t.Run("提示词编译前缀分离默认开启", func(t *testing.T) {
		opts := context.DefaultOptions()
		if !opts.SplitDynamicContext {
			t.Fatal("DefaultOptions.SplitDynamicContext 必须默认 true，否则前缀缓存命中会被动态 system 击穿")
		}
	})

	t.Run("上下文压缩策略生产默认值", func(t *testing.T) {
		policy := context.DefaultCompactionPolicy()
		if policy.TailWindowTurns != 20 {
			t.Fatalf("CompactionPolicy.TailWindowTurns = %d, want 20 (必须保护最近 20 轮对话原文)", policy.TailWindowTurns)
		}
		if policy.MinUncompactedTurns != 8 {
			t.Fatalf("CompactionPolicy.MinUncompactedTurns = %d, want 8 (小批次平滑压缩)", policy.MinUncompactedTurns)
		}
		if policy.PruneThoughtDepth != 20 {
			t.Fatalf("CompactionPolicy.PruneThoughtDepth = %d, want 20", policy.PruneThoughtDepth)
		}
	})

	t.Run("记忆检索候选池保底与降权策略", func(t *testing.T) {
		opts := context.DefaultOptions()
		if !opts.AllowUnrankedFallback {
			t.Fatal("DefaultOptions.AllowUnrankedFallback 必须默认 true (开启保底候选防语义改写漏召回)")
		}
		if opts.MaxUnrankedFallback != 2 {
			t.Fatalf("DefaultOptions.MaxUnrankedFallback = %d, want 2", opts.MaxUnrankedFallback)
		}
		if opts.MinFallbackImportance != 7 {
			t.Fatalf("DefaultOptions.MinFallbackImportance = %d, want 7 (重要性门槛防止噪声)", opts.MinFallbackImportance)
		}
	})
}
