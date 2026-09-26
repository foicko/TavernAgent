package application

// 插画提示词计划（架构设计 §2.5）。
//
// 与 ttsplan.go 同理由：从回合正文提取画面描述、拼接风格正负向词、前置卡片 sdData，
// 都是业务规则，不属于 HTTP 协议转换，也不该由两端各写一份。
//
// 规则出处：web/src/lib/imagePromptExtractor.ts；卡片 sdData 基底并入自身实现，
// 本接口沿用其拼装顺序，见 imageBuildPrompt）。

import (
	"encoding/json"
	"regexp"
	"strings"

	"tavernagent/internal/cardmarkers"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// ImagePlanRequest 是插画提示词计划的输入。
type ImagePlanRequest struct {
	SessionID string
	NodeID    string
	BranchID  string
	// BlockIndexes 可选：只用该节点内指定序号的块（空表示全部）。
	BlockIndexes []int
	// StylePreset 是艺术风格：anime（默认）| fantasy | cinematic | realistic。
	StylePreset string
	// CharacterName 可选覆盖；为空时取卡片角色名。
	CharacterName string
}

// ImageRef 是提示词关联的卡内图片资源引用。
type ImageRef struct {
	AssetKey string `json:"assetKey"`
}

// ImageSdDataBase 是卡片自带的生图提示词模板（作为 prompt 基底的说明）。
type ImageSdDataBase struct {
	// Prompts 是去空后的模板条目。
	Prompts []string `json:"prompts"`
	// Text 是拼接后的基底文本（客户端展示用，已包含在 prompt 里）。
	Text string `json:"text"`
}

// ImagePlan 是插画提示词计划的结果。
type ImagePlan struct {
	Prompt         string          `json:"prompt"`
	NegativePrompt string          `json:"negativePrompt"`
	SdDataBase     ImageSdDataBase `json:"sdDataBase"`
	CardID         string          `json:"cardId"`
	Refs           []ImageRef      `json:"refs"`
}

// imageStyle 是一套风格的正负向词。
type imageStyle struct {
	Positive string
	Negative string
}

// imageStylePrompts 出处：imagePromptExtractor.ts:10-27（与 C# 逐字一致）。
var imageStylePrompts = map[string]imageStyle{
	"anime": {
		Positive: "masterpiece, best quality, anime visual novel CG, 16:9 wallpaper, Makoto Shinkai aesthetic, atmospheric lighting, detailed background",
		Negative: "low quality, worst quality, deformed hands, extra limbs, bad anatomy, blurry, text, watermark, signature",
	},
	"fantasy": {
		Positive: "masterpiece, dark high fantasy illustration, intricate details, dramatic rim lighting, epic scene, painterly style",
		Negative: "worst quality, low quality, modern elements, deformed, blurry, bad hands, artifacts, watermark",
	},
	"cinematic": {
		Positive: "masterpiece, 35mm film still, cinematic lighting, depth of field, atmospheric, volumetric light, 8k wallpaper",
		Negative: "anime, cartoon, sketch, worst quality, low quality, deformed, blurry, watermark",
	},
	"realistic": {
		Positive: "masterpiece, ultra realistic photography, natural soft lighting, sharp focus, 8k uhd, highly detailed texture",
		Negative: "drawing, painting, illustration, cartoon, anime, worst quality, low quality, deformed, blurry",
	},
}

// imageStyleFor 取风格配置；未知风格回退 anime（imagePromptExtractor.ts:64）。
func imageStyleFor(preset string) imageStyle {
	if s, ok := imageStylePrompts[strings.ToLower(strings.TrimSpace(preset))]; ok {
		return s
	}
	return imageStylePrompts["anime"]
}

var (
	// imageParenthesisRe 清洗全/半角括号注释（imagePromptExtractor.ts:54-55）。
	imageFullWidthParenRe = regexp.MustCompile(`（[^）]*）`)
	imageHalfWidthParenRe = regexp.MustCompile(`\([^)]*\)`)
	// imageMarkdownRe 把 Markdown 符号替换为空格（imagePromptExtractor.ts:56）。
	imageMarkdownRe = regexp.MustCompile("[*#_`]")
	imageSpaceRe    = regexp.MustCompile(`\s+`)
)

// imageExcerptMaxRunes 是画面概括的截取长度。
// 注意：imagePromptExtractor.ts:62 的注释写的是 200，但代码实际 slice(0,180)；
// 这里以**代码**为准（注释与实现不一致时按实现对齐，避免改变既有输出）。
const imageExcerptMaxRunes = 180

// imageCleanContent 清洗括号注释、心理描写与 Markdown 符号。
func imageCleanContent(content string) string {
	out := imageFullWidthParenRe.ReplaceAllString(content, "")
	out = imageHalfWidthParenRe.ReplaceAllString(out, "")
	out = imageMarkdownRe.ReplaceAllString(out, " ")
	out = imageSpaceRe.ReplaceAllString(out, " ")
	return strings.TrimSpace(out)
}

// imageExcerpt 截取前 imageExcerptMaxRunes 个字符作为精简场景概括。
func imageExcerpt(content string) string {
	runes := []rune(content)
	if len(runes) <= imageExcerptMaxRunes {
		return content
	}
	return string(runes[:imageExcerptMaxRunes])
}

// imageBuildPrompt 拼装最终 prompt。
//
// 顺序（沿用 ImagePromptExtractor.cs:42-48 的自洽实现）：
//
//	[Character: 名, ] [sdData 基底, ] excerpt, 风格正向词
//
// web 端把 sdData 基底放在 extractImagePrompt 之外再前置（IllustrationModal.tsx:46），
// 会得到 "基底, Character: 名, …" 的另一个顺序；后端取 C# 的顺序，因为它是自洽的单一实现。
func imageBuildPrompt(excerpt string, base []string, characterName, preset string) string {
	style := imageStyleFor(preset)
	prompt := style.Positive
	if excerpt != "" {
		prompt = excerpt + ", " + prompt
	}
	if baseText := imageJoinBase(base); baseText != "" {
		prompt = baseText + ", " + prompt
	}
	if name := strings.TrimSpace(characterName); name != "" {
		prompt = "Character: " + name + ", " + prompt
	}
	return prompt
}

// imageJoinBase 拼接 sdData 基底条目（去空、逗号分隔）。
// 出处：web IllustrationModal.tsx:46 的 sdData.join(", ")；C# Extract 的 string.Join(", ", basePrompts)。
func imageJoinBase(base []string) string {
	kept := make([]string, 0, len(base))
	for _, p := range base {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ", ")
}

// ImagePlanService 组装插画提示词计划。
type ImagePlanService struct {
	store ports.Store
}

// NewImagePlanService 构造插画提示词计划服务。
func NewImagePlanService(store ports.Store) *ImagePlanService {
	return &ImagePlanService{store: store}
}

// Plan 生成一次插画提示词计划。
func (s *ImagePlanService) Plan(req ImagePlanRequest) (*ImagePlan, error) {
	if strings.TrimSpace(req.NodeID) == "" {
		return nil, Err("BAD_REQUEST", "缺少 nodeId，无法确定要配图的回合", 400)
	}
	node, err := s.store.GetNode(req.NodeID)
	if err != nil || node == nil {
		return nil, Err("NOT_FOUND", "节点不存在", 404)
	}
	if req.SessionID != "" && node.SessionID != req.SessionID {
		return nil, Err("NODE_SESSION_MISMATCH", "节点不属于该会话", 400)
	}

	content, blocks := imagePlanSource(node, req.BlockIndexes)
	excerpt := imageExcerpt(imageCleanContent(content))

	cardID, cardName, sdData := s.cardContext(node.SessionID)
	// 角色名优先用调用方显式覆盖（两端 UI 都允许改）；缺省回退到卡片角色名。
	characterName := firstNonEmptyStr(strings.TrimSpace(req.CharacterName), cardName)

	plan := &ImagePlan{
		Prompt:         imageBuildPrompt(excerpt, sdData, characterName, req.StylePreset),
		NegativePrompt: imageStyleFor(req.StylePreset).Negative,
		SdDataBase:     ImageSdDataBase{Prompts: sdData, Text: imageJoinBase(sdData)},
		CardID:         cardID,
		Refs:           imagePlanRefs(blocks),
	}
	if plan.SdDataBase.Prompts == nil {
		plan.SdDataBase.Prompts = []string{}
	}
	if plan.Refs == nil {
		plan.Refs = []ImageRef{}
	}
	return plan, nil
}

// imagePlanSource 取画面描述来源：优先拼接全部块（不按 kind 过滤，web 同此），
// 无 blocks 时回退到开场白等纯文本。
// 出处：imagePromptExtractor.ts:42-49。
func imagePlanSource(node *domain.PlotNode, blockIndexes []int) (string, []domain.TextBlock) {
	if node == nil || node.ContentJSON == "" {
		return "", nil
	}
	var content struct {
		Blocks      []domain.TextBlock `json:"blocks"`
		OpeningText string             `json:"openingText"`
	}
	if err := json.Unmarshal([]byte(node.ContentJSON), &content); err != nil {
		return "", nil
	}
	allow := indexSet(blockIndexes)
	if len(content.Blocks) > 0 {
		kept := make([]domain.TextBlock, 0, len(content.Blocks))
		parts := make([]string, 0, len(content.Blocks))
		for i, b := range content.Blocks {
			if allow != nil && !allow[i] {
				continue
			}
			if strings.TrimSpace(b.Text) == "" {
				continue
			}
			kept = append(kept, b)
			parts = append(parts, b.Text)
		}
		return strings.Join(parts, " "), kept
	}
	return strings.TrimSpace(content.OpeningText), nil
}

// cardContext 取会话绑定卡的 cardId、角色名与 sdData 基底。
// sdData 随卡冻结在 UIProfile 里（cardui.go 的 CardUIProfile.SdData），
// 与 web useCardSdData / 桌面 CardAssetCache.GetSdData 同源。
func (s *ImagePlanService) cardContext(sessionID string) (cardID, characterName string, sdData []string) {
	if strings.TrimSpace(sessionID) == "" {
		return "", "", nil
	}
	sess, err := s.store.GetSession(sessionID)
	if err != nil || sess == nil || sess.CharacterID == "" {
		return "", "", nil
	}
	entry, err := s.store.GetCard(sess.CharacterID)
	if err != nil || entry == nil || entry.CharacterJSON == "" {
		return sess.CharacterID, "", nil
	}
	card, err := ParseCharacterCard(entry.CharacterJSON)
	if err != nil || card == nil {
		return sess.CharacterID, "", nil
	}
	if len(card.Characters) > 0 {
		characterName = strings.TrimSpace(card.Characters[0].Name)
	}
	if characterName == "" {
		characterName = strings.TrimSpace(card.Name)
	}
	if card.UIProfile == nil {
		return sess.CharacterID, characterName, nil
	}
	return sess.CharacterID, characterName, card.UIProfile.SdData
}

// imagePlanRefs 收集本回合正文里引用到的卡内图片资源键（去重、保持出现顺序）。
//
// 客户端此前没有 refs 概念；这里给出的是"这一回合实际展示过的图片资源"，
// 它们正是最有资格作为参考图的素材。标记解析复用 cardmarkers.SplitImages——
// 表现类标记的唯一实现（inline.go:11 的同一取舍）。
func imagePlanRefs(blocks []domain.TextBlock) []ImageRef {
	var out []ImageRef
	seen := map[string]bool{}
	for _, b := range blocks {
		for _, seg := range cardmarkers.SplitImages(b.Text) {
			if seg.Type != "image" || seg.AssetKey == "" || seen[seg.AssetKey] {
				continue
			}
			seen[seg.AssetKey] = true
			out = append(out, ImageRef{AssetKey: seg.AssetKey})
		}
	}
	return out
}
