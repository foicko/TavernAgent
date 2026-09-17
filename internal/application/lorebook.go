package application

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

type LorebookPage struct {
	Entries    []ports.LorebookMatch `json:"entries"`
	HasMore    bool                  `json:"hasMore"`
	NextOffset int                   `json:"nextOffset"`
}

// Lorebook searches only the worldbook snapshot attached to this session.
// Search results are public settings; secret content comes from View's separate
// rule-filtered SecretView, and is never part of this index.
func (s *SessionService) Lorebook(sessionID, query string, offset, limit int) (*LorebookPage, error) {
	if offset < 0 || offset > 100000 || utf8.RuneCountInString(query) > 512 {
		return nil, Err("INVALID_QUERY", "世界书查询过长或分页位置无效", 400)
	}
	if limit <= 0 {
		limit = 40
	}
	limit = min(limit, 100)
	session, err := s.store.GetSession(sessionID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) || errors.Is(err, domain.ErrItemNotFound) {
			return nil, Err("NOT_FOUND", "会话不存在", 404)
		}
		return nil, Err("STORAGE_UNAVAILABLE", "读取会话失败", 503)
	}
	root, err := s.store.GetNode(session.RootNodeID)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取会话模板失败", 503)
	}
	var content struct {
		Templates map[string]*struct {
			TemplateVersionID string `json:"templateVersionId"`
		} `json:"templates"`
	}
	if err := json.Unmarshal([]byte(root.ContentJSON), &content); err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "会话模板无法解析", 503)
	}
	page := &LorebookPage{Entries: make([]ports.LorebookMatch, 0)}
	ref := content.Templates["lorebook"]
	if ref == nil || ref.TemplateVersionID == "" {
		return page, nil
	}
	entries, err := s.store.SearchLorebook(ref.TemplateVersionID, strings.TrimSpace(query), offset, limit+1)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "检索世界书失败", 503)
	}
	page.HasMore = len(entries) > limit
	if page.HasMore {
		entries = entries[:limit]
	}
	page.Entries = entries
	page.NextOffset = offset + len(entries)
	return page, nil
}
