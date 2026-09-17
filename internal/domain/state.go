package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"math/big"
	"slices"
	"sort"
	"strings"
)

// ---- 关系数值常量（设计 §7） ----

const (
	AffectionMin = -100
	AffectionMax = 100
	ScaleMin     = 0
	ScaleMax     = 100
	// MaxDeltaPerTurnRole 同一回合、同一角色、同一维度的聚合上限。
	MaxDeltaPerTurnRole = 10
)

// RelationshipField 是角色对玩家的可变化维度。
type RelationshipField string

const (
	FieldAffection RelationshipField = "affection"
	FieldTrust     RelationshipField = "trust"
	FieldAlertness RelationshipField = "alertness"
)

// RelationValue 保存一个角色对玩家的关系数值。
type RelationValue struct {
	Affection int `json:"affection"`
	Trust     int `json:"trust"`
	Alertness int `json:"alertness"`
}

// Bounds 返回给定字段的合法数值范围。
func (f RelationshipField) Bounds() (min, max int) {
	switch f {
	case FieldAffection:
		return AffectionMin, AffectionMax
	default:
		return ScaleMin, ScaleMax
	}
}

// ---- 物品 ----

// ItemInstance 是稳定 ID 标识的物品实例。
type ItemInstance struct {
	InstanceID string   `json:"instanceId"`
	TemplateID string   `json:"templateId"`
	Name       string   `json:"name"`
	Aliases    []string `json:"aliases,omitempty"`
	OwnerID    string   `json:"ownerId,omitempty"` // 为空表示无主/地点持有
	LocationID string   `json:"locationId,omitempty"`
	Quantity   int      `json:"quantity"`   // 非负整数
	Protection int      `json:"protection"` // 保护等级，越高越难失去
	Keepsake   bool     `json:"keepsake"`   // 是否占用信物槽（默认最多 3 件）
	Unique     bool     `json:"unique"`
}

// ---- 承诺 ----

// PromiseState 是承诺状态。
type PromiseState string

const (
	PromiseProposed  PromiseState = "proposed"
	PromiseActive    PromiseState = "active"
	PromiseFulfilled PromiseState = "fulfilled"
	PromiseBroken    PromiseState = "broken"
	PromiseCancelled PromiseState = "cancelled"
)

// Promise 记录参与者、内容、来源、状态。
type Promise struct {
	PromiseID      string       `json:"promiseId"`
	ParticipantIDs []string     `json:"participantIds"`
	Content        string       `json:"content"`
	SourceNodeID   string       `json:"sourceNodeId"`
	State          PromiseState `json:"state"`
	SettledByRec   string       `json:"settledByReceiptId,omitempty"` // 避免反复结算
}

// CharacterMood 是提交后驱动的情绪状态。
type CharacterMood struct {
	MoodCode string `json:"moodCode"`
	Text     string `json:"text"`
}

// Goal 是角色短期目标。
type Goal struct {
	Text        string `json:"text"`
	CharacterID string `json:"characterId"`
}

// Scene 是当前场景。
type Scene struct {
	SceneID    string `json:"sceneId"`
	LocationID string `json:"locationId,omitempty"`
	Title      string `json:"title,omitempty"`
}

// WorldState 是世界状态投影：世界事实 + 角色状态。
// 角色认知（记忆）单独存储；叙事表现属于节点正文。
type WorldState struct {
	Relationships   map[string]RelationValue `json:"relationships"` // key: characterId
	Items           map[string]ItemInstance  `json:"items"`         // key: instanceId
	Promises        map[string]Promise       `json:"promises"`      // key: promiseId
	Moods           map[string]CharacterMood `json:"moods"`         // key: characterId
	Goals           map[string]Goal          `json:"goals"`         // key: goalId
	Scene           *Scene                   `json:"scene,omitempty"`
	Milestones      map[string]string        `json:"milestones"` // key: milestoneId -> 描述
	UnlockedSecrets []string                 `json:"unlockedSecrets,omitempty"`
	Characters      map[string]CharacterInfo `json:"characters"` // key: characterId
}

// CharacterInfo 是角色的静态信息（初版最小集）。
type CharacterInfo struct {
	CharacterID string `json:"characterId"`
	Name        string `json:"name"`
	// Aliases 是同一角色的其它称呼（技术契约 §8.1「实体维护正式名和别名」）。
	// 卡片 JSON 里写了 aliases 就会带进来（角色结构直接取自卡片的该字段）。
	// 检索侧用它扩展实体命中：正文里只写别称时也要能定位到这个角色，
	// 否则「按别名问起的那个人」的相关记忆会整条漏召回。
	Aliases     []string `json:"aliases,omitempty"`
	Description string   `json:"description"`
	Participant bool     `json:"participant"`
	// Attributes 是**规则包**用的属性值（如 strength/dexterity/wisdom/charisma），
	// 不是人设字段。来源只能是规则包或配置事件：契约 §5.2 明确要求
	// "模型生成的选项属性不能直接成为规则"。
	// 未记录的属性在检定时按 domain.DefaultAttribute 处理（修正 0）。
	Attributes map[string]int `json:"attributes,omitempty"`
	Avatar     string         `json:"avatar,omitempty"`
	// V2 / V3 扩展结构化人设字段（可选，用于档案展示与详情透视）
	Personality  string   `json:"personality,omitempty"`
	Scenario     string   `json:"scenario,omitempty"`
	MesExample   string   `json:"mes_example,omitempty"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
	PostHistory  string   `json:"post_history_instructions,omitempty"`
	CreatorNotes string   `json:"creator_notes,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Creator      string   `json:"creator,omitempty"`
	Version      string   `json:"character_version,omitempty"`
	Nickname     string   `json:"nickname,omitempty"`
}

// SearchTerms 返回该角色的全部可检索称呼：正式名 + 别名（去空、去重、保持顺序）。
// 索引侧与查询侧共用同一条规则，避免两侧对「什么算这个角色」判断不一致。
func (c CharacterInfo) SearchTerms() []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(c.Aliases)+1)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[strings.ToLower(v)] {
			return
		}
		seen[strings.ToLower(v)] = true
		out = append(out, v)
	}
	add(c.Name)
	for _, a := range c.Aliases {
		add(a)
	}
	return out
}

// NewWorldState 构造空世界状态。
func NewWorldState() *WorldState {
	return &WorldState{
		Relationships: map[string]RelationValue{},
		Items:         map[string]ItemInstance{},
		Promises:      map[string]Promise{},
		Moods:         map[string]CharacterMood{},
		Goals:         map[string]Goal{},
		Milestones:    map[string]string{},
		Characters:    map[string]CharacterInfo{},
	}
}

// Clone 深度拷贝世界状态。
func (s *WorldState) Clone() *WorldState {
	c := NewWorldState()
	if s.Scene != nil {
		scene := *s.Scene
		c.Scene = &scene
	}
	for k, v := range s.Relationships {
		c.Relationships[k] = v
	}
	for k, v := range s.Items {
		it := v
		it.Aliases = slices.Clone(v.Aliases)
		c.Items[k] = it
	}
	for k, v := range s.Promises {
		p := v
		p.ParticipantIDs = slices.Clone(v.ParticipantIDs)
		c.Promises[k] = p
	}
	for k, v := range s.Moods {
		c.Moods[k] = v
	}
	for k, v := range s.Goals {
		c.Goals[k] = v
	}
	for k, v := range s.Milestones {
		c.Milestones[k] = v
	}
	c.UnlockedSecrets = append(c.UnlockedSecrets, s.UnlockedSecrets...)
	for k, v := range s.Characters {
		v.Aliases = slices.Clone(v.Aliases)
		v.Attributes = maps.Clone(v.Attributes)
		c.Characters[k] = v
	}
	return c
}

// HashID 输出世界状态的规范哈希（重放等价校验）。
func (s *WorldState) HashID() string {
	canonical := s.Clone()
	sort.Strings(canonical.UnlockedSecrets)
	canonical.UnlockedSecrets = slices.Compact(canonical.UnlockedSecrets)
	// encoding/json sorts map keys. Include every authoritative field, including
	// attributes, item protection, mood text and provenance references.
	raw, _ := json.Marshal(canonical)
	sum := sha256.Sum256(raw)
	return "v2:" + hex.EncodeToString(sum[:])
}

// MatchesHash 报告 value 是否与该状态的指纹一致。
//
// 指纹只有一种格式：HashID() 产出的 "v2:<sha256>"。它由当前代码计算，
// 因此可以严格比对——不一致即说明快照内容与指纹确实不同步。
func (s *WorldState) MatchesHash(value string) bool {
	return value == s.HashID()
}

// Marshal 序列化世界状态（权威快照内容）。
func (s *WorldState) Marshal() (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}

// UnmarshalWorld 反序列化世界状态。
func UnmarshalWorld(data string) (*WorldState, error) {
	s := NewWorldState()
	if err := json.Unmarshal([]byte(data), s); err != nil {
		return nil, err
	}
	return s.Clone(), nil
}

// String 保留旧版稳定文本；仅用于旧快照指纹校验与日志。
// 完整状态的确定性比较应使用 HashID。
func (s *WorldState) String() string {
	rel := make([]string, 0, len(s.Relationships))
	for k := range s.Relationships {
		rel = append(rel, k)
	}
	sort.Strings(rel)
	var b strings.Builder
	for _, k := range rel {
		v := s.Relationships[k]
		fmt.Fprintf(&b, "rel:%s=%d/%d/%d;", k, v.Affection, v.Trust, v.Alertness)
	}
	items := make([]string, 0, len(s.Items))
	for k := range s.Items {
		items = append(items, k)
	}
	sort.Strings(items)
	for _, k := range items {
		it := s.Items[k]
		fmt.Fprintf(&b, "item:%s@%s=%d;", k, it.OwnerID, it.Quantity)
	}
	promises := make([]string, 0, len(s.Promises))
	for k := range s.Promises {
		promises = append(promises, k)
	}
	sort.Strings(promises)
	for _, k := range promises {
		p := s.Promises[k]
		fmt.Fprintf(&b, "promise:%s=%s;", k, p.State)
	}
	moods := make([]string, 0, len(s.Moods))
	for k := range s.Moods {
		moods = append(moods, k)
	}
	sort.Strings(moods)
	for _, k := range moods {
		m := s.Moods[k]
		fmt.Fprintf(&b, "mood:%s=%s;", k, m.MoodCode)
	}
	goals := make([]string, 0, len(s.Goals))
	for k := range s.Goals {
		goals = append(goals, k)
	}
	sort.Strings(goals)
	for _, k := range goals {
		g := s.Goals[k]
		fmt.Fprintf(&b, "goal:%s@%s=%s;", k, g.CharacterID, g.Text)
	}
	milestones := make([]string, 0, len(s.Milestones))
	for k := range s.Milestones {
		milestones = append(milestones, k)
	}
	sort.Strings(milestones)
	for _, k := range milestones {
		fmt.Fprintf(&b, "milestone:%s=%s;", k, s.Milestones[k])
	}
	if len(s.UnlockedSecrets) > 0 {
		secs := append([]string(nil), s.UnlockedSecrets...)
		sort.Strings(secs)
		for _, sec := range secs {
			fmt.Fprintf(&b, "secret:%s;", sec)
		}
	}
	if s.Scene != nil {
		fmt.Fprintf(&b, "scene:%s@%s;", s.Scene.SceneID, s.Scene.LocationID)
	}
	return b.String()
}

// ---- 关系增量规则（契约 §5.1） ----

// ApplyRelationshipDelta 在副本上应用合法聚合后的增量。
// 对基准值 v 与聚合增量 d：
//   - d>0：增长不超过全局最大值，也不超过 max(0, stageCap-v)；
//   - d<0：减少不越过全局最小值；阶段上限不影响正常减少。
func (s *WorldState) ApplyRelationshipDelta(charID string, field RelationshipField, d, globalCap, stageCap int) (applied int, err error) {
	cur, ok := s.Relationships[charID]
	if !ok {
		cur = RelationValue{}
	}
	minV, maxV := field.Bounds()
	switch field {
	case FieldAffection:
		limit := cur.Affection + d
		if d > 0 {
			cap := maxV
			if globalCap >= minV && globalCap < cap {
				cap = globalCap
			}
			if stageCap >= minV && stageCap < cap {
				cap = stageCap
			}
			// 只限制增长：cap - cur 可以为负则不允许增长
			grow := maxInt(0, cap-cur.Affection)
			limit = cur.Affection + minInt(d, grow)
		} else if d < 0 {
			limit = cur.Affection + maxInt(d, minV-cur.Affection)
		}
		applied = limit - cur.Affection
		cur.Affection = limit
	case FieldTrust:
		limit := cur.Trust + d
		if d > 0 {
			grow := maxInt(0, maxV-cur.Trust)
			limit = cur.Trust + minInt(d, grow)
		} else if d < 0 {
			limit = cur.Trust + maxInt(d, minV-cur.Trust)
		}
		applied = limit - cur.Trust
		cur.Trust = limit
	case FieldAlertness:
		limit := cur.Alertness + d
		if d > 0 {
			grow := maxInt(0, maxV-cur.Alertness)
			limit = cur.Alertness + minInt(d, grow)
		} else if d < 0 {
			limit = cur.Alertness + maxInt(d, minV-cur.Alertness)
		}
		applied = limit - cur.Alertness
		cur.Alertness = limit
	default:
		return 0, fmt.Errorf("unknown relationship field %q", field)
	}
	if field == FieldAffection {
		if cur.Affection > maxV || cur.Affection < minV {
			return 0, fmt.Errorf("affection out of range: %d", cur.Affection)
		}
	}
	s.Relationships[charID] = cur
	return applied, nil
}

// AggregateDeltas 同轮同角色同维度聚合，限制 |sum| <= MaxDeltaPerTurnRole。
func AggregateDeltas(deltas []int) int {
	// Clamp only after the exact sum: saturating intermediate values would lose
	// cancellation, and native int addition could reverse the sign on overflow.
	var sum, value big.Int
	for _, d := range deltas {
		sum.Add(&sum, value.SetInt64(int64(d)))
	}
	if sum.Cmp(value.SetInt64(MaxDeltaPerTurnRole)) > 0 {
		return MaxDeltaPerTurnRole
	}
	if sum.Cmp(value.SetInt64(-MaxDeltaPerTurnRole)) < 0 {
		return -MaxDeltaPerTurnRole
	}
	return int(sum.Int64())
}

// ---- 物品规则（契约 §5.1 / C07 / C08） ----

// ValidateItemTransfer 校验转移：数量守恒、来源拥有、目标合法。
// 不修改状态；返回错误说明拒绝原因。
func (s *WorldState) ValidateItemTransfer(itemID, fromOwner, toOwner string, qty int) error {
	if err := s.validateItemQuantity(itemID, fromOwner, qty); err != nil {
		return err
	}
	it := s.Items[itemID]
	if qty != it.Quantity {
		return fmt.Errorf("partial instance transfer is unsupported; transfer all %d units or define separate instances", it.Quantity)
	}
	if toOwner != "" && toOwner != "player" {
		if _, ok := s.Characters[toOwner]; !ok {
			return fmt.Errorf("unknown item recipient %q", toOwner)
		}
	}
	if toOwner != "" && toOwner != fromOwner {
		return s.ValidateKeepsakeSlot(toOwner, it.Keepsake)
	}
	return nil
}

func (s *WorldState) validateItemQuantity(itemID, fromOwner string, qty int) error {
	if qty <= 0 {
		return fmt.Errorf("transfer quantity must be positive, got %d", qty)
	}
	it, ok := s.Items[itemID]
	if !ok {
		return fmt.Errorf("item %q does not exist", itemID)
	}
	if it.OwnerID != fromOwner {
		return fmt.Errorf("item %q is owned by %q, not %q", itemID, it.OwnerID, fromOwner)
	}
	if it.Quantity < qty {
		return fmt.Errorf("item %q has quantity %d < %d", itemID, it.Quantity, qty)
	}
	// 受保护物品不能从无主地点被取走——占位规则，后续由规则包细化。
	_ = it.Protection
	return nil
}

// ApplyItemTransfer 执行已校验的转移（唯一实例整体易主；数量守恒 C07）。
// 可堆叠消耗品在 M2 以"分实例"建模，避免单行被拆碎。
func (s *WorldState) ApplyItemTransfer(itemID, toOwner string, _ int) {
	it := s.Items[itemID]
	it.OwnerID = toOwner
	it.LocationID = ""
	if toOwner == "" && s.Scene != nil {
		it.LocationID = s.Scene.LocationID
	}
	s.Items[itemID] = it
}

// ApplyItemGrant 从授权来源事件新增物品（新增必须由允许来源授权，C08）。
func (s *WorldState) ApplyItemGrant(it ItemInstance) error {
	if strings.TrimSpace(it.InstanceID) == "" {
		return fmt.Errorf("item instance id required")
	}
	if it.Quantity < 0 {
		return fmt.Errorf("grant quantity must be non-negative")
	}
	if _, exists := s.Items[it.InstanceID]; exists {
		return fmt.Errorf("item %q already exists", it.InstanceID)
	}
	if it.Protection < 0 || (it.Unique && it.Quantity > 1) {
		return fmt.Errorf("invalid item protection or unique quantity")
	}
	if it.OwnerID != "" && it.OwnerID != "player" {
		if _, ok := s.Characters[it.OwnerID]; !ok {
			return fmt.Errorf("unknown item owner %q", it.OwnerID)
		}
	}
	if it.OwnerID != "" && it.Quantity > 0 {
		if err := s.ValidateKeepsakeSlot(it.OwnerID, it.Keepsake); err != nil {
			return err
		}
	}
	it.Aliases = slices.Clone(it.Aliases)
	s.Items[it.InstanceID] = it
	return nil
}

// KeepsakeCount 统计占用信物槽的数量。
func (s *WorldState) KeepsakeCount(ownerID string) int {
	n := 0
	for _, it := range s.Items {
		if it.Keepsake && it.Quantity > 0 && it.OwnerID == ownerID {
			n++
		}
	}
	return n
}

// ValidateKeepsakeSlot 校验信物槽（默认最多 3 件）。
func (s *WorldState) ValidateKeepsakeSlot(ownerID string, newKeepsake bool) error {
	if newKeepsake && s.KeepsakeCount(ownerID) >= 3 {
		return fmt.Errorf("keepsake slot limit (3) reached for %q", ownerID)
	}
	return nil
}

// ---- 承诺规则（契约 §5.1） ----

// SettlePromise 结算承诺：校验其未在一事件中重复结算。
func SettlePromise(p *Promise, target PromiseState, settleReceipt string) error {
	if p.SettledByRec != "" {
		return fmt.Errorf("promise %q already settled by receipt %q", p.PromiseID, p.SettledByRec)
	}
	p.State = target
	p.SettledByRec = settleReceipt
	return nil
}

// ---- 事件投影 ----

// RelationshipDeltaPayload 是 relationship_delta 事件负载。
type RelationshipDeltaPayload struct {
	CharacterID string `json:"characterId"`
	Field       string `json:"field"`
	Delta       int    `json:"delta"`
	Applied     int    `json:"applied"`
}

// ItemTransferPayload 是 item_transfer 事件负载。
type ItemTransferPayload struct {
	ItemID   string `json:"itemId"`
	From     string `json:"from"`
	To       string `json:"to"`
	Quantity int    `json:"quantity"`
}

// ItemGrantPayload 是 item_grant 事件负载。
type ItemGrantPayload struct {
	Item ItemInstance `json:"item"`
}

// PromiseProposePayload 是 promise_propose 事件负载。
type PromiseProposePayload struct {
	Promise Promise `json:"promise"`
}

type PromiseSettlePayload struct {
	PromiseID string       `json:"promiseId"`
	State     PromiseState `json:"state"`
	ReceiptID string       `json:"receiptId,omitempty"`
}

type ScenePayload struct {
	Scene Scene `json:"scene"`
}
type MilestonePayload struct {
	MilestoneID string `json:"milestoneId"`
	Description string `json:"description"`
}

// MemoryAddPayload 是 memory_add 事件负载。
type MemoryAddPayload struct {
	Memory MemoryRecord `json:"memory"`
}

// applier 将事件负载应用到状态副本。
// 返回已应用事件的紧凑记录（用于 hash 与重放等价验证）。
func ApplyEvent(s *WorldState, ev *DomainEvent) (string, error) {
	switch ev.Type {
	case EventRelationshipDelta:
		var p RelationshipDeltaPayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		_, err := s.ApplyRelationshipDelta(p.CharacterID, RelationshipField(p.Field), p.Applied, AffectionMax, AffectionMax)
		return fmt.Sprintf("rel:%s/%s/%d", p.CharacterID, p.Field, p.Applied), err
	case EventItemTransfer:
		var p ItemTransferPayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		if err := s.ValidateItemTransfer(p.ItemID, p.From, p.To, p.Quantity); err != nil {
			return "", err
		}
		s.ApplyItemTransfer(p.ItemID, p.To, p.Quantity)
		return fmt.Sprintf("transfer:%s->%s,%d", p.From, p.To, p.Quantity), nil
	case EventItemConsume:
		var p ItemTransferPayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		if p.To != "" && p.To != "consumed" {
			return "", fmt.Errorf("consumption cannot transfer ownership")
		}
		if err := s.validateItemQuantity(p.ItemID, p.From, p.Quantity); err != nil {
			return "", err
		}
		it := s.Items[p.ItemID]
		it.Quantity -= p.Quantity
		if it.Quantity == 0 {
			it.OwnerID, it.LocationID = "consumed", ""
		}
		s.Items[p.ItemID] = it
		return fmt.Sprintf("consume:%s,%d", p.ItemID, p.Quantity), nil
	case EventItemGrant:
		var p ItemGrantPayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		if err := s.ApplyItemGrant(p.Item); err != nil {
			return "", err
		}
		return fmt.Sprintf("grant:%s", p.Item.InstanceID), nil
	case EventPromisePropose:
		var p PromiseProposePayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		if _, exists := s.Promises[p.Promise.PromiseID]; exists {
			return "", fmt.Errorf("promise %q already exists", p.Promise.PromiseID)
		}
		s.Promises[p.Promise.PromiseID] = p.Promise
		return fmt.Sprintf("promise:%s", p.Promise.PromiseID), nil
	case EventPromiseSettle:
		var p PromiseSettlePayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		promise, ok := s.Promises[p.PromiseID]
		if !ok {
			return "", fmt.Errorf("unknown promise %s", p.PromiseID)
		}
		if promise.State != PromiseProposed && promise.State != PromiseActive {
			return "", fmt.Errorf("promise already settled")
		}
		if p.State != PromiseActive && p.State != PromiseFulfilled && p.State != PromiseBroken && p.State != PromiseCancelled {
			return "", fmt.Errorf("invalid promise transition")
		}
		promise.State, promise.SettledByRec = p.State, p.ReceiptID
		s.Promises[p.PromiseID] = promise
		return "promise-settle", nil
	case EventScenePropose:
		var p ScenePayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		if p.Scene.SceneID == "" {
			return "", fmt.Errorf("scene id required")
		}
		s.Scene = &p.Scene
		return "scene", nil
	case EventMilestonePropose:
		var p MilestonePayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		if p.MilestoneID == "" || strings.TrimSpace(p.Description) == "" {
			return "", fmt.Errorf("milestone id and description required")
		}
		s.Milestones[p.MilestoneID] = p.Description
		return "milestone", nil
	case EventMoodSet:
		var p struct {
			CharacterID string        `json:"characterId"`
			Mood        CharacterMood `json:"mood"`
		}
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		s.Moods[p.CharacterID] = p.Mood
		return "mood", nil
	case EventMemoryAdd, EventMemoryRevised, EventMemoryPinned, EventMemoryHidden:
		return "memory", nil // 记忆不进入世界状态投影（C04：认知与状态分离）
	case EventDirectorChange:
		return "director", nil // Intentions have their own projection; world hashes stay unchanged.
	case EventGoalSet:
		var p struct {
			GoalID      string `json:"goalId"`
			CharacterID string `json:"characterId"`
			Text        string `json:"text"`
		}
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		if _, ok := s.Characters[p.CharacterID]; !ok {
			return "", fmt.Errorf("unknown goal character")
		}
		if p.GoalID == "" {
			p.GoalID = "goal_" + p.CharacterID
		}
		s.Goals[p.GoalID] = Goal{CharacterID: p.CharacterID, Text: p.Text}
		return "goal", nil
	case EventSecretUnlock:
		var p SecretUnlockPayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			return "", err
		}
		s.UnlockSecret(p.SecretID)
		return "secret:" + p.SecretID, nil
	default:
		return "", fmt.Errorf("event %q has no applier", ev.Type)
	}
}

// ApplyEvents 在副本上顺序应用事件，返回每事件的效果标签。
func ApplyEvents(s *WorldState, evs []*DomainEvent) ([]string, error) {
	labels := make([]string, 0, len(evs))
	for _, ev := range evs {
		label, err := ApplyEvent(s, ev)
		if err != nil {
			return labels, err
		}
		labels = append(labels, label)
	}
	return labels, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
