package application

import (
	"encoding/json"

	"tavernagent/internal/domain"
)

// 本文件是秘密/世界观条目的读取与映射（M4d）。
//
// 秘密**定义**来自卡片（随角色模板版本落库，T20：会话内版本固定），
// 解锁**状态**来自世界状态投影的 UnlockedSecrets（随事件演进、随分支隔离）。
// 两者分开是刻意的：定义是"世界上存在什么秘密"，状态是"这条路径上揭示了哪些"。

// secretDefsOf 读取会话的秘密定义（提交路径用）。
func (s *TurnService) secretDefsOf(sessionID string) []domain.SecretDef {
	return sessionSecretDefs(s.store, sessionID)
}

// sessionSecretDefs 读取会话的秘密定义。
//
// 与世界书同一条读取路径：根节点 → character 模板版本 → 卡片 JSON。
// 读不到时返回空（会话没有秘密是正常情况，不是错误）。
// 做成独立函数是因为两个消费者都要它：提交路径（解锁判定）
// 与读取视图（前端「世界观/秘密」区块）。
func sessionSecretDefs(store interface {
	GetSession(sessionID string) (*domain.Session, error)
	GetNode(nodeID string) (*domain.PlotNode, error)
	GetTemplateVersion(id string) (*domain.TemplateVersion, error)
}, sessionID string) []domain.SecretDef {
	sess, err := store.GetSession(sessionID)
	if err != nil || sess == nil || sess.RootNodeID == "" {
		return nil
	}
	root, err := store.GetNode(sess.RootNodeID)
	if err != nil || root == nil {
		return nil
	}
	var rc struct {
		Templates map[string]struct {
			TemplateVersionID string `json:"templateVersionId"`
		} `json:"templates"`
	}
	if err := json.Unmarshal([]byte(root.ContentJSON), &rc); err != nil {
		return nil
	}
	ref, ok := rc.Templates["character"]
	if !ok || ref.TemplateVersionID == "" {
		return nil
	}
	tv, err := store.GetTemplateVersion(ref.TemplateVersionID)
	if err != nil || tv == nil || tv.Content == "" {
		return nil
	}
	card, err := ParseCharacterCard(tv.Content)
	if err != nil {
		return nil
	}
	return secretsFromCard(card)
}

// secretsFromCard 把卡片秘密条目映射为领域定义。
//
// RevealWhen 在这里解析：解析失败的条目被跳过而不是让整个会话不可用——
// Setup 已经在入口拦截过非法规则，这里遇到只可能是数据在会话建立后损坏。
func secretsFromCard(card *CharacterCard) []domain.SecretDef {
	if card == nil || len(card.Secrets) == 0 {
		return nil
	}
	out := make([]domain.SecretDef, 0, len(card.Secrets))
	for _, it := range card.Secrets {
		def := domain.SecretDef{
			SecretID: it.SecretID,
			Title:    it.Title,
			Content:  it.Content,
			Order:    it.Order,
		}
		if def.SecretID == "" {
			// 卡片未写 ID 时按标题派生，保证秘密始终可被事件引用。
			def.SecretID = "secret_" + hashString(def.Title + def.Content)[:12]
		}
		if expr, err := domain.ParseRuleExpr(it.RevealWhen); err == nil {
			def.RevealWhen = expr
		}
		out = append(out, def)
	}
	return out
}
