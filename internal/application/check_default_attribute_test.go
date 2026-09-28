package application

import (
	"testing"

	"tavernagent/internal/domain"
)

// M4：规则上下文里未被记录的角色/属性必须与 attributeOf 一致地回落到
// DefaultAttribute（修正 0）。此前该闭包直接返回 0，与 attributeOf 的 10 分叉，
// 同一属性在「检定结算」与「规则求值」两条路径上会得到不同的修正。
func TestRuleContextAttributeUsesBaselineDefault(t *testing.T) {
	state := domain.NewWorldState()
	state.Characters["npc"] = domain.CharacterInfo{
		CharacterID: "npc",
		Name:        "NPC",
		Attributes:  map[string]int{"strength": 14},
	}
	ctx := ruleContextOf(state)

	cases := []struct {
		name        string
		characterID string
		attribute   string
		want        int
	}{
		{"已记录属性原样返回", "npc", "strength", 14},
		{"未记录属性回落基准值", "npc", "wisdom", domain.DefaultAttribute},
		{"未记录角色回落基准值", "ghost", "strength", domain.DefaultAttribute},
		{"空属性名回落基准值", "npc", "", domain.DefaultAttribute},
	}
	for _, tc := range cases {
		if got := ctx.Attribute(tc.characterID, tc.attribute); got != tc.want {
			t.Errorf("%s: Attribute(%q,%q) = %d, want %d", tc.name, tc.characterID, tc.attribute, got, tc.want)
		}
	}
}
