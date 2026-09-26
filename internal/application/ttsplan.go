package application

// TTS 演播计划（架构设计 §2.4）。
//
// 为什么放在 application 而不是 tts 适配器：这里没有任何网络或音频 I/O，只有
// "哪些文本要念、用什么情绪念、上下文摘要是什么、缓存键怎么算"这些业务规则。
// tts 适配器负责与合成后端通信（合成/音色）；把导演规则塞进去，会让"换一个
// 合成后端"也被迫复核演播语义。
//
// 规则逐条来自 web/src/lib/ttsDirector.ts 与 web/src/lib/dialogueExtractor.ts；
// 每处移植都标注了出处，以免日后又漂移出第二份实现。

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// 演播计划的模式：来源文本范围（对应客户端 dialogueOnly）。
const (
	// ttsModeDialogue 只念角色台词（web 默认，TTSPlayButton 的 dialogueOnly=true）。
	ttsModeDialogue = "dialogue"
	// ttsModeFull 念全部正文（旁白 + 内心独白 + 台词）。
	ttsModeFull = "full"
)

// ---- 输出契约 ----

// TTSSegment 是一段要朗读的内容及其演播参数。
type TTSSegment struct {
	SegmentID string `json:"segmentId"`
	SpeakerID string `json:"speakerId"`
	// SpeakerName 是朗读时使用的角色名（找不到归属时回退到当前角色）。
	SpeakerName string `json:"speakerName"`
	// Text 是最终送去合成的文本：导演模式注入标签，非导演模式剥离标签。
	Text    string `json:"text"`
	Emotion string `json:"emotion"`
	// ContextSummary 是本段所依据的上下文摘要（场景/阶段/心境）。
	ContextSummary string `json:"contextSummary"`
	// CacheKey 是内容派生缓存键（含客户端 TTS 设置），两端共用同一个值。
	CacheKey string `json:"cacheKey"`
}

// VoiceHint 是演播计划给出的音色建议。
type VoiceHint struct {
	PresetID string  `json:"presetId"`
	Speed    float64 `json:"speed"`
}

// TTSPlan 是一次演播计划的完整结果。
type TTSPlan struct {
	Segments  []TTSSegment `json:"segments"`
	VoiceHint VoiceHint    `json:"voiceHint"`
}

// TTSPlanRequest 是演播计划的输入（HTTP 层已完成协议转换）。
type TTSPlanRequest struct {
	SessionID string
	NodeID    string
	// BranchID 只用于归属校验（节点本身已确定分支），可为空。
	BranchID string
	// BlockIndexes 可选：只对该节点内指定序号的块规划（空表示全部）。
	BlockIndexes []int
	// Mode 是来源文本范围：dialogue（默认）| full。
	Mode string
	// Director 为真时按 MiMo 导演模式注入演播标签与导演指令。
	Director bool
	// Engine/Voice/BaseURL/Speed 是客户端 TTS 设置：只参与缓存键计算，
	// 使两端（以及后端合成缓存）得到同一个键。
	Engine  string
	Voice   string
	BaseURL string
	Speed   float64
}

// TTSPlanContext 是演播计划所依据的上下文（从卡与状态投影中提取的纯数据）。
type TTSPlanContext struct {
	CharacterName    string
	CharacterRole    string
	CharacterPersona string
	CharacterTags    []string
	// VoiceArchetype 为空时由人设与标签动态推断（ttsDetectVoiceArchetype）。
	VoiceArchetype string
	SceneLocation  string
	SceneStage     string
	MoodCode       string
	MoodText       string
	Relationship   bool
	Affection      int
	Trust          int
	Alertness      int
	// TurnNarration 是本回合的旁白语境（ttsDirector.ts:35 的 turnNarration）。
	TurnNarration string

	// charNames 是 characterId → 角色名（用于把块的 speakerId 还原成名字）。
	charNames map[string]string
	// primaryCharID 是取心境/关系用的主角角色 ID。
	primaryCharID string
}

// TTSPlanService 组装演播计划：读取节点正文与上下文，输出结构化分段。
type TTSPlanService struct {
	store ports.Store
}

// NewTTSPlanService 构造演播计划服务。
func NewTTSPlanService(store ports.Store) *TTSPlanService {
	return &TTSPlanService{store: store}
}

// Plan 生成一次演播计划。
//
// 上下文缺失（无卡、无状态投影、无导演草稿）一律降级为"中性演播"，而不是报错：
// 演播只是阅读的辅助，缺上下文不该让"朗读这一回合"不可用。
func (s *TTSPlanService) Plan(req TTSPlanRequest) (*TTSPlan, error) {
	if strings.TrimSpace(req.NodeID) == "" {
		return nil, Err("BAD_REQUEST", "缺少 nodeId，无法确定要朗读的回合", 400)
	}
	node, err := s.store.GetNode(req.NodeID)
	if err != nil || node == nil {
		return nil, Err("NOT_FOUND", "节点不存在", 404)
	}
	if req.SessionID != "" && node.SessionID != req.SessionID {
		return nil, Err("NODE_SESSION_MISMATCH", "节点不属于该会话", 400)
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != ttsModeFull {
		mode = ttsModeDialogue
	}
	sources, narration := ttsPlanSources(node, mode, req.BlockIndexes)

	ctx := s.gatherContext(node)
	ctx.TurnNarration = narration

	var instruction string
	if req.Director {
		instruction = ttsBuildInstruction(ctx)
	}

	segments := make([]TTSSegment, 0, len(sources))
	for i, src := range sources {
		text := src.Text
		if req.Director {
			text = ttsEnrichDialogue(text, ctx)
		} else {
			// 非导演模式必须剥离标签，否则引擎会把 "(冷峻)" 念出来（ttsPlayer.ts:78）。
			text = ttsStripAudioTags(text)
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		segments = append(segments, TTSSegment{
			SegmentID:      ttsSegmentID(node.NodeID, src.BlockIndex, i),
			SpeakerID:      src.SpeakerID,
			SpeakerName:    ctx.speakerName(src.SpeakerID),
			Text:           text,
			Emotion:        ttsStyleWord(ctx),
			ContextSummary: ttsContextSummary(ctx),
			CacheKey:       ttsCacheKey(req, instruction, text),
		})
	}

	return &TTSPlan{
		Segments:  segments,
		VoiceHint: VoiceHint{PresetID: ttsArchetypePreset(ctx), Speed: ttsSpeed(req.Speed)},
	}, nil
}

// gatherContext 从会话卡库与状态投影中提取演播上下文。
// 取数口径对齐客户端：角色名/人设来自卡，场景/心境/关系来自"该节点那一刻"的状态投影，
// stage 来自导演当前节拍（web TTSButton.tsx:105-113）。
func (s *TTSPlanService) gatherContext(node *domain.PlotNode) TTSPlanContext {
	ctx := TTSPlanContext{charNames: map[string]string{}}
	if node == nil {
		return ctx
	}
	if sess, err := s.store.GetSession(node.SessionID); err == nil && sess != nil && sess.CharacterID != "" {
		if entry, cerr := s.store.GetCard(sess.CharacterID); cerr == nil && entry != nil && entry.CharacterJSON != "" {
			if card, perr := ParseCharacterCard(entry.CharacterJSON); perr == nil && card != nil {
				ctx.CharacterName = strings.TrimSpace(card.Name)
				// 身份标语用卡片描述的首个可读行（card.go 的 CardRole，与前端 cleanShortRole 同规则）。
				ctx.CharacterRole = CardRole(card.Description)
				ctx.CharacterTags = card.Tags
				if len(card.Characters) > 0 {
					c0 := card.Characters[0]
					ctx.CharacterPersona = firstNonEmptyStr(c0.Personality, c0.Description, card.Personality, card.Description)
					if role := CardRole(c0.Description); role != "" && role != "导入的角色卡" {
						ctx.CharacterRole = role
					}
					ctx.primaryCharID = c0.CharacterID
					for _, ch := range card.Characters {
						if ch.CharacterID != "" {
							ctx.charNames[ch.CharacterID] = ch.Name
						}
					}
				} else {
					ctx.CharacterPersona = firstNonEmptyStr(card.Personality, card.Description)
				}
			}
		}
	}

	if snap, serr := s.store.StateAt(node.NodeID); serr == nil && snap != nil && snap.StateJSON != "" {
		if ws, werr := domain.UnmarshalWorld(snap.StateJSON); werr == nil && ws != nil {
			if ws.Scene != nil {
				ctx.SceneLocation = firstNonEmptyStr(ws.Scene.Title, ws.Scene.LocationID)
			}
			primary := ctx.primaryCharID
			if primary == "" {
				primary = firstMapKey(ws.Moods)
			}
			if m, ok := ws.Moods[primary]; ok {
				ctx.MoodCode, ctx.MoodText = m.MoodCode, m.Text
			}
			if rel, ok := ws.Relationships[primary]; ok {
				ctx.Relationship = true
				ctx.Affection, ctx.Trust, ctx.Alertness = rel.Affection, rel.Trust, rel.Alertness
			}
		}
	}

	if ds, derr := s.store.DirectorAt(node.NodeID); derr == nil && ds != nil {
		if sum := ds.Summary(); sum != nil {
			ctx.SceneStage = strings.TrimSpace(sum.CurrentBeatTitle)
		}
	}
	return ctx
}

// speakerName 把块的 speakerId 还原为角色名；无法归属时回退到卡名，再回退到占位名。
// 回退链出处：web TTSButton.tsx:116 的 `char.name || char.shortName || "当前角色"`。
func (c TTSPlanContext) speakerName(speakerID string) string {
	if name, ok := c.charNames[speakerID]; ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if strings.TrimSpace(c.CharacterName) != "" {
		return strings.TrimSpace(c.CharacterName)
	}
	return "当前角色"
}

// ---- 切分：从哪里取要朗读的文本 ----

type ttsSource struct {
	BlockIndex int
	SpeakerID  string
	Text       string
}

// ttsPlanSources 依据节点内容切分朗读来源。
//
// 出处：
//   - 有结构化 blocks 时按 kind 精确过滤（dialogueExtractor.ts:33-55）：dialogue 模式
//     只取 dialogue 块；full 模式取全部块。
//   - 无 blocks（如根节点开场白）时回退到引号对白提取（dialogueExtractor.ts:20, 67-93）；
//     full 模式整段朗读。
//   - 旁白语境（turnNarration）始终取全部 narration 块，不受模式与 blockIndexes 影响
//     （TTSButton.tsx:108-113，用于 enrich 的 [轻叹] 判定）。
func ttsPlanSources(node *domain.PlotNode, mode string, blockIndexes []int) ([]ttsSource, string) {
	if node == nil || node.ContentJSON == "" {
		return nil, ""
	}
	var content struct {
		Blocks      []domain.TextBlock `json:"blocks"`
		OpeningText string             `json:"openingText"`
	}
	if err := json.Unmarshal([]byte(node.ContentJSON), &content); err != nil {
		return nil, ""
	}

	if len(content.Blocks) > 0 {
		allow := indexSet(blockIndexes)
		out := make([]ttsSource, 0, len(content.Blocks))
		narration := make([]string, 0, len(content.Blocks))
		for i, b := range content.Blocks {
			text := strings.TrimSpace(b.Text)
			if b.Kind == "narration" && text != "" {
				narration = append(narration, text)
			}
			if text == "" {
				continue
			}
			if allow != nil && !allow[i] {
				continue
			}
			if mode != ttsModeFull && b.Kind != "dialogue" {
				continue
			}
			out = append(out, ttsSource{BlockIndex: i, SpeakerID: b.SpeakerID, Text: text})
		}
		return out, strings.Join(narration, " ")
	}

	text := strings.TrimSpace(content.OpeningText)
	if text == "" {
		return nil, ""
	}
	if mode == ttsModeFull {
		return []ttsSource{{BlockIndex: -1, Text: text}}, ""
	}
	quotes := ttsExtractQuotes(text)
	out := make([]ttsSource, 0, len(quotes))
	for _, q := range quotes {
		out = append(out, ttsSource{BlockIndex: -1, Text: q})
	}
	return out, ""
}

// dialogueQuoteRe 匹配中英文小说常见对白引号。
// 出处：dialogueExtractor.ts:20（「」/“”/『』 与直双引号）。
var dialogueQuoteRe = regexp.MustCompile(`[「“『]([^」”』]+)[」”』]|"([^"\r\n]+)"`)

// ttsExtractQuotes 提取引号内的对白（保持出现顺序）。
func ttsExtractQuotes(text string) []string {
	var out []string
	for _, m := range dialogueQuoteRe.FindAllStringSubmatch(text, -1) {
		if v := strings.TrimSpace(firstNonEmptyStr(m[1], m[2])); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// ---- 标签剥离与演播增强（移植自 ttsDirector.ts / TtsDirector.cs）----

var (
	// ttsParenTagRe 圆括号语气标签：(冷峻)、（冷峻深沉）。出处 ttsDirector.ts:46。
	ttsParenTagRe = regexp.MustCompile(`[（(][^）)\r\n]{1,30}[）)]`)
	// ttsBracketTagRe 方括号音频标签：[停顿片刻]、【冷笑】。出处 ttsDirector.ts:48。
	ttsBracketTagRe = regexp.MustCompile(`[[【][^\]】\r\n]{1,30}[\]】]`)
	ttsDupSpaceRe   = regexp.MustCompile(`[ \t]{2,}`)

	// ttsStylePrefixPresentRe 判定句首是否已有风格括号（ttsDirector.ts:179）。
	ttsStylePrefixPresentRe = regexp.MustCompile(`^[（(][^）)\r\n]{1,15}[）)]`)
	// ttsLaughPresenceRe 判定是否存在"呵/哼"这类笑声（ttsDirector.ts:217）。
	ttsLaughPresenceRe = regexp.MustCompile(`(?:^|[，。！？\s)）])(呵|哼)(?:[，。！？\s]|$)`)
	ttsLaughRe         = regexp.MustCompile(`(呵|哼)`)
	// ttsLeadingPrefixRe 匹配句首风格括号（用于在其后插入 [轻叹]，ttsDirector.ts:225）。
	ttsLeadingPrefixRe = regexp.MustCompile(`^([（(][^）)\r\n]+[）)])`)
)

// ttsStripAudioTags 剥离导演语气标记与音频控制标签。
// 出处：ttsDirector.ts:42-52。
func ttsStripAudioTags(text string) string {
	if text == "" {
		return ""
	}
	out := ttsParenTagRe.ReplaceAllString(text, "")
	out = ttsBracketTagRe.ReplaceAllString(out, "")
	out = ttsDupSpaceRe.ReplaceAllString(out, " ")
	return strings.TrimSpace(out)
}

// ttsArchetypeRule 是声线推断规则（顺序敏感：先女后男、先少后长）。
type ttsArchetypeRule struct {
	re     *regexp.Regexp
	preset string // 对应 tts.DefaultVoices() 的 ID
	phrase string
}

// ttsArchetypeRules 出处：ttsDirector.ts:57-95（正则与文案逐字对应）。
// preset 是 DefaultVoices（tts.go）中的音色 ID，使 voiceHint 可直接引用音色目录。
var ttsArchetypeRules = []ttsArchetypeRule{
	{regexp.MustCompile(`少女|女孩|公主|小师妹|女学生|萝莉|女仆|千金|姑娘`),
		"zh-CN-XiaoxiaoNeural", "年轻女声，清脆灵动，充满少女感"},
	{regexp.MustCompile(`御姐|女王|女当家|女主人|夫人|师尊|魔女|掌门|大姐|贵妇`),
		"zh-CN-XiaoyiNeural", "成熟女声，从容优雅，声线沉着"},
	{regexp.MustCompile(`少年|男孩|王子|弟子|小哥|学弟|少侠|师弟|剑童`),
		"zh-CN-YunxiNeural", "清爽年轻男声，清澈明朗，富有朝气"},
	{regexp.MustCompile(`大叔|老者|长者|师父|骑士|将军|老管家|老爷|宗师|父亲`),
		"zh-CN-YunjianNeural", "成熟沉稳男声，浑厚庄重，从容不迫"},
	{regexp.MustCompile(`女|female|woman|she|her`),
		"zh-CN-XiaoxiaoNeural", "自然女声，咬字清晰，情感真实"},
	{regexp.MustCompile(`男|male|man|he|him`),
		"zh-CN-YunxiNeural", "自然男声，沉着清晰，发声松弛"},
}

const (
	ttsDefaultArchetypePhrase = "发声自然清晰，角色代入感强"
	ttsDefaultVoicePreset     = "zh-CN-XiaoxiaoNeural"
)

// ttsVoiceCorpus 拼接推断语料（名 + 身份 + 人设 + 标签，小写）。
// 出处：ttsDirector.ts:63-68。
func ttsVoiceCorpus(ctx TTSPlanContext) string {
	return strings.ToLower(strings.Join([]string{
		ctx.CharacterName,
		ctx.CharacterRole,
		ctx.CharacterPersona,
		strings.Join(ctx.CharacterTags, " "),
	}, " "))
}

// ttsDetectVoiceArchetype 依据人设与标签推断声线原型（短语 + 音色预设）。
func ttsDetectVoiceArchetype(ctx TTSPlanContext) (preset, phrase string) {
	corpus := ttsVoiceCorpus(ctx)
	for _, rule := range ttsArchetypeRules {
		if rule.re.MatchString(corpus) {
			return rule.preset, rule.phrase
		}
	}
	return ttsDefaultVoicePreset, ttsDefaultArchetypePhrase
}

// ttsArchetypePreset 返回建议音色预设。
// VoiceArchetype 是自由文本的声线描述（TS 里由调用方直接给短语），无法反查音色 ID，
// 因此预设始终由人设推断得出。
func ttsArchetypePreset(ctx TTSPlanContext) string {
	preset, _ := ttsDetectVoiceArchetype(ctx)
	return preset
}

// ttsArchetypePhrase 返回声线原型短语。
func ttsArchetypePhrase(ctx TTSPlanContext) string {
	if v := strings.TrimSpace(ctx.VoiceArchetype); v != "" {
		return v
	}
	_, phrase := ttsDetectVoiceArchetype(ctx)
	return phrase
}

// ttsCombinedMood 拼接心境代码与文本（代码小写）。
// 出处：ttsDirector.ts:132, 215。
func ttsCombinedMood(ctx TTSPlanContext) string {
	return strings.ToLower(ctx.MoodCode) + " " + ctx.MoodText
}

// ttsGuidanceRule 是"发声指导"规则（顺序敏感）。
type ttsGuidanceRule struct {
	re   *regexp.Regexp
	text string
}

// ttsGuidanceRules 出处：ttsDirector.ts:135-149。
var ttsGuidanceRules = []ttsGuidanceRule{
	{regexp.MustCompile(`angry|furious|rage|怒|火|恨`), "发声紧绷，语速偏快，气息沉重，字句间带有压迫感与质问的锐利度。"},
	{regexp.MustCompile(`fear|panic|nervous|shock|怕|慌|惊|惧`), "声线略带细微颤音与慌乱，呼吸浅急，语速稍显迟疑与无助，咬字轻短。"},
	{regexp.MustCompile(`sad|sorrow|grief|cry|悲|伤|绝望|痛`), "声线暗哑低沉，语速缓慢，带有隐忍的克制感与微弱叹息，情绪向下沉。"},
	{regexp.MustCompile(`happy|joy|laugh|smile|喜|乐|欣慰|笑`), "语调轻快飞扬，尾音微扬，带有真实的轻快笑意与放松舒畅，明亮生动。"},
	{regexp.MustCompile(`whisper|shy|soft|blush|羞|怯|轻柔|耳语`), "低语轻诉，贴近耳畔般的气声流淌，极具沉浸感与细腻情感张力，呼吸声真实可感。"},
	{regexp.MustCompile(`alert|guard|警|防|冷`), "音色沉着严谨，语调收敛警觉，字句清晰平稳，保持冷峻的审视距离。"},
}

const ttsDefaultGuidance = "发声通道松弛沉着，音调自然平缓，字句间留白恰当，节奏从容不怒自威。"

// ttsInstructionGuidance 依据心境（含关系张力）选出指导语。
func ttsInstructionGuidance(ctx TTSPlanContext) string {
	mood := ttsCombinedMood(ctx)
	guidance := ttsDefaultGuidance
	for _, rule := range ttsGuidanceRules {
		if rule.re.MatchString(mood) {
			guidance = rule.text
			break
		}
	}
	if ctx.Relationship {
		// 戒心优先于温情：警惕关系下不该同时表达亲昵（ttsDirector.ts:153-159）。
		if ctx.Alertness >= 60 {
			guidance += " 对对话者带有鲜明的戒心与阶级/立场距离感。"
		} else if ctx.Affection >= 50 {
			guidance += " 对对话者流露不易察觉的温情与信任，尾音微带暖意气声。"
		}
	}
	return guidance
}

// ttsBuildInstruction 生成导演指令（逐行对齐 ttsDirector.ts:100-164）。
func ttsBuildInstruction(ctx TTSPlanContext) string {
	parts := make([]string, 0, 4)

	charName := strings.TrimSpace(ctx.CharacterName)
	if charName == "" {
		charName = "说话角色"
	}
	roleText := ""
	if role := strings.TrimSpace(ctx.CharacterRole); role != "" {
		roleText = "（" + role + "）"
	}
	roleLine := "【角色】" + charName + roleText + "。"
	if persona := ttsTruncatePersona(strings.TrimSpace(ctx.CharacterPersona)); persona != "" {
		roleLine += persona + "。"
	}
	roleLine += "声线基调：" + ttsArchetypePhrase(ctx) + "。"
	parts = append(parts, roleLine)

	loc := strings.TrimSpace(ctx.SceneLocation)
	if loc == "" {
		loc = "剧情交谈现场"
	}
	sceneDesc := loc
	if stage := strings.TrimSpace(ctx.SceneStage); stage != "" {
		sceneDesc += "，阶段：" + stage
	}
	parts = append(parts, "【场景】"+sceneDesc+"。")

	if mood := strings.TrimSpace(ctx.MoodText); mood != "" {
		parts = append(parts, "【心境】"+mood+"。")
	}
	parts = append(parts, "【指导】"+ttsInstructionGuidance(ctx))
	return strings.Join(parts, "\n")
}

// ttsTruncatePersona 长人设截断为 48 字 + 省略号（ttsDirector.ts:108-111，那里按
// JS 字符串长度 50 判定；中文按字符计，这里用 rune 数）。
func ttsTruncatePersona(persona string) string {
	if persona == "" {
		return ""
	}
	runes := []rune(persona)
	if len(runes) > 50 {
		return string(runes[:48]) + "…"
	}
	return persona
}

// ttsContextSummary 输出本段所依据的上下文摘要（场景/阶段/心境）。
// 出处：ttsDirector.ts:117-126 的场景行与心境行。
func ttsContextSummary(ctx TTSPlanContext) string {
	loc := strings.TrimSpace(ctx.SceneLocation)
	if loc == "" {
		loc = "剧情交谈现场"
	}
	parts := []string{loc}
	if stage := strings.TrimSpace(ctx.SceneStage); stage != "" {
		parts = append(parts, "阶段："+stage)
	}
	if mood := strings.TrimSpace(ctx.MoodText); mood != "" {
		parts = append(parts, "心境："+mood)
	}
	return strings.Join(parts, "，")
}

// ttsStyleWord 返回与心境相符的风格词（不带括号），空串表示无明确情绪。
// 出处：ttsDirector.ts:186-205 的前缀推断（同一梯级，供 emotion 字段与注入共用）。
func ttsStyleWord(ctx TTSPlanContext) string {
	mood := ttsCombinedMood(ctx)
	for _, rule := range ttsStylePrefixRules {
		if rule.re.MatchString(mood) {
			return rule.word
		}
	}
	moodText := ctx.MoodText
	if n := len([]rune(moodText)); n > 0 && n <= 4 {
		return moodText
	}
	if strings.ToLower(ctx.MoodCode) == "calm" {
		return "平缓沉着"
	}
	return ""
}

// ttsStylePrefixRules 出处：ttsDirector.ts:186-199。
var ttsStylePrefixRules = []struct {
	re   *regexp.Regexp
	word string
}{
	{regexp.MustCompile(`angry|furious|怒|恨`), "愤懑质问"},
	{regexp.MustCompile(`fear|panic|怕|慌|惊`), "慌乱不安"},
	{regexp.MustCompile(`sad|sorrow|悲|伤|痛`), "哽咽低沉"},
	{regexp.MustCompile(`happy|joy|喜|乐|欣慰`), "欣喜轻快"},
	{regexp.MustCompile(`shy|blush|羞|怯`), "羞赧轻声"},
	{regexp.MustCompile(`whisper|耳语`), "低语轻柔"},
	{regexp.MustCompile(`alert|guard|警|防`), "严肃警觉"},
}

// ttsEnrichDialogue 注入与心境相符的风格前缀与音频标签。
// 出处：ttsDirector.ts:169-237。
func ttsEnrichDialogue(text string, ctx TTSPlanContext) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	out := trimmed

	// 1. 句首无风格括号时按心境补前缀。
	if !ttsStylePrefixPresentRe.MatchString(out) {
		if word := ttsStyleWord(ctx); word != "" {
			out = "(" + word + ")" + out
		}
	}

	// 2. 笑声/叹息动作标签。
	mood := ttsCombinedMood(ctx)
	if ttsLaughPresenceRe.MatchString(out) {
		if containsAny(mood, "happy", "joy", "smile", "喜", "乐") {
			out = replaceFirstMatch(ttsLaughRe, "[轻笑]$1", out)
		} else {
			out = replaceFirstMatch(ttsLaughRe, "[冷哼]$1", out)
		}
	} else if containsAny(mood, "sad", "sorrow", "whisper", "叹", "疲", "累") ||
		strings.Contains(ctx.TurnNarration, "叹") {
		if !strings.Contains(out, "[轻叹]") && !strings.Contains(out, "[深呼吸]") {
			out = ttsLeadingPrefixRe.ReplaceAllString(out, "$1[轻叹]")
		}
	}

	// 3. 首个省略号增强为停顿（只增强一处，保持自然留白）。
	return ttsEnhanceEllipsis(out)
}

// ttsEnhanceEllipsis 把第一个 "……" 增强为 "……[停顿片刻]"。
// 出处：ttsDirector.ts:229-234。
func ttsEnhanceEllipsis(s string) string {
	if !strings.Contains(s, "……") {
		return s
	}
	var b strings.Builder
	count := 0
	for {
		i := strings.Index(s, "……")
		if i < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		count++
		if count == 1 {
			b.WriteString("……[停顿片刻]")
		} else {
			b.WriteString("……")
		}
		s = s[i+len("……"):]
	}
	return b.String()
}

// ---- 缓存键与段标识 ----

// ttsCacheKey 生成内容派生的缓存键。
//
// 出处：web/src/lib/ttsPlayer.ts:90 用同一元组
// `engine:voice:speed:baseUrl:instruction:text` 作为缓存键；后端 tts.go:166 的合成
// 缓存也以该元组作前缀（那里用 md5）。本接口按架构设计 §2.4 返回 "sha256:<hex>"。
// 速度统一按 %.2f 格式化，与 tts.go 的合成缓存口径一致。
func ttsCacheKey(req TTSPlanRequest, instruction, text string) string {
	raw := fmt.Sprintf("%s:%s:%.2f:%s:%s:%s",
		req.Engine, req.Voice, ttsSpeed(req.Speed), req.BaseURL, instruction, text)
	sum := sha256.Sum256([]byte(raw))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ttsSegmentID 生成稳定的分段标识（同一节点、同一块、同一序号恒得同一 ID）。
func ttsSegmentID(nodeID string, blockIndex, seq int) string {
	sum := sha256.Sum256([]byte(nodeID + ":" + strconv.Itoa(blockIndex) + ":" + strconv.Itoa(seq)))
	return "seg_" + hex.EncodeToString(sum[:8])
}

// ttsSpeed 归一化语速（<=0 视为默认 1.0，与 tts.go:161-164 一致）。
func ttsSpeed(speed float64) float64 {
	if speed <= 0 {
		return 1.0
	}
	return speed
}

// ---- 小工具 ----

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// replaceFirstMatch 只替换首个匹配（Go 的 Regexp 没有"替换 N 次"，手工拼接）。
func replaceFirstMatch(re *regexp.Regexp, repl, s string) string {
	loc := re.FindStringSubmatchIndex(s)
	if loc == nil {
		return s
	}
	expanded := re.ExpandString(nil, repl, s, loc)
	return s[:loc[0]] + string(expanded) + s[loc[1]:]
}

// indexSet 把块序号列表转成集合；空列表视为"不过滤"（nil 表示全部）。
func indexSet(indexes []int) map[int]bool {
	if len(indexes) == 0 {
		return nil
	}
	out := make(map[int]bool, len(indexes))
	for _, i := range indexes {
		out[i] = true
	}
	return out
}

// firstMapKey 返回 map 的最小键（用于无卡时确定性地挑一个角色）。
func firstMapKey[V any](m map[string]V) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys[0]
}
