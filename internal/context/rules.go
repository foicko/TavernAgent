package context

import (
	"fmt"
	"sort"
	"strings"
	"tavernagent/internal/domain"
)

func renderActionRefs(rules domain.Ruleset) string {
	if len(rules.Actions) == 0 {
		return ""
	}
	ids := make([]string, 0, len(rules.Actions))
	for id := range rules.Actions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	b.WriteString("\n【可引用的规则动作】只有玩家选定动作后才执行；前置条件由后端验证。\n")
	for _, id := range ids {
		r := rules.Actions[id]
		fmt.Fprintf(&b, "- actionRef=%s：%s（%s，DC %d）\n", id, r.Label, r.Attribute, r.DC)
	}
	return b.String()
}
