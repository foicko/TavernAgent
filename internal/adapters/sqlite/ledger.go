package sqlite

import (
	"strings"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// Permanent effects are never capped or tied to the optional dialogue window.
// Recent significant rulings have a separate limit to keep ordinary history bounded.
func (s *Store) ReceiptsOnPath(nodeID string) ([]ports.ReceiptAtNode, error) {
	rows, err := s.rdb().Query(`WITH RECURSIVE candidates AS MATERIALIZED (
 SELECT r.*,t.result_node_id FROM action_receipts r JOIN turn_requests t ON t.turn_id=r.turn_id
 WHERE t.session_id=(SELECT session_id FROM plot_nodes WHERE node_id=?) AND r.status='committed'
 AND (COALESCE(json_extract(r.result_json,'$.permanentEffect'),'')<>''
  OR json_extract(r.result_json,'$.outcome') IN ('critical_success','critical_failure')
  OR (json_extract(r.result_json,'$.outcome')='failure' AND json_extract(r.result_json,'$.dc')>=15))
), up(id) AS (
 SELECT node_id FROM plot_nodes WHERE node_id=? AND EXISTS(SELECT 1 FROM candidates)
 UNION ALL SELECT p.parent_id FROM plot_nodes p JOIN up ON up.id=p.node_id WHERE p.parent_id IS NOT NULL
), eligible AS (
 SELECT r.*,n.node_id,n.turn_number,
  COALESCE(json_extract(r.result_json,'$.permanentEffect'),'')<>'' AS permanent
 FROM candidates r JOIN plot_nodes n ON n.node_id=r.result_node_id JOIN up ON up.id=n.node_id
), selected AS (
 SELECT * FROM eligible WHERE permanent=1
 UNION ALL SELECT * FROM (SELECT * FROM eligible WHERE permanent=0 ORDER BY turn_number DESC,receipt_id LIMIT 10)
)
SELECT `+receiptCols+`,node_id,turn_number FROM selected ORDER BY turn_number DESC,receipt_id`, nodeID, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ports.ReceiptAtNode{}
	for rows.Next() {
		r := &domain.ActionReceipt{}
		at := ports.ReceiptAtNode{Receipt: r}
		if err := rows.Scan(&r.ReceiptID, &r.TurnID, &r.ActionID, &r.RollID, &r.BaseHeadID, &r.RulesetVersion, &r.ResultJSON, &r.Status, &at.NodeID, &at.TurnNumber); err != nil {
			return nil, err
		}
		out = append(out, at)
	}
	return out, rows.Err()
}

// EventsOnPath 按拓扑时序返回祖先链上的指定事件。
func (s *Store) EventsOnPath(nodeID string, eventTypes ...domain.EventType) ([]ports.EventAtNode, error) {
	if nodeID == "" {
		return nil, nil
	}
	query := `WITH RECURSIVE up(id) AS (
 SELECT node_id FROM plot_nodes WHERE node_id=?
 UNION ALL SELECT p.parent_id FROM plot_nodes p JOIN up ON up.id=p.node_id WHERE p.parent_id IS NOT NULL
)
SELECT e.event_id, e.node_id, e.event_index, e.type, e.payload_json, e.ruleset_version, n.turn_number
FROM domain_events e
JOIN plot_nodes n ON n.node_id = e.node_id
JOIN up ON up.id = n.node_id`

	args := []any{nodeID}
	if len(eventTypes) > 0 {
		ph := make([]string, len(eventTypes))
		for i, t := range eventTypes {
			ph[i] = "?"
			args = append(args, string(t))
		}
		query += ` WHERE e.type IN (` + strings.Join(ph, ",") + `)`
	}
	query += ` ORDER BY n.turn_number ASC, e.event_index ASC`

	rows, err := s.rdb().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ports.EventAtNode
	for rows.Next() {
		var ev domain.DomainEvent
		var at ports.EventAtNode
		var typ string
		if err := rows.Scan(&ev.EventID, &ev.NodeID, &ev.EventIndex, &typ, &ev.PayloadJSON, &ev.RulesetVersion, &at.TurnNumber); err != nil {
			return nil, err
		}
		ev.Type = domain.EventType(typ)
		at.NodeID = ev.NodeID
		at.Event = &ev
		out = append(out, at)
	}
	return out, rows.Err()
}
