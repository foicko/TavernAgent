// 会话变量事件的应用。
//
// 与物品/社交/导演事件分离：变量不参与规则判定，只承载卡片的会话级键值状态
// （语言、场景编号、开关等）。空值表示删除该键，避免留下无意义的空串。
package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// isVarEvent 判定事件是否属于变量域（与下面 applier 的 case 集合必须一致）。
func isVarEvent(t EventType) bool {
	return t == EventVarSet
}

// applyVarEvent 应用变量事件，返回值契约与 ApplyEvent 相同。
func applyVarEvent(s *WorldState, ev *DomainEvent) (string, error) {
	var p VarPayload
	if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
		return "", err
	}
	key := strings.TrimSpace(p.Key)
	if key == "" {
		return "", fmt.Errorf("variable key required")
	}
	if p.Value == "" {
		delete(s.Variables, key)
		return "var:" + key, nil
	}
	if s.Variables == nil {
		s.Variables = map[string]string{}
	}
	s.Variables[key] = p.Value
	return "var:" + key, nil
}
