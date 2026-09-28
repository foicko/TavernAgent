package context_test

// 精简测试后保留的共享脚手架：秘密不泄漏与承重摘要不变量两组用例依赖它。
import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

const (
	testSessionID = "sess_test"
	testRootID    = "root_test"
	testBranchID  = "branch_test"

	testPlayerName = "测试者"
	testPlayerRole = "旅行者"
)

type fixture struct {
	store    *sqlite.Store
	compiler *ctxpkg.Compiler
}

func newFixtureFull(t *testing.T, books []domain.Lorebook, opening, secretsJSON string, opts ctxpkg.CompilerOptions) *fixture {
	t.Helper()
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	snapshot := map[string]any{"lorebook": nil, "rules": nil}
	cardJSON := `{"schemaVersion":2,"cardId":"card_t","name":"测试卡","characters":[],"secrets":` + secretsJSON + `}`
	charTpl := &domain.TemplateVersion{
		TemplateVersionID: "tpl_card_t", Kind: domain.TemplateCharacter, SchemaVersion: 1,
		Content: cardJSON, ContentHash: "h_card",
	}
	var templates []*domain.TemplateVersion
	templates = append(templates, charTpl)
	snapshot["character"] = map[string]any{
		"templateVersionId": charTpl.TemplateVersionID, "schemaVersion": 1, "contentHash": "h_card",
	}
	if len(books) > 0 {
		content, _ := json.Marshal(books)
		tpl := &domain.TemplateVersion{
			TemplateVersionID: "tpl_lb", Kind: domain.TemplateLorebook, SchemaVersion: 1,
			Content: string(content),
		}
		templates = append(templates, tpl)
		snapshot["lorebook"] = map[string]any{
			"templateVersionId": tpl.TemplateVersionID, "schemaVersion": 1,
		}
	}
	snapshot["player"] = map[string]any{
		"schemaVersion": 1,
		"content":       map[string]any{"name": testPlayerName, "role": testPlayerRole},
	}
	rootContent, _ := json.Marshal(map[string]any{"templates": snapshot, "openingText": opening})
	root := &domain.PlotNode{
		NodeID: testRootID, SessionID: testSessionID, Kind: domain.NodeKindRoot,
		SchemaVersion: 1, ContentJSON: string(rootContent),
	}
	sess := &domain.Session{SessionID: testSessionID, RootNodeID: testRootID, Title: "测试"}
	branch := &domain.Branch{BranchID: testBranchID, SessionID: testSessionID, Name: "main", HeadNodeID: testRootID}
	if err := st.CreateSession(sess, root, branch, templates); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return &fixture{store: st, compiler: ctxpkg.New(st, opts)}
}

// systemPrompt 返回注入材料全文（见 injectedMaterials）。
func (f *fixture) systemPrompt(t *testing.T, nodeID, input string) string {
	t.Helper()
	return f.systemPromptWithState(t, nodeID, input, nil)
}

func (f *fixture) compileMessages(t *testing.T, nodeID, input string, state *domain.WorldState) []ports.ChatMessage {
	t.Helper()
	return f.compileMessagesWithChecks(t, nodeID, input, state, nil)
}

func (f *fixture) compileMessagesWithChecks(t *testing.T, nodeID, input string, state *domain.WorldState, checks []domain.CheckResult) []ports.ChatMessage {
	t.Helper()
	req, err := f.compiler.Compile(context.Background(), testSessionID, nodeID, input, ctxpkg.TurnDirectives{}, state, checks)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(req.Messages) == 0 || req.Messages[0].Role != "system" {
		t.Fatalf("首条消息应为 system，实际 %+v", req.Messages)
	}
	return req.Messages
}

// systemPromptWithState 返回注入材料全文。
func (f *fixture) systemPromptWithState(t *testing.T, nodeID, input string, state *domain.WorldState) string {
	t.Helper()
	return injectedMaterials(f.compileMessages(t, nodeID, input, state))
}

// injectedMaterials 拼接请求里的注入材料：首条静态 system 前缀 + 末尾的尾部状态块。
func injectedMaterials(msgs []ports.ChatMessage) string {
	var b strings.Builder
	for i, m := range msgs {
		isTailBlock := i == len(msgs)-1 && strings.Contains(m.Content, ctxpkg.SystemReminderOpen)
		if m.Role == "system" || isTailBlock {
			b.WriteString(m.Content)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func saveSummary(t *testing.T, f *fixture, from, to, text string) {
	t.Helper()
	if err := f.store.SaveSummary(&domain.SummaryArtifact{
		SummaryID:  "sum_" + to + "_" + text[:6],
		FromNodeID: from, ToNodeID: to,
		Text: text, SummaryVersion: 1,
	}); err != nil {
		t.Fatalf("save summary: %v", err)
	}
}
