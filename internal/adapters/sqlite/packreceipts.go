package sqlite

import (
	"strings"
	"tavernagent/internal/domain"
)

func receiptsForNodesQ(q queryer, nodes []string) ([]domain.PortableReceipt, error) {
	out := []domain.PortableReceipt{}
	// Bound the SQL parameter count for very large exported trees.
	for start := 0; start < len(nodes); start += 400 {
		end := min(start+400, len(nodes))
		args := make([]any, 0, end-start)
		for _, n := range nodes[start:end] {
			args = append(args, n)
		}
		marks := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")
		rows, err := q.Query(`SELECT `+prefixed(receiptCols, "r")+`,t.result_node_id FROM action_receipts r JOIN turn_requests t ON t.turn_id=r.turn_id WHERE r.status='committed' AND t.result_node_id IN (`+marks+`) ORDER BY t.result_node_id,r.receipt_id`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			r := &domain.ActionReceipt{}
			p := domain.PortableReceipt{Receipt: r}
			if err := rows.Scan(&r.ReceiptID, &r.TurnID, &r.ActionID, &r.RollID, &r.BaseHeadID, &r.RulesetVersion, &r.ResultJSON, &r.Status, &p.NodeID); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
