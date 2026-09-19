package context_test

import (
	"testing"

	ctxpkg "tavernagent/internal/context"
)

// promptFingerprintGolden 是提示词模板集合的黄金指纹。
//
// 为什么必须有这份测试：提示词是最容易在不经意间被改动的部分——补一句措辞、
// 调整一个示例、顺手改个标点，都会改变模型看到的内容，而这类改动在代码评审里
// 看起来"无害"。一旦指纹变了而没人意识到，后续"成功率为什么变了"就失去归因依据
// （ADS-7.8-04：提示词需要版本化快照与回归测试）。
//
// 改动提示词时的正确操作：
//  1. 确认这次改动是有意的；
//  2. 把 internal/context/prompt_version.go 的 PromptVersion +1；
//  3. 用 PromptFingerprint() 的新值更新下面的常量。
//
// 若只是注释、重命名、不改变渲染结果的纯重构，指纹本就不会变，
// 此时本测试应当依旧通过——它失败就说明渲染结果确实变了。
const promptFingerprintGolden = "d2be89c8a92f"

func TestPromptFingerprintIsLocked(t *testing.T) {
	got := ctxpkg.PromptFingerprint()
	t.Logf("prompt manifest = %s (templates: %s)", ctxpkg.PromptManifest(), ctxpkg.PromptTemplateNames())
	if got != promptFingerprintGolden {
		t.Fatalf("提示词指纹变化：got %q, want %q\n"+
			"若这是有意的提示词改动，请把 PromptVersion +1 并更新黄金指纹；\n"+
			"否则说明渲染结果被意外改变。当前版本 %s。",
			got, promptFingerprintGolden, ctxpkg.PromptManifest())
	}
}

// TestPromptFingerprintCoversEverySharedTemplate 保证指纹覆盖的是"模板全集"。
//
// 存在理由：指纹只对纳入 promptTemplates 的文本敏感，漏登记一个模板就会让
// 改动它在 CI 里静默通过——那正是本机制要防的事。
func TestPromptFingerprintCoversEverySharedTemplate(t *testing.T) {
	names := ctxpkg.PromptTemplateNames()
	for _, want := range []string{
		"narrator_role", "frame_protocol", "frame_probe", "external_boundary",
		"director_head", "director_mid", "director_tail", "summary_prompt",
		"system_reminder", "external_content",
	} {
		if !containsName(names, want) {
			t.Fatalf("提示词模板 %q 未纳入指纹（当前：%s）；新增模板必须登记进 promptTemplates", want, names)
		}
	}
}

func containsName(csv, name string) bool {
	for len(csv) > 0 {
		i := 0
		for i < len(csv) && csv[i] != ',' {
			i++
		}
		if csv[:i] == name {
			return true
		}
		if i == len(csv) {
			break
		}
		csv = csv[i+1:]
	}
	return false
}
