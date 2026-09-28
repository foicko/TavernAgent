package context

import "testing"

// 黄金指纹：钉住"模型所见提示词"的模板集合。
//
// 任何会改变模型所见内容的改动都会改变 PromptFingerprint()，从而让本测试失败——
// 这正是目的：提示词是最容易在不经意间被改动的部分，改了就应当被显式看见。
//
// 有意修改提示词时：升 PromptVersion、同步更新 docs/PROMPTS.md，并把下面的
// 期望值改成新的 PromptManifest() 输出（可用 `go test ./internal/context -run
// TestPromptManifestIsPinned -v` 的失败信息拿到新值）。
func TestPromptManifestIsPinned(t *testing.T) {
	const want = "v7/db1270b985da"
	if got := PromptManifest(); got != want {
		t.Fatalf("提示词版本/指纹变化：got %s, want %s\n"+
			"若为有意修改：升 PromptVersion、同步 docs/PROMPTS.md，并更新本测试期望值。", got, want)
	}
}

// 模板清单是版本号与指纹的分母：数量变化（新增/删除模板）必须是有意的。
func TestPromptTemplateCountIsPinned(t *testing.T) {
	const want = 15
	if got := len(promptTemplates); got != want {
		t.Fatalf("提示词模板数量 = %d, want %d；新增模板必须追加到末尾并同步黄金指纹。", got, want)
	}
}
