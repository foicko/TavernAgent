package application

import (
	"testing"

	ctxpkg "tavernagent/internal/context"
)

// 目录是客户端的枚举与标签唯一来源：字段缺失会让界面出现空白选项，
// 因此这里显式钉住关键内容，而不是只测"非空"。
func TestBuildCatalogContent(t *testing.T) {
	c := BuildCatalog()
	if c.Version == "" {
		t.Fatal("版本为空：客户端缓存无法失效")
	}
	if len(c.Attributes) == 0 || len(c.CheckOutcomes) == 0 {
		t.Fatalf("检定元数据缺失: attrs=%d outcomes=%d", len(c.Attributes), len(c.CheckOutcomes))
	}
	if len(c.Slots) != 3 {
		t.Fatalf("槽位数 = %d, 期望 3", len(c.Slots))
	}
	if c.DefaultProtocol == "" {
		t.Fatal("默认协议为空")
	}
	// 每个预设的协议必须是已声明的协议之一：否则新建连接会落到未知适配器。
	known := map[string]bool{}
	for _, p := range c.Protocols {
		known[p.Value] = true
	}
	for _, preset := range c.ProviderPresets {
		if !known[preset.Kind] {
			t.Errorf("预设 %s 的协议 %q 未在 protocols 中声明", preset.ID, preset.Kind)
		}
	}
	if len(c.ProviderPresets) == 0 {
		t.Fatal("服务商预设为空")
	}
	// 思考强度必须包含"默认"档（空值），否则界面无法表达"不下发思考参数"。
	found := false
	for _, e := range c.ThinkingEfforts {
		if e.Value == "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("思考强度缺少默认档")
	}
}

// 预算策略常量必须与 context 包的实现同源，否则"界面说还有空间、
// 实际已被裁剪"会再次分叉（这正是本目录要消灭的问题）。
func TestCatalogContextPolicyMatchesCompiler(t *testing.T) {
	c := BuildCatalog()
	if c.ContextPolicy.DefaultWindow != ctxpkg.DefaultContextWindow {
		t.Errorf("默认窗口 = %d, 期望 %d", c.ContextPolicy.DefaultWindow, ctxpkg.DefaultContextWindow)
	}
	if c.ContextPolicy.DefaultReservedOut != ctxpkg.DefaultReservedOutput {
		t.Errorf("输出预留 = %d, 期望 %d", c.ContextPolicy.DefaultReservedOut, ctxpkg.DefaultReservedOutput)
	}
	// 安全余量下限与除数：用 InputBudget 反推验证，避免常量与函数脱钩。
	const window, reserved = 40000, 2000
	if got, want := ctxpkg.InputBudget(window, reserved), window-reserved-2000; got != want {
		t.Errorf("InputBudget(40000,2000) = %d, 期望 %d", got, want)
	}
	if got := ctxpkg.InputBudget(-1, 0); got != ctxpkg.DefaultContextWindow-ctxpkg.DefaultReservedOutput-ctxpkg.SafetyMargin(ctxpkg.DefaultContextWindow) {
		t.Errorf("缺省输入预算 = %d", got)
	}
}

func TestBuildCatalogVersionIsStable(t *testing.T) {
	first, second := BuildCatalog(), BuildCatalog()
	if first.Version != second.Version {
		t.Fatal("同一份内容两次构建的版本不一致：版本必须由内容决定")
	}
}
