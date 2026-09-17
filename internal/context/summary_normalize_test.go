package context

import (
	"strings"
	"testing"
)

// 归一器用例用的是**真实抓到的模型输出形状**（见 output/functional/acceptance/evidence/）：
//  1. <character_dynamics> 套自身；
//  2. <open_loops> 被塞进 <character_dynamics> 里。
//
// 目标不是"让它通过"，而是"修好排版后仍然过严格校验"。
func TestNormalizeSummaryRepairsRealDrift(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{
			name: "章节套自身",
			raw: `<story_checkpoint>
<character_dynamics>
<character_dynamics>
<mindset character="池夏">从试探转为戒备。</mindset>
<hidden_tension>她怀疑旅人藏了东西。</hidden_tension>
</character_dynamics>
</character_dynamics>
<narrative_arc>雾夜登船，旅人报出码头编号。</narrative_arc>
<open_loops>
- [悬念] 等船灯为什么自己亮
</open_loops>
<milestones>
- [第7轮] 旧规矩被摆上台面
</milestones>
</story_checkpoint>`,
		},
		{
			name: "顶层章节被塞进 character_dynamics",
			raw: `<story_checkpoint>
<narrative_arc>旅人追问灯的来历。</narrative_arc>
<character_dynamics>
<mindset character="池夏">嘴上逞强，手却在照顾旅人。</mindset>
<open_loops>
- [承诺] 数完人数再决定开不开船
</open_loops>
<hidden_tension>她没说出多出来的那道刻痕。</hidden_tension>
</character_dynamics>
<milestones>
- [第11轮] 双方就旧规矩达成共识
</milestones>
</story_checkpoint>`,
		},
		{
			name: "围栏 + 未知标签 + 游离文本",
			raw:  "```xml\n这里是摘要：\n<story_checkpoint>\n<narrative_arc>开场回顾。<b>强调</b>继续。</narrative_arc>\n<open_loops>\n- [威胁] 雾里有人\n</open_loops>\n</story_checkpoint>\n```",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, normalized, err := ParseAndNormalizeSummary(tc.raw)
			if err != nil {
				t.Fatalf("归一后仍不合法：%v\n%s", err, text)
			}
			if !normalized {
				t.Fatalf("应标记为发生过修复")
			}
			if n := strings.Count(text, "<character_dynamics>"); n > 1 {
				t.Fatalf("同名章节未合并（%d 个）：\n%s", n, text)
			}
			if n := strings.Count(text, "<narrative_arc>"); n != 1 {
				t.Fatalf("narrative_arc 缺失或重复（%d 个）：\n%s", n, text)
			}
			if err := validateSummaryXML(text); err != nil {
				t.Fatalf("归一结果未通过严格校验：%v\n%s", err, text)
			}
		})
	}
}

// 合法输入不该被改动，也不该被标记为修复过（否则计数失去意义）。
func TestNormalizeSummaryKeepsValidInputUntouched(t *testing.T) {
	valid := "<story_checkpoint><narrative_arc>开场回顾。</narrative_arc><open_loops>- [悬念] 灯</open_loops></story_checkpoint>"
	text, normalized, err := ParseAndNormalizeSummary(valid)
	if err != nil || normalized {
		t.Fatalf("合法输入被判定为需要修复：normalized=%v err=%v", normalized, err)
	}
	if text != valid {
		t.Fatalf("合法输入被改写：\n%s", text)
	}
}

// 归一不做无中生有：缺章节时仍然失败，交给修复重试与退避处理。
func TestNormalizeSummaryDoesNotInventMissingSections(t *testing.T) {
	raw := `<story_checkpoint><narrative_arc>只有这一段。</narrative_arc></story_checkpoint>`
	if _, _, err := ParseAndNormalizeSummary(raw); err == nil {
		t.Fatalf("缺少 open_loops 时必须继续失败，不能凭空补章节")
	}
}

// 子章节的属性必须原样保留：mindset 靠 character="名字" 区分角色，
// 丢掉属性等于把角色心声张冠李戴。
func TestNormalizeSummaryPreservesSubsectionAttributes(t *testing.T) {
	raw := `<story_checkpoint>
<character_dynamics><character_dynamics>
<mindset character="池夏">戒备</mindset>
<mindset character="旅人">犹豫</mindset>
</character_dynamics></character_dynamics>
<narrative_arc>回顾。</narrative_arc>
<open_loops>- [悬念] 灯</open_loops>
</story_checkpoint>`
	text, _, err := ParseAndNormalizeSummary(raw)
	if err != nil {
		t.Fatalf("归一失败：%v", err)
	}
	if strings.Count(text, `character="池夏"`) != 1 || strings.Count(text, `character="旅人"`) != 1 {
		t.Fatalf("子章节属性丢失：\n%s", text)
	}
}
