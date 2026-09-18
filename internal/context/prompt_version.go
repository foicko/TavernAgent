package context

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// 提示词版本与指纹（ADS-7.8-04）。
//
// 目标：让"改了提示词之后成功率变了"这件事可归因。此前提示词是散落在多个文件里的
// 裸字符串常量，既没有版本号也没有指纹——同一次代码版本之间无法判断行为差异
// 来自模型、Harness 还是提示词；而提示词恰恰是最容易在不经意间被改动的部分
// （补一句措辞、调一个示例）。这里把它变成可快照、可回归的契约：
//
//  1. PromptVersion 供人读，用于对外说明"这套提示词是第几版"；
//  2. PromptFingerprint 由模板内容算出，用于与真实用量台账对齐——同一次运行
//     到底跑的是哪一份提示词，可逐字节验证；
//  3. prompt_fingerprint_test.go 钉住黄金指纹，改动提示词而不更新版本号会让 CI 失败。
const (
	// PromptVersion 是提示词契约版本号。
	//
	// 何时必须 +1：任何会改变模型所见内容的改动——角色准则、输出协议、资料边界
	// 声明、摘要提示词、导演提示词、帧协议字段。改动后必须同步更新黄金指纹。
	//
	// 何时不必 +1：注释、变量重命名、不改变渲染结果的纯重构。
	PromptVersion = 1
)

// promptTemplate 是被纳入指纹的提示词模板。Name 只用于组装哈希输入与排障展示。
type promptTemplate struct {
	Name string
	Text string
}

// promptTemplates 是参与指纹的模板清单。
//
// 顺序一经确定不得调整：调整顺序会改变指纹却不改变语义，制造无意义的版本噪音。
// 新增模板必须追加到末尾，并同步更新黄金指纹。
var promptTemplates = []promptTemplate{
	{"narrator_role", NarratorRoleInstruction},
	{"frame_protocol", FrameProtocolInstruction},
	{"frame_probe", FrameProbeInstruction},
	{"external_boundary", ExternalBoundaryInstruction},
	{"director_head", DirectorInstructionHead},
	{"director_mid", DirectorInstructionMid},
	{"director_tail", DirectorInstructionTail},
	{"summary_prompt", CompactionSystemPrompt},
	{"system_reminder", SystemReminderOpen + SystemReminderClose},
	{"external_content", externalContentOpen + externalContentClose},
}

// PromptFingerprint 返回提示词模板集合的短指纹（12 位十六进制）。
//
// 用 NUL 作分隔符：它能出现在任何一段模板文本里的概率极低，从而避免
// "边界位移"式碰撞（把 A 的尾部和 B 的头部错拼成同一串仍得到相同哈希）。
func PromptFingerprint() string {
	h := sha256.New()
	for _, t := range promptTemplates {
		fmt.Fprintf(h, "%s\x00%s\x00", t.Name, t.Text)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// PromptManifest 返回"版本 + 指纹"，用于启动日志与诊断接口。
func PromptManifest() string {
	return fmt.Sprintf("v%d/%s", PromptVersion, PromptFingerprint())
}

// PromptTemplateNames 返回参与指纹的模板名，便于排障时确认覆盖范围。
func PromptTemplateNames() string {
	names := make([]string, 0, len(promptTemplates))
	for _, t := range promptTemplates {
		names = append(names, t.Name)
	}
	return strings.Join(names, ",")
}
