package context_test

import (
	"context"
	"strings"
	"testing"

	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
)

// TestParseAblation 锁定消融开关的解析语义（ADS-7.8-01）。
//
// 关键约定：未知特性名必须**报错**而不是静默忽略——评估里一个拼错的开关名
// 会让这一组变成"以为关了其实没关"，比直接失败危险得多。
func TestParseAblation(t *testing.T) {
	cases := []struct {
		spec  string
		want  string // Ablation.String()
		isErr bool
	}{
		{"", "none", false},
		{"nonexistent-feature", "", true},
		{"all", "compaction,dynamic-context,lorebook,memory,summaries", false},
		{"memory", "memory", false},
		{"MEMORY , lorebook", "lorebook,memory", false},
		{"all,memory", "compaction,dynamic-context,lorebook,memory,summaries", false},
		{"dynamic-context,summaries", "dynamic-context,summaries", false},
	}
	for _, tc := range cases {
		got, err := ctxpkg.ParseAblation(tc.spec)
		if tc.isErr {
			if err == nil {
				t.Fatalf("ParseAblation(%q) 应报错（未知特性必须失败而不是静默忽略）", tc.spec)
			}
			if !strings.Contains(err.Error(), "未知消融特性") {
				t.Fatalf("ParseAblation(%q) 错误信息应指出未知特性，实际: %v", tc.spec, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseAblation(%q): %v", tc.spec, err)
		}
		if got.String() != tc.want {
			t.Fatalf("ParseAblation(%q) = %q, want %q", tc.spec, got.String(), tc.want)
		}
	}
}

// TestAblationRemovesInjectedMaterials 验证消融开关真的把材料从请求里拿掉了。
//
// 这是消融机制的**有效性**断言：只测"开关能解析"等于没测——真正要防的是
// 「开关设了但管线里没人读它」，那样基线评估会得出"这些特性原来没用"的错误结论。
func TestAblationRemovesInjectedMaterials(t *testing.T) {
	books := book(domain.LorebookEntry{
		EntryID: "lb_clock", Enabled: true, Keys: []string{"钟楼"},
		Content: "钟楼建于三百年以前。",
	})

	compile := func(t *testing.T, ablationSpec string) string {
		t.Helper()
		opts := ctxpkg.DefaultOptions()
		ab, err := ctxpkg.ParseAblation(ablationSpec)
		if err != nil {
			t.Fatal(err)
		}
		ab.Apply(&opts)
		f := newFixtureWithOpening(t, books, "故事开始了。", opts)
		head := f.seedMemory(t, testRootID, remember("m1", "她来自北方，很怕冷。"))
		req, err := f.compiler.Compile(context.Background(), testSessionID, head, "她来自北方，很怕冷，站在钟楼前。", nil, nil)
		if err != nil {
			t.Fatalf("compile(%q): %v", ablationSpec, err)
		}
		return injectedMaterials(req.Messages)
	}

	base := compile(t, "")
	if !strings.Contains(base, "钟楼建于三百年以前") || !strings.Contains(base, "她来自北方") {
		t.Fatalf("基线应同时注入世界书与记忆:\n%s", base)
	}
	if !strings.Contains(base, "【当前情境与状态提示】") {
		t.Fatalf("基线应包含动态状态块:\n%s", base)
	}
	// 分两路注入的形态：静态前缀独立成块，尾部另有状态块。
	// 用**结束标记**判定而不是开始标记：资料边界声明里本身就提到了 <system-reminder>
	// 这个标签名，按开始标记判定会把规则文本误当成状态块。
	if !strings.Contains(base, ctxpkg.SystemReminderClose) {
		t.Fatalf("基线应使用尾部状态块形态:\n%s", base)
	}

	noLore := compile(t, "lorebook")
	if strings.Contains(noLore, "钟楼建于三百年以前") {
		t.Fatalf("消融 lorebook 后世界书仍被注入:\n%s", noLore)
	}
	if !strings.Contains(noLore, "她来自北方") {
		t.Fatalf("消融 lorebook 不应影响记忆注入:\n%s", noLore)
	}

	noMemory := compile(t, "memory")
	if strings.Contains(noMemory, "她来自北方") {
		t.Fatalf("消融 memory 后记忆仍被注入:\n%s", noMemory)
	}

	// dynamic-context 关掉动静分离：动态状态回到首条 system（不再有尾部状态块）。
	merged := compile(t, "dynamic-context")
	if strings.Contains(merged, ctxpkg.SystemReminderClose) {
		t.Fatalf("消融 dynamic-context 后不应再有尾部状态块:\n%s", merged)
	}
	if !strings.Contains(merged, "钟楼建于三百年以前") {
		t.Fatalf("消融 dynamic-context 只应改变布局，材料仍须注入:\n%s", merged)
	}
}

// TestAblationDisablesCompaction 验证 compaction 消融同时关闭"生成"这一侧。
//
// 只关注入不关生成，会让"无压缩"那一组仍然承担后台摘要生成的模型开销——
// 评估里两个配置的差异就不再是单一变量。
func TestAblationDisablesCompaction(t *testing.T) {
	ab, err := ctxpkg.ParseAblation("compaction")
	if err != nil {
		t.Fatal(err)
	}
	opts := ctxpkg.DefaultOptions()
	ab.Apply(&opts)
	if !opts.CompactionPolicy.Disabled {
		t.Fatal("compaction 消融必须置位 CompactionPolicy.Disabled")
	}
	// 造一段远超阈值的祖先链，确认 DecideCompaction 恒判不压缩。
	ancestors := make([]*domain.PlotNode, 0, 60)
	for i := 0; i < 60; i++ {
		ancestors = append(ancestors, &domain.PlotNode{
			NodeID: "n" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
			Kind:   domain.NodeKindTurn, Depth: i, TurnNumber: i + 1,
		})
	}
	if d := ctxpkg.DecideCompaction(ancestors, nil, opts.CompactionPolicy); d.ShouldCompact {
		t.Fatalf("消融 compaction 后仍判定应压缩: %+v", d)
	}
	if d := ctxpkg.DecideCompaction(ancestors, nil, ctxpkg.DefaultCompactionPolicy()); !d.ShouldCompact {
		t.Fatalf("未消融时应判定可压缩（否则本用例没有区分度）: %+v", d)
	}
}
