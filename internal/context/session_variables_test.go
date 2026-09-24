package context

import (
	"strings"
	"testing"

	"tavernagent/internal/domain"
)

func TestRenderSessionVariablesEmpty(t *testing.T) {
	if got := renderSessionVariables(nil); got != "" {
		t.Errorf("nil state = %q, want empty", got)
	}
	if got := renderSessionVariables(&domain.WorldState{}); got != "" {
		t.Errorf("no variables = %q, want empty", got)
	}
	if got := renderSessionVariables(&domain.WorldState{Variables: map[string]string{"  ": "x"}}); got != "" {
		t.Errorf("blank keys = %q, want empty", got)
	}
}

func TestRenderSessionVariablesSorted(t *testing.T) {
	state := &domain.WorldState{Variables: map[string]string{"lang": "KO", "scene": "3", "flag": "on"}}
	got := renderSessionVariables(state)
	for _, want := range []string{"【会话变量】", "- flag = on", "- lang = KO", "- scene = 3"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// 键必须排序：flag < lang < scene。
	if strings.Index(got, "- flag") > strings.Index(got, "- lang") ||
		strings.Index(got, "- lang") > strings.Index(got, "- scene") {
		t.Errorf("keys not sorted:\n%s", got)
	}
}
