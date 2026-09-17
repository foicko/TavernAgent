package sqlite

import (
	"strings"
	"tavernagent/internal/domain"
)

func childArgs(sessionID string, parents []string) (string, []any) {
	args := []any{sessionID}
	for _, p := range parents {
		args = append(args, p)
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(parents)), ","), args
}

func (s *Store) ChildWindow(sessionID string, parents []string, limit int, content bool) (map[string][]*domain.PlotNode, bool, error) {
	out := map[string][]*domain.PlotNode{}
	if len(parents) == 0 || limit <= 0 {
		return out, false, nil
	}
	limit = min(limit, 500)
	marks, args := childArgs(sessionID, parents)
	cols := nodeCols
	if !content {
		cols = strings.Replace(cols, "content_json", "'' AS content_json", 1)
	}
	args = append(args, limit+1)
	rows, err := s.rdb().Query(`SELECT `+cols+` FROM plot_nodes WHERE session_id=? AND parent_id IN (`+marks+`) ORDER BY depth,parent_id,created_at,node_id LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	num := 0
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, false, err
		}
		num++
		if num > limit {
			return out, true, nil
		}
		out[n.ParentID] = append(out[n.ParentID], n)
	}
	return out, false, rows.Err()
}

func (s *Store) ChildCounts(sessionID string, parents []string) (map[string]int, error) {
	out := map[string]int{}
	if len(parents) == 0 {
		return out, nil
	}
	marks, args := childArgs(sessionID, parents)
	rows, err := s.rdb().Query(`SELECT parent_id,COUNT(*) FROM plot_nodes WHERE session_id=? AND parent_id IN (`+marks+`) GROUP BY parent_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		out[id] = count
	}
	return out, rows.Err()
}
