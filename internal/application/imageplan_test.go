package application

import (
	"strings"
	"testing"
)

// 插画提示词计划的用例对齐 web/src/lib/imagePromptExtractor.ts 的同名测试。

const imagePlanBlocksJSON = `{"blocks":[
  {"kind":"narration","text":"寒风中，她站在酒馆门前，月光洒在银白色的长发上。"},
  {"kind":"dialogue","text":"「要进来坐坐吗？」[IMG:emo_smile]"},
  {"kind":"inner_monologue","text":"（心中有些警惕）"}
]}`

func TestImagePlanFromBlocks(t *testing.T) {
	st, _, sessionID, rootID, _ := planStack(t)
	svc := NewImagePlanService(st)
	nodeID := insertPlanNode(t, st, sessionID, rootID, imagePlanBlocksJSON)

	plan, err := svc.Plan(ImagePlanRequest{SessionID: sessionID, NodeID: nodeID, StylePreset: "anime"})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !strings.Contains(plan.Prompt, "寒风中，她站在酒馆门前") {
		t.Errorf("prompt 缺画面描述: %s", plan.Prompt)
	}
	// 括号注释（心理描写）必须清洗掉。
	if strings.Contains(plan.Prompt, "心中有些警惕") {
		t.Errorf("prompt 未清洗括号注释: %s", plan.Prompt)
	}
	if !strings.Contains(plan.Prompt, "masterpiece, best quality, anime visual novel CG") {
		t.Errorf("prompt 缺风格正向词: %s", plan.Prompt)
	}
	// 角色名与 sdData 基底都在最前面（顺序：角色 → 基底 → 画面 → 风格）。
	if !strings.HasPrefix(plan.Prompt, "Character: 林小月, masterpiece, 1girl, chinese hanfu, ") {
		t.Errorf("prompt 前缀顺序不对: %s", plan.Prompt)
	}
	if !strings.Contains(plan.NegativePrompt, "worst quality") {
		t.Errorf("negativePrompt = %s", plan.NegativePrompt)
	}
	if plan.CardID != "card_plan" {
		t.Errorf("cardId = %q", plan.CardID)
	}
	if plan.SdDataBase.Text != "masterpiece, 1girl, chinese hanfu" || len(plan.SdDataBase.Prompts) != 2 {
		t.Errorf("sdDataBase = %+v", plan.SdDataBase)
	}
	if len(plan.Refs) != 1 || plan.Refs[0].AssetKey != "emo_smile" {
		t.Errorf("refs = %+v, 期望收集正文引用的卡内图片", plan.Refs)
	}
}

func TestImagePlanStylePresetsAndEmptySource(t *testing.T) {
	st, _, sessionID, rootID, legacySessionID := planStack(t)
	svc := NewImagePlanService(st)

	nodeID := insertPlanNode(t, st, sessionID, rootID, `{"text":"rainy neon city"}`)
	cinematic, err := svc.Plan(ImagePlanRequest{SessionID: sessionID, NodeID: nodeID, StylePreset: "cinematic"})
	if err != nil {
		t.Fatalf("plan(cinematic): %v", err)
	}
	if !strings.Contains(cinematic.Prompt, "35mm film still, cinematic lighting") {
		t.Errorf("cinematic 风格未生效: %s", cinematic.Prompt)
	}
	if !strings.Contains(cinematic.NegativePrompt, "anime, cartoon, sketch") {
		t.Errorf("cinematic 负向词 = %s", cinematic.NegativePrompt)
	}

	// 空来源：只留风格正向词，不报错；无卡上下文时 cardId 仍回传会话绑定值。
	emptyID := insertPlanNode(t, st, legacySessionID, "root_"+legacySessionID, `{}`)
	empty, err := svc.Plan(ImagePlanRequest{SessionID: legacySessionID, NodeID: emptyID, StylePreset: "fantasy"})
	if err != nil {
		t.Fatalf("plan(empty): %v", err)
	}
	if empty.Prompt != imageStylePrompts["fantasy"].Positive {
		t.Errorf("空来源 prompt = %s", empty.Prompt)
	}
	if empty.CardID != "card_legacy_plan" {
		t.Errorf("cardId = %q, 期望会话绑定卡 ID", empty.CardID)
	}
	if empty.SdDataBase.Prompts == nil || len(empty.SdDataBase.Prompts) != 0 {
		t.Errorf("sdDataBase.prompts 应为空数组: %#v", empty.SdDataBase.Prompts)
	}
	if empty.Refs == nil || len(empty.Refs) != 0 {
		t.Errorf("refs 应为空数组: %#v", empty.Refs)
	}

	// 角色名可被请求显式覆盖。
	over, err := svc.Plan(ImagePlanRequest{SessionID: sessionID, NodeID: nodeID, CharacterName: "旅人"})
	if err != nil {
		t.Fatalf("plan(override): %v", err)
	}
	if !strings.HasPrefix(over.Prompt, "Character: 旅人, ") {
		t.Errorf("角色名覆盖未生效: %s", over.Prompt)
	}
}

func TestImagePlanUnknownNode(t *testing.T) {
	st, _, _, _, _ := planStack(t)
	svc := NewImagePlanService(st)
	if _, err := svc.Plan(ImagePlanRequest{NodeID: "n_missing"}); err == nil {
		t.Error("未知节点必须报错")
	}
	if _, err := svc.Plan(ImagePlanRequest{}); err == nil {
		t.Error("缺少 nodeId 必须报错")
	}
}
