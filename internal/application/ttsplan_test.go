package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// 演播计划的用例逐条对齐 web/src/lib/ttsDirector.ts 的同名测试，
// 目的是让"规则被搬到后端"这件事可被验证，而不是只靠人工阅读。

func TestTTSStripAudioTags(t *testing.T) {
	cases := []struct{ in, want string }{
		{"(冷峻)你想带我走？（轻蔑微讽）凭你也配。", "你想带我走？凭你也配。"},
		{"[深呼吸]外头狂风大作……[停顿片刻]快走！【冷笑】", "外头狂风大作……快走！"},
		{"(冰冷低音) [深呼吸] 别靠近我…… [停顿片刻]", "别靠近我……"},
		{"", ""},
	}
	for _, c := range cases {
		if got := ttsStripAudioTags(c.in); got != c.want {
			t.Errorf("stripAudioTags(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

func TestTTSDetectVoiceArchetype(t *testing.T) {
	cases := []struct {
		ctx        TTSPlanContext
		wantPreset string
		wantPhrase string
	}{
		{TTSPlanContext{CharacterName: "林小月", CharacterRole: "初入江湖的剑宗小师妹", CharacterPersona: "性格单纯天真，容易害羞"},
			"zh-CN-XiaoxiaoNeural", "年轻女声"},
		{TTSPlanContext{CharacterName: "艾伦", CharacterRole: "见习少年剑士", CharacterPersona: "热血冲动，憧憬冒险"},
			"zh-CN-YunxiNeural", "清爽年轻男声"},
		{TTSPlanContext{CharacterName: "岑当家", CharacterRole: "商会女主人", CharacterPersona: "手段雷厉风行，不怒自威的御姐"},
			"zh-CN-XiaoyiNeural", "成熟女声"},
		{TTSPlanContext{CharacterName: "玄空", CharacterRole: "隐世宗师", CharacterPersona: "白发老者，道骨仙风"},
			"zh-CN-YunjianNeural", "成熟沉稳男声"},
	}
	for _, c := range cases {
		preset, phrase := ttsDetectVoiceArchetype(c.ctx)
		if preset != c.wantPreset || !strings.Contains(phrase, c.wantPhrase) {
			t.Errorf("detectVoiceArchetype(%q) = (%q, %q), 期望预设 %q 含 %q",
				c.ctx.CharacterName, preset, phrase, c.wantPreset, c.wantPhrase)
		}
	}
}

func TestTTSBuildDirectorInstruction(t *testing.T) {
	instr := ttsBuildInstruction(TTSPlanContext{
		CharacterName: "艾伦", CharacterRole: "少年剑士", CharacterPersona: "性格直率开朗，富有正义感",
		SceneLocation: "晨曦酒馆二楼吧台", SceneStage: "初次相遇",
		MoodText: "兴奋难耐，充满期待", MoodCode: "happy",
		Relationship: true, Affection: 75, Alertness: 10, Trust: 80,
	})
	for _, want := range []string{
		"【角色】艾伦（少年剑士）。", "清爽年轻男声",
		"【场景】晨曦酒馆二楼吧台，阶段：初次相遇。",
		"【心境】兴奋难耐，充满期待。", "【指导】", "轻快笑意", "温情与信任",
	} {
		if !strings.Contains(instr, want) {
			t.Errorf("导演指令缺少 %q:\n%s", want, instr)
		}
	}

	fear := ttsBuildInstruction(TTSPlanContext{CharacterName: "小月", MoodText: "瑟瑟发抖，极为惶恐", MoodCode: "fear"})
	if !strings.Contains(fear, "细微颤音与慌乱") || !strings.Contains(fear, "呼吸浅急") {
		t.Errorf("恐惧指导未生效:\n%s", fear)
	}

	// 戒心优先于温情：警惕关系下不得同时表达亲昵。
	alert := ttsBuildInstruction(TTSPlanContext{
		CharacterName: "小月", Relationship: true, Alertness: 80, Affection: 90,
	})
	if !strings.Contains(alert, "戒心") || strings.Contains(alert, "温情与信任") {
		t.Errorf("戒心应优先于温情:\n%s", alert)
	}
}

func TestTTSEnrichDialogueWithAudioTags(t *testing.T) {
	cases := []struct {
		text, moodCode, moodText, want string
	}{
		{"你为什么要骗我？！", "angry", "暴怒质问", "(愤懑质问)你为什么要骗我？！"},
		{"那个……谢谢你刚才护着我。", "shy", "脸红心跳", "(羞赧轻声)那个……[停顿片刻]谢谢你刚才护着我。"},
		{"(冷酷)滚开。", "angry", "", "(冷酷)滚开。"},
	}
	for _, c := range cases {
		got := ttsEnrichDialogue(c.text, TTSPlanContext{MoodCode: c.moodCode, MoodText: c.moodText})
		if got != c.want {
			t.Errorf("enrich(%q) = %q, 期望 %q", c.text, got, c.want)
		}
	}
	if got := ttsEnrichDialogue("呵，你以为你能赢？", TTSPlanContext{MoodCode: "angry"}); !strings.Contains(got, "[冷哼]呵") {
		t.Errorf("愤怒笑声标签 = %q", got)
	}
	if got := ttsEnrichDialogue("呵，你这人真有意思！", TTSPlanContext{MoodCode: "happy"}); !strings.Contains(got, "[轻笑]呵") {
		t.Errorf("愉悦笑声标签 = %q", got)
	}
}

// planCard 带 sdData 冻结档案，供演播（角色人设）与插画（sdData 基底）共用。
const planCard = `{
  "schemaVersion": 2,
  "cardId": "card_plan",
  "name": "林小月",
  "description": "初入江湖的剑宗小师妹",
  "tags": ["少女", "剑客"],
  "personality": "天真害羞",
  "characters": [
    {"characterId": "npc_xiaoyue", "name": "林小月", "description": "剑宗小师妹，性格单纯",
     "personality": "天真害羞，说话轻声细语", "participant": true}
  ],
  "uiProfile": {
    "capabilities": {"hasSDData": true},
    "sdData": ["masterpiece, 1girl", "chinese hanfu"]
  }
}`

// legacyPlanCard 走 CharacterJSON 兼容入口：卡库中并无此卡，
// 用于验证"无卡片上下文"时的降级行为。
const legacyPlanCard = `{
  "schemaVersion": 1,
  "cardId": "card_legacy_plan",
  "name": "无名剑客",
  "characters": [{"characterId": "npc_none", "name": "无名剑客", "participant": true}]
}`

// planStack 建一个仓储 + 会话服务 + 两个会话（带卡 / 不带卡）。
func planStack(t *testing.T) (*sqlite.Store, *SessionService, string, string, string) {
	t.Helper()
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.SaveCard(&domain.CharacterCardEntry{
		CardID: "card_plan", Name: "林小月", CharacterJSON: planCard,
	}); err != nil {
		t.Fatalf("save card: %v", err)
	}
	sess := NewSessionService(st)
	withCard, err := sess.Setup(context.Background(), &SessionSetupRequest{
		Title: "演播", CardID: "card_plan", Player: Player{Name: "旅人"},
		OpeningText: "「你终于来了。」外面下着雨。",
	})
	if err != nil {
		t.Fatalf("setup with card: %v", err)
	}
	noCard, err := sess.Setup(context.Background(), &SessionSetupRequest{
		Title: "无卡", CharacterJSON: legacyPlanCard, Player: Player{Name: "旅人"},
		OpeningText: "「有人吗？」门内一片寂静。",
	})
	if err != nil {
		t.Fatalf("setup legacy: %v", err)
	}
	return st, sess, withCard.Session.SessionID, withCard.RootNode.NodeID, noCard.Session.SessionID
}

// insertPlanNode 直接插入一个带 blocks 的回合节点（计划只读取节点内容与状态投影）。
func insertPlanNode(t *testing.T, st *sqlite.Store, sessionID, parentID, contentJSON string) string {
	t.Helper()
	nodeID := "n_plan_" + time.Now().Format("150405.000000000")
	if err := st.InsertNode(&domain.PlotNode{
		NodeID: nodeID, SessionID: sessionID, ParentID: parentID,
		Kind: domain.NodeKindTurn, Depth: 1, TurnNumber: 1, SchemaVersion: 1,
		CreatedAt: time.Now(), ContentJSON: contentJSON,
	}); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	return nodeID
}

const planBlocksJSON = `{"blocks":[
  {"kind":"narration","text":"雨还在下。"},
  {"kind":"dialogue","speakerId":"npc_xiaoyue","text":"(冷峻)你终于来了。"},
  {"kind":"inner_monologue","text":"她攥紧了衣角。"},
  {"kind":"dialogue","speakerId":"npc_xiaoyue","text":"快进来吧。"}
]}`

func TestTTSPlanSegmentationBoundaries(t *testing.T) {
	st, _, sessionID, rootID, _ := planStack(t)
	svc := NewTTSPlanService(st)
	nodeID := insertPlanNode(t, st, sessionID, rootID, planBlocksJSON)

	dialogue, err := svc.Plan(TTSPlanRequest{SessionID: sessionID, NodeID: nodeID, Mode: "dialogue"})
	if err != nil {
		t.Fatalf("plan(dialogue): %v", err)
	}
	if len(dialogue.Segments) != 2 {
		t.Fatalf("session dialogue 分段数 = %d, 期望 2（仅台词块）", len(dialogue.Segments))
	}
	// 非导演模式必须剥离语气标签，否则引擎会念出 "(冷峻)"。
	if dialogue.Segments[0].Text != "你终于来了。" {
		t.Errorf("首段文本 = %q, 期望剥离括号标签", dialogue.Segments[0].Text)
	}
	if dialogue.Segments[0].SpeakerName != "林小月" {
		t.Errorf("speakerName = %q, 期望按 speakerId 还原为卡内角色名", dialogue.Segments[0].SpeakerName)
	}
	if !strings.HasPrefix(dialogue.Segments[0].SegmentID, "seg_") {
		t.Errorf("segmentId = %q", dialogue.Segments[0].SegmentID)
	}
	if !strings.HasPrefix(dialogue.Segments[0].CacheKey, "sha256:") {
		t.Errorf("cacheKey = %q", dialogue.Segments[0].CacheKey)
	}
	if dialogue.Segments[0].CacheKey == dialogue.Segments[1].CacheKey {
		t.Error("不同文本的分段必须得到不同缓存键")
	}
	if dialogue.VoiceHint.PresetID != "zh-CN-XiaoxiaoNeural" || dialogue.VoiceHint.Speed != 1.0 {
		t.Errorf("voiceHint = %+v", dialogue.VoiceHint)
	}

	full, err := svc.Plan(TTSPlanRequest{SessionID: sessionID, NodeID: nodeID, Mode: "full"})
	if err != nil {
		t.Fatalf("plan(full): %v", err)
	}
	if len(full.Segments) != 4 {
		t.Fatalf("full 分段数 = %d, 期望 4（全部块）", len(full.Segments))
	}

	// blockIndexes 只取命中的块。
	subset, err := svc.Plan(TTSPlanRequest{SessionID: sessionID, NodeID: nodeID, Mode: "full", BlockIndexes: []int{0}})
	if err != nil {
		t.Fatalf("plan(subset): %v", err)
	}
	if len(subset.Segments) != 1 || subset.Segments[0].Text != "雨还在下。" {
		t.Fatalf("blockIndexes 过滤失败: %+v", subset.Segments)
	}
}

func TestTTSPlanDirectorEnrichmentAndCacheKey(t *testing.T) {
	st, _, sessionID, rootID, _ := planStack(t)
	svc := NewTTSPlanService(st)
	nodeID := insertPlanNode(t, st, sessionID, rootID, planBlocksJSON)

	req := TTSPlanRequest{
		SessionID: sessionID, NodeID: nodeID, Mode: "dialogue",
		Director: true, Engine: "mimo", Voice: "mimo_default", Speed: 1.2,
	}
	plan, err := svc.Plan(req)
	if err != nil {
		t.Fatalf("plan(director): %v", err)
	}
	if len(plan.Segments) == 0 {
		t.Fatal("导演模式不应丢段")
	}
	// 导演指令包含场景与人设；正文不再剥离标签。
	if !strings.Contains(plan.Segments[0].ContextSummary, "剧情交谈现场") {
		t.Errorf("contextSummary = %q", plan.Segments[0].ContextSummary)
	}
	// 语速进入缓存键：同一个 segment 在 speed 不同时必须换键。
	slower := req
	slower.Speed = 1.0
	plan2, err := svc.Plan(slower)
	if err != nil {
		t.Fatalf("plan(slower): %v", err)
	}
	if plan.Segments[0].CacheKey == plan2.Segments[0].CacheKey {
		t.Error("语速不同必须得到不同缓存键（缓存键含 TTS 设置）")
	}
	if plan.VoiceHint.Speed != 1.2 {
		t.Errorf("voiceHint.speed = %v, 期望回显请求语速", plan.VoiceHint.Speed)
	}
}

func TestTTSPlanEmptyAndNoContextDegrades(t *testing.T) {
	st, _, sessionID, rootID, legacySessionID := planStack(t)
	svc := NewTTSPlanService(st)

	// 空节点（无 blocks、无开场白）：返回空分段而不是 null / 报错。
	emptyID := insertPlanNode(t, st, sessionID, rootID, `{}`)
	empty, err := svc.Plan(TTSPlanRequest{SessionID: sessionID, NodeID: emptyID})
	if err != nil {
		t.Fatalf("plan(empty): %v", err)
	}
	if empty.Segments == nil || len(empty.Segments) != 0 {
		t.Fatalf("空文本应得到空分段数组, 实得 %#v", empty.Segments)
	}

	// 无卡片上下文（卡库中无此卡）：仍给出分段，但情绪为空、场景为中性兜底、角色名占位。
	legacyRoot := "root_" + legacySessionID
	plan, err := svc.Plan(TTSPlanRequest{SessionID: legacySessionID, NodeID: legacyRoot, Mode: "dialogue"})
	if err != nil {
		t.Fatalf("plan(no-context): %v", err)
	}
	if len(plan.Segments) != 1 || plan.Segments[0].Text != "有人吗？" {
		t.Fatalf("引号对白提取失败: %+v", plan.Segments)
	}
	if plan.Segments[0].Emotion != "" {
		t.Errorf("无心境时 emotion 应为空, 实得 %q", plan.Segments[0].Emotion)
	}
	if plan.Segments[0].ContextSummary != "剧情交谈现场" {
		t.Errorf("无场景时 contextSummary = %q, 期望中性兜底", plan.Segments[0].ContextSummary)
	}
	if plan.Segments[0].SpeakerName != "当前角色" {
		t.Errorf("无卡时 speakerName = %q, 期望占位名", plan.Segments[0].SpeakerName)
	}
}

func TestTTSPlanUnknownNodeAndSessionMismatch(t *testing.T) {
	st, _, sessionID, rootID, _ := planStack(t)
	svc := NewTTSPlanService(st)

	if _, err := svc.Plan(TTSPlanRequest{NodeID: "n_missing"}); err == nil {
		t.Error("未知节点必须报错")
	}
	if _, err := svc.Plan(TTSPlanRequest{NodeID: rootID, SessionID: "s_other"}); err == nil {
		t.Error("节点与会话不匹配必须报错")
	}
	if _, err := svc.Plan(TTSPlanRequest{SessionID: sessionID}); err == nil {
		t.Error("缺少 nodeId 必须报错")
	}
}
