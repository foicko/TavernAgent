package domain

import "testing"

// 属性修正必须是 floor((值-10)/2)，而不是整数除法（截断）。
// 低属性区间两者不同，截断会让角色凭空多得 1 点。
func TestAttributeModIsFloorNotTruncation(t *testing.T) {
	for _, tc := range []struct {
		value int
		want  int
	}{
		{1, -5}, {7, -2}, {8, -1}, {9, -1}, {10, 0},
		{11, 0}, {12, 1}, {14, 2}, {18, 4}, {20, 5},
	} {
		if got := AttributeMod(tc.value); got != tc.want {
			t.Fatalf("AttributeMod(%d) = %d, want %d", tc.value, got, tc.want)
		}
	}
}

// 判定顺序是规则的一部分：自然 20 / 自然 1 优先于总额比较。
func TestEvaluateCheckNaturalTwentyAndOneOverride(t *testing.T) {
	rule := ActionRule{ActionID: "check.strength", Attribute: "strength", DC: 30}
	// 自然 20：即使总额远低于 DC 也是大成功。
	got, err := EvaluateCheck(rule, 1, 20)
	if err != nil || got.Outcome != OutcomeCriticalSuccess {
		t.Fatalf("自然 20 未判为大成功: %+v err=%v", got, err)
	}
	// 自然 1：即使属性极高也是大失败。
	got, err = EvaluateCheck(rule, 20, 1)
	if err != nil || got.Outcome != OutcomeCriticalFailure {
		t.Fatalf("自然 1 未判为大失败: %+v err=%v", got, err)
	}
}

// 其余四档：>= DC+10 大成功、>= DC 成功、<= DC-10 大失败、其余失败。
func TestEvaluateCheckOutcomeTiers(t *testing.T) {
	// DC 12、属性 10（修正 0）：骰值即总额。
	rule := ActionRule{ActionID: "check.dexterity", Attribute: "dexterity", DC: 12}
	for _, tc := range []struct {
		natural int
		want    CheckOutcome
	}{
		{2, OutcomeCriticalFailure}, // 2 <= 12-10
		{3, OutcomeFailure},
		{11, OutcomeFailure},
		{12, OutcomeSuccess},
		{21 - 20, OutcomeFailure}, // 占位，见下
		{20, OutcomeCriticalSuccess},
	} {
		if tc.natural == 1 {
			continue
		}
		got, _ := EvaluateCheck(rule, 10, tc.natural)
		if got.Outcome != tc.want {
			t.Fatalf("骰值 %d: 得到 %s, want %s（总额 %d, DC %d）", tc.natural, got.Outcome, tc.want, got.Total, rule.DC)
		}
	}
	// 属性 18（修正 +4）时骰值 18 → 总额 22 >= 22 → 大成功。
	got, _ := EvaluateCheck(rule, 18, 18)
	if got.Outcome != OutcomeCriticalSuccess {
		t.Fatalf("总额 22 应判为大成功，得到 %s（总额 %d）", got.Outcome, got.Total)
	}
}

func TestEvaluateCheckRejectsOutOfRangeDie(t *testing.T) {
	rule := ActionRule{ActionID: "check.luck", Attribute: "luck", DC: 10}
	for _, n := range []int{0, 21, -1} {
		if _, err := EvaluateCheck(rule, 10, n); err == nil {
			t.Fatalf("骰值 %d 应被拒绝", n)
		}
	}
}

func TestRuleExprBooleanCombinators(t *testing.T) {
	ctx := RuleContext{
		HasItem:      func(id string) bool { return id == "key" },
		HasMilestone: func(id string) bool { return id == "gate_open" },
	}
	and := RuleExpr{Op: OpAnd, Children: []RuleExpr{
		{Op: OpHasItem, Arg: "key"},
		{Op: OpHasItem, Arg: "lantern"},
	}}
	if ok, err := and.Eval(ctx); err != nil || ok {
		t.Fatalf("and 应因缺少灯笼为 false: ok=%v err=%v", ok, err)
	}
	or := RuleExpr{Op: OpOr, Children: []RuleExpr{
		{Op: OpHasItem, Arg: "lantern"},
		{Op: OpHasMilestone, Arg: "gate_open"},
	}}
	if ok, err := or.Eval(ctx); err != nil || !ok {
		t.Fatalf("or 应为 true: ok=%v err=%v", ok, err)
	}
	not := RuleExpr{Op: OpNot, Children: []RuleExpr{{Op: OpHasItem, Arg: "lantern"}}}
	if ok, err := not.Eval(ctx); err != nil || !ok {
		t.Fatalf("not 应为 true: ok=%v err=%v", ok, err)
	}
}

func TestRuleExprFieldComparison(t *testing.T) {
	ctx := RuleContext{
		Relation:  func(id, field string) int { return 7 },
		Attribute: func(id, attr string) int { return 15 },
	}
	ge := RuleExpr{Op: OpCmp, Field: "affection:npc_a", Cmp: CmpGte, Value: "5"}
	if ok, err := ge.Eval(ctx); err != nil || !ok {
		t.Fatalf("关系比较: ok=%v err=%v", ok, err)
	}
	attr := RuleExpr{Op: OpCmp, Field: "attribute:npc_a/strength", Cmp: CmpLt, Value: "16"}
	if ok, err := attr.Eval(ctx); err != nil || !ok {
		t.Fatalf("属性比较: ok=%v err=%v", ok, err)
	}
}

// 白名单之外的字段必须报错，而不是静默当作 false——
// 静默 false 会让"前置条件不满足"看起来像是规则判定，掩盖配置错误。
func TestRuleExprRejectsUnknownField(t *testing.T) {
	bad := RuleExpr{Op: OpCmp, Field: "inventory.count", Cmp: CmpGt, Value: "1"}
	if _, err := bad.Eval(RuleContext{}); err == nil {
		t.Fatalf("白名单外的字段应报错")
	}
	badAttr := RuleExpr{Op: OpCmp, Field: "attribute:npc_a", Cmp: CmpGt, Value: "1"}
	if _, err := badAttr.Eval(RuleContext{}); err == nil {
		t.Fatalf("属性字段缺少键名应报错")
	}
}

// 非法算子同样必须报错。
func TestRuleExprRejectsUnknownOperator(t *testing.T) {
	if _, err := (RuleExpr{Op: RuleOp("exec")}).Eval(RuleContext{}); err == nil {
		t.Fatalf("未知算子应报错")
	}
	if _, err := (RuleExpr{Op: OpNot}).Eval(RuleContext{}); err == nil {
		t.Fatalf("not 缺子表达式应报错")
	}
}

// 深度与节点数上限：外部规则包可能是恶意构造的深层嵌套。
func TestRuleExprLimits(t *testing.T) {
	deep := RuleExpr{Op: OpNot}
	cur := &deep
	for i := 0; i < MaxRuleExprDepth+5; i++ {
		next := RuleExpr{Op: OpNot}
		cur.Children = []RuleExpr{next}
		cur = &cur.Children[0]
	}
	if _, err := deep.Eval(RuleContext{}); err == nil {
		t.Fatalf("超过最大深度应报错")
	}
	// 全树验证，不允许布尔短路绕过节点上限。
	wide := RuleExpr{Op: OpOr}
	for i := 0; i < MaxRuleExprNodes+10; i++ {
		wide.Children = append(wide.Children, RuleExpr{Op: OpHasItem, Arg: "x"})
	}
	if _, err := wide.Eval(RuleContext{}); err == nil {
		t.Fatalf("超过最大节点数应报错")
	}
}

func TestRuleExprValidatesHiddenChildren(t *testing.T) {
	for _, op := range []RuleOp{OpAnd, OpOr} {
		t.Run(string(op), func(t *testing.T) {
			expr := RuleExpr{Op: op, Children: []RuleExpr{{Op: OpHasItem, Arg: "key"}, {Op: "exec"}}}
			ctx := RuleContext{HasItem: func(string) bool { return op == OpOr }}
			if _, err := expr.Eval(ctx); err == nil {
				t.Fatal("short-circuit hid invalid child")
			}
			expr.Children = make([]RuleExpr, MaxRuleExprNodes+1)
			for i := range expr.Children {
				expr.Children[i] = RuleExpr{Op: OpHasItem, Arg: "key"}
			}
			if _, err := expr.Eval(ctx); err == nil {
				t.Fatal("short-circuit bypassed node limit")
			}
		})
	}
	if _, err := ParseRuleExpr(`{"op":"and","children":[{"op":"hasItem","arg":"missing"},{"op":"exec"}]}`); err == nil {
		t.Fatal("parse accepted an invalid rule")
	}
	if _, err := ParseRuleExpr(`{"op":"hasItem","arg":"key","children":[{"op":"exec"}]}`); err == nil {
		t.Fatal("leaf accepted hidden children")
	}
}

// rollId 的确定性是"重生成复用原检定"这一契约要求的支点。
func TestRollIDDeterministicAndScoped(t *testing.T) {
	a := RollID("head1", "check.strength", "ruleset.simplified.v1")
	b := RollID("head1", "check.strength", "ruleset.simplified.v1")
	if a != b {
		t.Fatalf("同输入应得到同一 rollId: %q vs %q", a, b)
	}
	if RollID("head2", "check.strength", "ruleset.simplified.v1") == a {
		t.Fatalf("换父节点应得到新 rollId")
	}
	if RollID("head1", "check.luck", "ruleset.simplified.v1") == a {
		t.Fatalf("换动作应得到新 rollId")
	}
	if RollID("head1", "check.strength", "ruleset.other") == a {
		t.Fatalf("换规则版本应得到新 rollId")
	}
}

// 未登记的动作就是未授权（actionRef 的授权面）。
func TestRulesetLookupIsAuthorizationSurface(t *testing.T) {
	rs := DefaultRuleset()
	if _, ok := rs.Lookup("check.strength"); !ok {
		t.Fatalf("登记过的动作应可查到")
	}
	if _, ok := rs.Lookup("model.invented.action"); ok {
		t.Fatalf("模型自造的动作不应被授权")
	}
	if _, ok := (Ruleset{}).Lookup("check.strength"); ok {
		t.Fatalf("空规则包不应授权任何动作")
	}
}

func TestCheckResultSucceeded(t *testing.T) {
	if !(CheckResult{Outcome: OutcomeSuccess}).Succeeded() {
		t.Fatalf("成功应算成功")
	}
	if !(CheckResult{Outcome: OutcomeCriticalSuccess}).Succeeded() {
		t.Fatalf("大成功应算成功")
	}
	if (CheckResult{Outcome: OutcomeFailure}).Succeeded() {
		t.Fatalf("失败不应算成功")
	}
	if (CheckResult{Outcome: OutcomeCriticalFailure}).Succeeded() {
		t.Fatalf("大失败不应算成功")
	}
}
