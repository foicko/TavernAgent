package application

import (
	"context"
	"encoding/json"
	"time"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/util/id"
)

// Player 是玩家身份配置（开局向导：姓名 + 身份倾向 + 起始背包）。
type Player struct {
	Name     string                `json:"name"`
	Role     string                `json:"role,omitempty"` // 身份/倾向描述（如"流浪剑客"）
	Backpack []domain.ItemInstance `json:"backpack,omitempty"`
}

// SessionSetupRequest 是创建会话的输入。
type SessionSetupRequest struct {
	IdempotencyKey   string
	Title            string
	CharacterJSON    string
	Player           Player
	OpeningVariantID string // 选择的备选开场（缺省回退第一个变体）
	OpeningText      string // 显式开场文本（兼容旧接口；不为空时优先）
}

// playerSnapshot 是玩家身份快照（T20：根节点记录角色/玩家/世界书/规则模板版本快照）。
type playerSnapshot struct {
	Name     string                `json:"name"`
	Role     string                `json:"role,omitempty"`
	Backpack []domain.ItemInstance `json:"backpack,omitempty"`
}

// SetupResult 是创建会话的输出。
type SetupResult struct {
	Session     *domain.Session
	Branch      *domain.Branch
	RootNode    *domain.PlotNode
	State       *domain.WorldState
	OpeningText string
}

// SessionService 负责配置与开局。
type SessionService struct {
	store ports.SessionDeps
}

func NewSessionService(store ports.Store) *SessionService { return &SessionService{store: store} }

// Setup 创建会话：解析角色卡 → 确定玩家身份与开场 → 构建初始世界状态 →
// 创建根节点（含模板版本快照 T20）+ 主分支 + 角色模板版本。
func (s *SessionService) Setup(ctx context.Context, req *SessionSetupRequest) (*SetupResult, error) {
	sessionID, requestHash, prior, err := s.beginSetup(ctx, req)
	if err != nil || prior != nil {
		return prior, err
	}
	card, resolved, err := parseSetupCard(req)
	if err != nil {
		return nil, err
	}
	// 派生 ID 必须使用完整 sessionID：UUIDv7 的前 8 位是毫秒时间高位，
	// 同窗口内创建的两个会话会得到相同前缀，进而撞 plot_nodes.node_id 主键
	// （表现为第二次创建会话 503）。契约 §2：外部 ID 不透明，不依赖其可读性。
	rootID := "root_" + sessionID
	title := req.Title
	if title == "" {
		title = card.Name
	}

	// 玩家身份快照（进入根节点的模板版本快照）。
	// 缺省称呼与前端 DEFAULT_PLAYER_NAME（web/src/lib/characterMacros.ts）保持同值：
	// 界面不再自行兜底称呼，两边不一致会让"没填名字"显示出两个不同的词。
	ps := playerSnapshot{Name: "旅人", Role: req.Player.Role, Backpack: req.Player.Backpack}
	if req.Player.Name != "" {
		ps.Name = req.Player.Name
	}
	playerInfo := domain.CharacterInfo{CharacterID: "player", Name: ps.Name, Participant: true}

	// 角色卡宏展开：{{char}}/{{user}} 是卡片生态的占位符，必须在这里换成实际称呼。
	// 展开点选在双方称呼都已确定的会话创建，之后进入世界状态、开场正文、
	// 世界书与提示词的就都是可读文本。
	card.ExpandMacros(ps.Name)
	resolved.Text = expandCardMacros(resolved.Text, card.Name, ps.Name)

	state, err := card.BuildInitialState(playerInfo, req.Player.Backpack, resolved)
	if err != nil {
		return nil, Err("CARD_INVALID", "初始世界状态非法: "+err.Error(), 422)
	}
	stateJSON, _ := state.Marshal()

	tplID := id.New()
	tpl := &domain.TemplateVersion{
		TemplateVersionID: tplID, Kind: domain.TemplateCharacter, SchemaVersion: 1,
		Content: req.CharacterJSON, ContentHash: hashString(req.CharacterJSON),
	}
	psJSON, _ := json.Marshal(ps)
	templateSnapshot, tpls := buildTemplateSnapshot(card, tpl, psJSON)

	rootContent, _ := json.Marshal(map[string]any{
		"setupRequestHash": requestHash,
		"stateVersion":     "v1",
		"initialState":     json.RawMessage(stateJSON),
		"openingText":      resolved.Text,
		"openingVariantId": resolved.VariantID,
		"cardRef":          tplID,
		"templates":        templateSnapshot,
	})
	root := &domain.PlotNode{
		NodeID: rootID, SessionID: sessionID, Kind: domain.NodeKindRoot,
		SchemaVersion: 1, ContentJSON: string(rootContent),
	}
	sess := &domain.Session{
		SessionID: sessionID, RootNodeID: rootID, Title: title,
		CreatedAt: time.Now(), RulesetVersion: RulesetVersion,
		// M4l：会话归属角色卡的稳定 ID（契约 §11.2.1 双向硬绑定）。
		// normalizeCard 保证 CardID 非空——自定义卡由内容哈希派生，同一张卡
		// 重复导入得到同一 ID，"按角色列会话"与受理校验都依赖这一稳定性。
		CharacterID: card.CardID,
	}
	if card.Rules != nil {
		sess.RulesetVersion = card.Rules.Version
	}
	branch := &domain.Branch{BranchID: "branch_main_" + sessionID, SessionID: sessionID, Name: "main", HeadNodeID: rootID, Version: 0}

	sn := &domain.StateSnapshot{NodeID: rootID, SnapshotVersion: 1, RulesetVersion: sess.RulesetVersion, StateJSON: stateJSON, StateHash: state.HashID()}
	if err := s.store.CreateSessionWithSnapshot(sess, root, branch, tpls, sn); err != nil {
		// Concurrent retries can race between lookup and insert. Re-read the
		// immutable creation receipt after a duplicate/ambiguous storage result.
		if requestHash != "" {
			if prior, lookupErr := s.findSetup(sessionID, requestHash); lookupErr != nil || prior != nil {
				return prior, lookupErr
			}
		}
		return nil, Err("STORAGE_UNAVAILABLE", "创建会话失败: "+err.Error(), 503)
	}
	return &SetupResult{Session: sess, Branch: branch, RootNode: root, State: state, OpeningText: resolved.Text}, nil
}

// beginSetup 校验开局请求并处理幂等：命中既有会话时直接返回它的结果，
// 调用方拿到非 nil 的 prior 时应当原样返回。
func (s *SessionService) beginSetup(ctx context.Context, req *SessionSetupRequest) (string, string, *SetupResult, error) {
	if req == nil {
		return "", "", nil, Err("BAD_REQUEST", "缺少开局请求", 400)
	}
	if err := ctx.Err(); err != nil {
		return "", "", nil, err
	}
	if len(req.IdempotencyKey) > 128 {
		return "", "", nil, Err("BAD_REQUEST", "幂等键不能超过 128 字节", 400)
	}
	if req.IdempotencyKey == "" {
		return id.New(), "", nil, nil
	}
	sessionID := "session_" + hashString(req.IdempotencyKey)
	body, _ := json.Marshal(req)
	requestHash := hashString(string(body))
	prior, err := s.findSetup(sessionID, requestHash)
	return sessionID, requestHash, prior, err
}

// parseSetupCard 解析角色卡并做全部入口校验：
// 规则包、秘密揭示条件与开场变体都要在创建会话时就判定——
// 非法配置不该等到用户写了半天剧情才炸。
func parseSetupCard(req *SessionSetupRequest) (*CharacterCard, *ResolvedOpening, error) {
	card, err := ParseCharacterCard(req.CharacterJSON)
	if err != nil {
		return nil, nil, Err("CARD_INVALID", "角色卡解析失败: "+err.Error(), 422)
	}
	if card.Rules != nil {
		if err := domain.ValidateRuleset(*card.Rules); err != nil {
			return nil, nil, Err("CARD_INVALID", "规则包无效: "+err.Error(), 422)
		}
	}
	// 空条件合法（只能由显式配置事件解锁）。
	for _, sec := range card.Secrets {
		if _, err := domain.ParseRuleExpr(sec.RevealWhen); err != nil {
			return nil, nil, Err("CARD_INVALID", "秘密「"+sec.Title+"」的揭示条件非法: "+err.Error(), 422)
		}
	}
	resolved, err := card.ResolveOpening(req.OpeningVariantID, req.OpeningText)
	if err != nil {
		return nil, nil, Err("CARD_NO_OPENING", "角色卡未提供任何开场变体: "+err.Error(), 422)
	}
	if req.OpeningVariantID != "" && req.OpeningText == "" && resolved.VariantID != req.OpeningVariantID {
		return nil, nil, Err("CARD_VARIANT_NOT_FOUND", "指定的开场变体不存在: "+req.OpeningVariantID, 422)
	}
	return card, resolved, nil
}

// buildTemplateSnapshot 组装 T20 模板版本快照（角色/玩家/世界书/规则四类，
// 缺失项记为 null），并返回需要一并落库的模板版本行。
// 世界书：卡片声明了 LorebookRefs 时落一条 lorebook 模板版本，并把引用写入根节点，
// 使「引用」可追溯（内容注入见 M3 世界书链路）。
func buildTemplateSnapshot(card *CharacterCard, tpl *domain.TemplateVersion, psJSON []byte) (map[string]any, []*domain.TemplateVersion) {
	snapshot := map[string]any{
		"character": map[string]any{"templateVersionId": tpl.TemplateVersionID, "schemaVersion": tpl.SchemaVersion, "contentHash": tpl.ContentHash},
		"player":    map[string]any{"schemaVersion": 1, "contentHash": hashString(string(psJSON)), "content": json.RawMessage(psJSON)},
		"lorebook":  nil,
		"rules":     nil,
	}
	tpls := []*domain.TemplateVersion{tpl}
	if card.Rules != nil {
		raw, _ := json.Marshal(card.Rules)
		rulesTpl := &domain.TemplateVersion{TemplateVersionID: id.New(), Kind: domain.TemplateRules, SchemaVersion: 1, Content: string(raw), ContentHash: hashString(string(raw))}
		tpls = append(tpls, rulesTpl)
		snapshot["rules"] = map[string]any{"templateVersionId": rulesTpl.TemplateVersionID, "schemaVersion": 1, "contentHash": rulesTpl.ContentHash}
	}
	if len(card.LorebookRefs) > 0 {
		lbJSON, _ := json.Marshal(card.LorebookRefs)
		lbTpl := &domain.TemplateVersion{
			TemplateVersionID: id.New(), Kind: domain.TemplateLorebook, SchemaVersion: 1,
			Content: string(lbJSON), ContentHash: hashString(string(lbJSON)),
		}
		tpls = append(tpls, lbTpl)
		snapshot["lorebook"] = map[string]any{
			"templateVersionId": lbTpl.TemplateVersionID, "schemaVersion": lbTpl.SchemaVersion,
			"contentHash": lbTpl.ContentHash, "refs": json.RawMessage(lbJSON),
		}
	}
	return snapshot, tpls
}
