package application

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	"tavernagent/internal/domain"
)

type MemoryQuery struct {
	Search string
	Kind   string
	Limit  int
	Cursor string
}

type MemoryPage struct {
	Memories   []MemoryView       `json:"memories"`
	Usage      domain.MemoryUsage `json:"usage"`
	NodeID     string             `json:"nodeId"`
	NextCursor string             `json:"nextCursor,omitempty"`
	Total      int                `json:"total"`
	Counts     map[string]int     `json:"counts"`
}

type memoryCursor struct {
	Version int    `json:"v"`
	Session string `json:"s"`
	Branch  string `json:"b"`
	Node    string `json:"n"`
	Query   string `json:"q"`
	After   string `json:"a"`
}

// ListPageAt binds every page to an immutable inspected node. A background
// reflection advancing the branch cannot insert or remove rows between pages.
func (s *MemoryService) ListPageAt(sessionID, branchID, nodeID string, query MemoryQuery) (*MemoryPage, error) {
	if query.Limit == 0 {
		query.Limit = 40
	}
	if query.Limit < 1 || query.Limit > 200 || utf8.RuneCountInString(query.Search) > 300 || len(query.Cursor) > 4096 {
		return nil, Err("BAD_REQUEST", "limit 须为 1～200，搜索内容不超过 300 字", 400)
	}
	query.Search = strings.ToLower(strings.TrimSpace(query.Search))
	query.Kind = strings.ToLower(strings.TrimSpace(query.Kind))
	switch query.Kind {
	case "", "all", "hidden", "episodic", "inference", "observed", "reported", "inferred", "secret":
	default:
		return nil, Err("BAD_REQUEST", "未知的记忆分类", 400)
	}
	if branchID == "" {
		branches, err := s.store.ListBranches(sessionID)
		if err != nil {
			return nil, memoryReadError(err, "读取分支失败")
		}
		if len(branches) == 0 {
			return nil, Err("NOT_FOUND", "分支不存在", 404)
		}
		branchID = branches[0].BranchID
	}
	fingerprint := hashString(query.Search + "\x00" + query.Kind)
	var cursor memoryCursor
	if query.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(query.Cursor)
		if err != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Version != 1 || cursor.Node == "" || cursor.After == "" {
			return nil, Err("BAD_CURSOR", "分页游标无效，请重新读取第一页", 400)
		}
		if cursor.Session != sessionID || cursor.Branch != branchID || cursor.Query != fingerprint || (nodeID != "" && cursor.Node != nodeID) {
			return nil, Err("CURSOR_MISMATCH", "游标不属于当前节点或筛选条件，请重新读取第一页", 400)
		}
		nodeID = cursor.Node
	}
	if nodeID == "" {
		branch, err := s.store.GetBranch(branchID)
		if err != nil {
			return nil, memoryReadError(err, "读取分支失败")
		}
		if branch == nil || branch.SessionID != sessionID {
			return nil, Err("NOT_FOUND", "分支不存在", 404)
		}
		nodeID = branch.HeadNodeID
	}
	views, usage, err := s.ListAt(sessionID, branchID, nodeID)
	if err != nil {
		return nil, err
	}
	page := &MemoryPage{Memories: []MemoryView{}, Usage: usage, NodeID: nodeID, Counts: map[string]int{"all": 0, "hidden": 0, "episodic": 0, "inference": 0, "secret": 0}}
	filtered := make([]MemoryView, 0, len(views))
	for _, m := range views {
		category := "episodic"
		if m.Kind == domain.MemoryInferred {
			category = "inference"
		} else if m.Kind == domain.MemorySecret {
			category = "secret"
		}
		if m.Hidden {
			page.Counts["hidden"]++
		} else {
			page.Counts["all"]++
			page.Counts[category]++
		}
		if query.Kind == "hidden" {
			if !m.Hidden {
				continue
			}
		} else if query.Kind != "" {
			if m.Hidden {
				continue
			}
			if query.Kind != "all" && query.Kind != category && query.Kind != string(m.Kind) {
				continue
			}
		}
		if query.Search != "" && !strings.Contains(strings.ToLower(m.Content+" "+strings.Join(m.EntityIDs, " ")), query.Search) {
			continue
		}
		filtered = append(filtered, m)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].CreatedTurn != filtered[j].CreatedTurn {
			return filtered[i].CreatedTurn > filtered[j].CreatedTurn
		}
		return filtered[i].MemoryID > filtered[j].MemoryID
	})
	page.Total = len(filtered)
	start := 0
	if cursor.After != "" {
		found := false
		for i, m := range filtered {
			if m.MemoryID == cursor.After {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return nil, Err("CURSOR_EXPIRED", "记忆快照已变更，请重新读取第一页", 409)
		}
	}
	end := min(start+query.Limit, len(filtered))
	page.Memories = append(page.Memories, filtered[start:end]...)
	if end < len(filtered) {
		raw, _ := json.Marshal(memoryCursor{1, sessionID, branchID, nodeID, fingerprint, filtered[end-1].MemoryID})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return page, nil
}
