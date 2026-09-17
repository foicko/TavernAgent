package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// 本文件是技术契约 §5.2 的规则部分：受限规则表达式、规则包与检定判定。
// 全部是**纯函数**——随机骰值由调用方传入，规则层自己不掷骰、不读库、不联网。
// 这样同一份规则在同一输入下必然得到同一结果，可被测试与重放。

// ---- 受限规则表达式 ----
//
// 契约 §5.2 要求「规则 DSL 使用受限表达式或声明式 AST，只支持字段比较、
// 布尔组合、hasItem、hasMilestone 等白名单函数；禁止 eval、脚本执行、
// 文件读取和无限递归」。
//
// 这里选择**声明式 AST + 显式求值**，而不是解析表达式字符串：
//   - 没有解析器就没有注入面，"eval" 无从谈起；
//   - 树结构天然无环，不存在"无限递归"的可能；
//   - 代价是规则包不能用手写字符串表达，但这正是要的限制。

// RuleOp 是规则表达式的算子。
type RuleOp string

const (
	OpAnd          RuleOp = "and"
	OpOr           RuleOp = "or"
	OpNot          RuleOp = "not"
	OpCmp          RuleOp = "cmp"
	OpHasItem      RuleOp = "hasItem"
	OpHasMilestone RuleOp = "hasMilestone"
	OpHasSecret    RuleOp = "hasSecret"
)

// CmpOp 是字段比较运算符。
type CmpOp string

const (
	CmpEq  CmpOp = "eq"
	CmpNe  CmpOp = "ne"
	CmpGt  CmpOp = "gt"
	CmpGte CmpOp = "gte"
	CmpLt  CmpOp = "lt"
	CmpLte CmpOp = "lte"
)

// 求值上限。树结构本身无环，但规则包可能来自外部（导入的剧情包），
// 因此把深度与节点数都钉死在可预期范围内，避免一份恶意规则拖垮一次提交。
const (
	MaxRuleExprDepth = 12
	MaxRuleExprNodes = 256
)

// 可比较字段的白名单。规则**只能**引用这些字段，
// 其它字段名一律报错（不是静默当 false——静默会让前置条件被误判为不满足，
// 而反过来误判为满足更危险：它会放行不该放行的动作）。
const (
	FieldPrefixAffection = "affection:"
	FieldPrefixTrust     = "trust:"
	FieldPrefixAlertness = "alertness:"
	FieldPrefixAttribute = "attribute:" // attribute:<characterId>/<attributeKey>
	FieldSceneLocation   = "scene.location"
)

// RuleContext 是规则求值能看到的全部事实。
//
// 刻意做成窄视图：规则只能"读"这些，不能改状态、读文件或联网。
// 由调用方从 WorldState 构造——规则层不认识 WorldState 的具体结构，
// 因此世界状态演进不会牵动规则求值。
type RuleContext struct {
	// Relation 返回角色某维度（affection/trust/alertness）的关系值。
	Relation func(characterID, field string) int
	// Attribute 返回角色的属性值；未记录应返回 0，规则层不替它猜默认值。
	Attribute     func(characterID, attribute string) int
	HasItem       func(itemID string) bool
	HasMilestone  func(milestoneID string) bool
	HasSecret     func(secretID string) bool
	SceneLocation string
}

// RuleExpr 是规则表达式节点。字段按算子语义使用，未使用的留空。
type RuleExpr struct {
	Op    RuleOp `json:"op"`
	Field string `json:"field,omitempty"`
	Cmp   CmpOp  `json:"cmp,omitempty"`
	Value string `json:"value,omitempty"`
	Arg   string `json:"arg,omitempty"`
	// Children 是布尔组合（and/or/not）的子表达式。
	Children []RuleExpr `json:"children,omitempty"`
}

// Eval 求值规则表达式。
//
// 规则本身非法时**返回错误**而不是当作 false：让调用方明确失败并说明理由，
// 好过默默把动作拒掉或悄悄放行。
func (e RuleExpr) Eval(ctx RuleContext) (bool, error) {
	used := 0
	return evalRule(e, ctx, 1, &used)
}

func evalRule(e RuleExpr, ctx RuleContext, depth int, used *int) (bool, error) {
	if depth > MaxRuleExprDepth {
		return false, fmt.Errorf("规则表达式超过最大深度 %d", MaxRuleExprDepth)
	}
	*used++
	if *used > MaxRuleExprNodes {
		return false, fmt.Errorf("规则表达式超过最大节点数 %d", MaxRuleExprNodes)
	}
	if len(e.Children) > 0 && e.Op != OpAnd && e.Op != OpOr && e.Op != OpNot {
		return false, fmt.Errorf("算子 %q 不能包含子表达式", e.Op)
	}
	switch e.Op {
	case OpAnd:
		if len(e.Children) == 0 {
			return false, errors.New("and 至少需要一个子表达式")
		}
		result := true
		for _, child := range e.Children {
			ok, err := evalRule(child, ctx, depth+1, used)
			if err != nil {
				return false, err
			}
			result = result && ok
		}
		return result, nil
	case OpOr:
		if len(e.Children) == 0 {
			return false, errors.New("or 至少需要一个子表达式")
		}
		result := false
		for _, child := range e.Children {
			ok, err := evalRule(child, ctx, depth+1, used)
			if err != nil {
				return false, err
			}
			result = result || ok
		}
		return result, nil
	case OpNot:
		if len(e.Children) != 1 {
			return false, fmt.Errorf("not 需要恰好一个子表达式，得到 %d", len(e.Children))
		}
		ok, err := evalRule(e.Children[0], ctx, depth+1, used)
		if err != nil {
			return false, err
		}
		return !ok, nil
	case OpHasItem:
		if e.Arg == "" {
			return false, errors.New("hasItem 需要 arg（物品 ID）")
		}
		if ctx.HasItem == nil {
			return false, nil
		}
		return ctx.HasItem(e.Arg), nil
	case OpHasMilestone:
		if e.Arg == "" {
			return false, errors.New("hasMilestone 需要 arg（里程碑 ID）")
		}
		if ctx.HasMilestone == nil {
			return false, nil
		}
		return ctx.HasMilestone(e.Arg), nil
	case OpHasSecret:
		if e.Arg == "" {
			return false, errors.New("hasSecret 需要 arg（秘密 ID）")
		}
		if ctx.HasSecret == nil {
			return false, nil
		}
		return ctx.HasSecret(e.Arg), nil
	case OpCmp:
		return evalCmp(e, ctx)
	default:
		return false, fmt.Errorf("未知规则算子 %q", e.Op)
	}
}

func evalCmp(e RuleExpr, ctx RuleContext) (bool, error) {
	if e.Field == "" {
		return false, errors.New("cmp 需要 field")
	}
	// 场景字段没有数值语义，只按字符串比较。
	if e.Field == FieldSceneLocation {
		return cmpText(ctx.SceneLocation, e.Cmp, e.Value)
	}
	lhs, err := resolveRuleField(ctx, e.Field)
	if err != nil {
		return false, err
	}
	rhs, err := strconv.Atoi(strings.TrimSpace(e.Value))
	if err != nil {
		return false, fmt.Errorf("字段 %q 的比较值必须是整数，得到 %q", e.Field, e.Value)
	}
	switch e.Cmp {
	case CmpEq:
		return lhs == rhs, nil
	case CmpNe:
		return lhs != rhs, nil
	case CmpGt:
		return lhs > rhs, nil
	case CmpGte:
		return lhs >= rhs, nil
	case CmpLt:
		return lhs < rhs, nil
	case CmpLte:
		return lhs <= rhs, nil
	default:
		return false, fmt.Errorf("未知比较运算符 %q", e.Cmp)
	}
}

func cmpText(lhs string, op CmpOp, rhs string) (bool, error) {
	switch op {
	case CmpEq:
		return lhs == rhs, nil
	case CmpNe:
		return lhs != rhs, nil
	default:
		return false, fmt.Errorf("字段 %q 只支持 eq/ne 比较，得到 %q", FieldSceneLocation, op)
	}
}

func resolveRuleField(ctx RuleContext, field string) (int, error) {
	switch {
	case strings.HasPrefix(field, FieldPrefixAffection),
		strings.HasPrefix(field, FieldPrefixTrust),
		strings.HasPrefix(field, FieldPrefixAlertness):
		prefix, id, _ := strings.Cut(field, ":")
		if id == "" {
			return 0, fmt.Errorf("关系字段 %q 缺少角色 ID", field)
		}
		if ctx.Relation == nil {
			return 0, nil
		}
		return ctx.Relation(id, strings.TrimSuffix(prefix, ":")), nil
	case strings.HasPrefix(field, FieldPrefixAttribute):
		id, key, ok := strings.Cut(strings.TrimPrefix(field, FieldPrefixAttribute), "/")
		if !ok || id == "" || key == "" {
			return 0, fmt.Errorf("属性字段 %q 应形如 attribute:<characterId>/<key>", field)
		}
		if ctx.Attribute == nil {
			return 0, nil
		}
		return ctx.Attribute(id, key), nil
	default:
		return 0, fmt.Errorf("规则引用了白名单之外的字段 %q", field)
	}
}

// ParseRuleExpr 从 JSON 字符串解析规则表达式。
//
// 卡片里的 RevealWhen 是字符串字段，但规则本身是**声明式 AST**——
// 所以它存的是 AST 的 JSON 编码（例如 {"op":"hasItem","arg":"key"}），
// 而不是表达式文本。这样既不用写表达式解析器（没有解析器就没有注入面），
// 又能保持卡片字段向后兼容（旧卡片没有该字段）。
//
// 空串返回 nil（表示无条件），非法 JSON 或未知算子返回错误——
// 在导入/建会话时就拒绝，而不是等到运行时才炸。
func ParseRuleExpr(s string) (*RuleExpr, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out RuleExpr
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("规则表达式不是合法 JSON: %w", err)
	}
	// 立即试求值一次空上下文，把未知算子/缺字段挡在入口。
	if _, err := out.Eval(RuleContext{}); err != nil {
		return nil, fmt.Errorf("规则表达式非法: %w", err)
	}
	return &out, nil
}

// ---- 规则包与检定 ----

// ActionRule 是一个被允许的动作（技术契约 §5.2）。
//
// DC 与属性来自这里，**不能来自模型提议**：契约明确要求
// "模型生成的选项属性不能直接成为规则"。
type ActionRule struct {
	ActionID string `json:"actionId"`
	Label    string `json:"label,omitempty"`
	// Attribute 是本检定使用的属性键（如 strength / dexterity / wisdom / charisma）。
	Attribute string `json:"attribute"`
	DC        int    `json:"dc"`
	// Requires 是可选前置条件；nil 表示无条件允许。
	Requires *RuleExpr `json:"requires,omitempty"`
	// Consequences are authored in the ruleset, never accepted from a model.
	Consequences     map[CheckOutcome][]RuleEffect `json:"consequences,omitempty"`
	PermanentEffects map[CheckOutcome]string       `json:"permanentEffects,omitempty"`
}

type RuleEffect struct {
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Ruleset 是一套可被后端独立校验的规则。
type Ruleset struct {
	Version string                `json:"version"`
	Actions map[string]ActionRule `json:"actions"`
}

// Lookup 返回动作规则。未登记的动作就是未授权——这是 actionRef 的授权面。
func (r Ruleset) Lookup(actionID string) (ActionRule, bool) {
	if r.Actions == nil {
		return ActionRule{}, false
	}
	rule, ok := r.Actions[actionID]
	return rule, ok
}

// DefaultAttribute 是角色未记录某属性时的取值。
// 取 10 是因为契约的属性修正以 10 为基准（修正 0），即"未经训练"。
const DefaultAttribute = 10

// DefaultRuleset 返回首个简化规则包（契约 §5.2）。
//
// 明确它是自定义规则，**不等同于任何现成 TRPG 系统**。动作用
// "check.<属性>" 命名而不是具体剧情动作，是因为首版还没把卡片里的动作表
// 接进来；动作表可从规则包扩展，语义不变。
func DefaultRuleset() Ruleset {
	return Ruleset{
		Version: "ruleset.simplified.v1",
		Actions: map[string]ActionRule{
			"check.strength":  {ActionID: "check.strength", Label: "力量检定", Attribute: "strength", DC: 12},
			"check.dexterity": {ActionID: "check.dexterity", Label: "敏捷检定", Attribute: "dexterity", DC: 12},
			"check.wisdom":    {ActionID: "check.wisdom", Label: "察觉检定", Attribute: "wisdom", DC: 12},
			"check.charisma":  {ActionID: "check.charisma", Label: "交涉检定", Attribute: "charisma", DC: 12},
			"check.luck":      {ActionID: "check.luck", Label: "运气检定", Attribute: "luck", DC: 10},
		},
	}
}

// CheckOutcome 是检定结果。
type CheckOutcome string

const (
	OutcomeCriticalSuccess CheckOutcome = "critical_success"
	OutcomeSuccess         CheckOutcome = "success"
	OutcomeFailure         CheckOutcome = "failure"
	OutcomeCriticalFailure CheckOutcome = "critical_failure"
)

// CheckResult 是一次检定的完整记录（契约 §5.2 要求记录的字段）。
type CheckResult struct {
	RollID          string       `json:"rollId"`
	ActionID        string       `json:"actionId"`
	Attribute       string       `json:"attribute"`
	AttributeVal    int          `json:"attributeValue"`
	AttributeMod    int          `json:"attributeModifier"`
	Natural         int          `json:"natural"` // 原始 d20 骰值
	Total           int          `json:"total"`   // 总值 = 骰值 + 属性修正
	DC              int          `json:"dc"`
	Outcome         CheckOutcome `json:"outcome"`
	RulesetVer      string       `json:"rulesetVersion"`
	PermanentEffect string       `json:"permanentEffect,omitempty"`
	Effects         []RuleEffect `json:"effects,omitempty"`
}

// Succeeded 判定是否为成功（含大成功）。
func (r CheckResult) Succeeded() bool {
	return r.Outcome == OutcomeSuccess || r.Outcome == OutcomeCriticalSuccess
}

// AttributeMod 是契约给出的属性修正：floor((属性值 - 10) / 2)。
//
// 不能用整数除法直接代替 floor：Go 的整数除法对负数是**截断**，
// 而 floor 是向下取整。(1-10)/2 = -4.5 → floor 是 -5，截断是 -4。
// 低于 10 的部分单独处理，避免低属性角色凭空多得 1 点。
func AttributeMod(value int) int {
	if value < 10 {
		return -((10 - value + 1) / 2)
	}
	return (value - 10) / 2
}

// EvaluateCheck 按契约 §5.2 的顺序判定一次检定。
//
// 判定顺序是规则的一部分，不能重排：
//
//	自然 20 → 大成功
//	否则自然 1 → 大失败
//	否则总值 >= DC+10 → 大成功
//	否则总值 >= DC → 成功
//	否则总值 <= DC-10 → 大失败
//	其余 → 失败
//
// natural 由调用方给出（随机源在应用层），因此本函数是纯函数：
// 同样的规则、属性与骰值必然得到同样的结果——重生成复用结果靠的就是这一点。
func EvaluateCheck(rule ActionRule, attributeValue, natural int) (CheckResult, error) {
	if natural < 1 || natural > 20 {
		return CheckResult{}, fmt.Errorf("d20 骰值越界: %d", natural)
	}
	mod := AttributeMod(attributeValue)
	total := natural + mod
	var outcome CheckOutcome
	switch {
	case natural == 20:
		outcome = OutcomeCriticalSuccess
	case natural == 1:
		outcome = OutcomeCriticalFailure
	case total >= rule.DC+10:
		outcome = OutcomeCriticalSuccess
	case total >= rule.DC:
		outcome = OutcomeSuccess
	case total <= rule.DC-10:
		outcome = OutcomeCriticalFailure
	default:
		outcome = OutcomeFailure
	}
	return CheckResult{
		ActionID:        rule.ActionID,
		PermanentEffect: rule.PermanentEffects[outcome],
		Effects:         append([]RuleEffect(nil), rule.Consequences[outcome]...),
		Attribute:       rule.Attribute,
		AttributeVal:    attributeValue,
		AttributeMod:    mod,
		Natural:         natural,
		Total:           total,
		DC:              rule.DC,
		Outcome:         outcome,
	}, nil
}

// RollID 是行动实例的标识。
//
// 契约 §5.2：文字重生成时，若基准父节点、动作内容和规则版本完全相同，
// 则复用原 rollId。把 rollId 定义成这三者的确定性函数，复用就不必依赖
// "去找上一次掷骰的结果"这类额外机制——同样的输入自然得到同样的 ID。
//
// 反过来说：只有父节点推进了或换了动作，才会得到新 ID，因此
// "文字重生成不会重掷骰"这条要求由标识本身保证，而不是靠调用方记得。
func RollID(baseHeadID, actionID, rulesetVersion string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{baseHeadID, actionID, rulesetVersion}, "|")))
	return "roll_" + hex.EncodeToString(sum[:])[:16]
}

// errNoCheckResult 表示收据里没有检定结果（例如它是纯物品动作）。
var errNoCheckResult = errors.New("收据中没有检定结果")

// ---- 秘密与条件揭示（技术契约 §5.2，M4d）----

// SecretDef 是一条秘密/世界观条目的定义（来自卡片）。
//
// Content 在揭示前**不得**进入模型上下文与玩家可见文本——这是 T25 的核心。
// RevealWhen 满足后产生 secret_unlock 事件，正文最早在**下一次**符合
// 可见性条件的上下文出现（不是当轮：本轮编译发生在解锁判定之前）。
type SecretDef struct {
	SecretID string `json:"secretId"`
	Title    string `json:"title,omitempty"`
	Content  string `json:"content"`
	// RevealWhen 是揭示条件；nil 表示无法用规则自动判定
	//（只能由后续的显式配置事件解锁）。
	RevealWhen *RuleExpr `json:"revealWhen,omitempty"`
	Order      int       `json:"order,omitempty"`
}

// SecretUnlockPayload 是秘密解锁事件的负载。
type SecretUnlockPayload struct {
	SecretID string `json:"secretId"`
	Title    string `json:"title,omitempty"`
}

// UnlockSecret 把秘密标记为已解锁（幂等）。
//
// 重复解锁（重放、同一轮多个来源）不产生重复条目也不报错：
// 解锁是"状态从未揭示变为已揭示"的一次性转移，重复到达是正常现象。
func (s *WorldState) UnlockSecret(secretID string) {
	if secretID == "" {
		return
	}
	for _, id := range s.UnlockedSecrets {
		if id == secretID {
			return
		}
	}
	s.UnlockedSecrets = append(s.UnlockedSecrets, secretID)
}

// IsSecretUnlocked 判定秘密是否已解锁。
func (s *WorldState) IsSecretUnlocked(secretID string) bool {
	if secretID == "" {
		return false
	}
	for _, id := range s.UnlockedSecrets {
		if id == secretID {
			return true
		}
	}
	return false
}
