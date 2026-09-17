package context

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Validate element structure, not substrings that could occur in comments or
// CDATA. Only a meaningful, complete summary may replace source history.
func validateSummaryXML(raw string) error {
	decoder := xml.NewDecoder(strings.NewReader(raw))
	var stack []string
	sections := map[string]int{}
	var narrative strings.Builder
	rootSeen := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("摘要 XML 无效: %w", err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			name := token.Name.Local
			if token.Name.Space != "" {
				return errors.New("摘要不允许命名空间")
			}
			if len(stack) == 0 {
				if rootSeen || name != "story_checkpoint" {
					return errors.New("摘要必须只有一个 story_checkpoint 根元素")
				}
				rootSeen = true
			} else {
				parent := stack[len(stack)-1]
				allowed := parent == "story_checkpoint" && (name == "narrative_arc" || name == "character_dynamics" || name == "open_loops" || name == "milestones")
				allowed = allowed || parent == "character_dynamics" && (name == "mindset" || name == "hidden_tension")
				if !allowed {
					return errors.New("摘要 XML 含未授权层级")
				}
				if parent == "story_checkpoint" {
					sections[name]++
					if sections[name] > 1 {
						return errors.New("摘要章节不能重复")
					}
				}
			}
			for _, attr := range token.Attr {
				if name != "mindset" || attr.Name.Local != "character" || attr.Name.Space != "" {
					return errors.New("摘要 XML 含未授权属性")
				}
			}
			stack = append(stack, name)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 2 && stack[1] == "narrative_arc" {
				narrative.Write(token)
			}
			if len(stack) == 0 && strings.TrimSpace(string(token)) != "" {
				return errors.New("摘要根元素之外不能有文本")
			}
		case xml.Directive, xml.ProcInst:
			return errors.New("摘要禁止 XML 指令")
		}
	}
	if !rootSeen || len(stack) != 0 || sections["narrative_arc"] != 1 || sections["open_loops"] != 1 || strings.TrimSpace(narrative.String()) == "" {
		return errors.New("摘要必须包含非空 narrative_arc 和 open_loops 章节")
	}
	return nil
}
