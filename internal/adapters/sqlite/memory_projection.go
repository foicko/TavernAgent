package sqlite

import (
	"database/sql"
	"encoding/json"
	"tavernagent/internal/domain"
)

const schemaV14 = `
CREATE TABLE memory_projection_snapshots (
 node_id TEXT PRIMARY KEY REFERENCES plot_nodes(node_id),
 memory_ids TEXT NOT NULL
);
`

// Projections retain all unsuperseded chain tips, including subject-key losers.
// Keeping those losers is necessary: a later revision that changes a subject
// can make an older independent fact visible again. Subject and visibility
// filtering therefore remain domain operations, after path reconstruction.
func (s *Store) ProjectedMemories(nodeID string) ([]*domain.MemoryRecord, error) {
	s.projectionBuildMu.Lock()
	defer s.projectionBuildMu.Unlock()
	if records, ok := s.cachedMemoryProjection(nodeID); ok {
		return domain.ApplyMemoryOverlays(records), nil
	}
	node, err := s.GetNode(nodeID)
	if err != nil {
		return nil, err
	}
	base, cachedParent := s.cachedMemoryProjection(node.ParentID)
	var tips []*domain.MemoryRecord
	if cachedParent {
		delta, err := s.MemoriesInChain([]string{nodeID})
		if err != nil {
			return nil, err
		}
		tips = unreplacedMemories(append(base, delta...))
	} else {
		// Walk only to the closest checkpoint. The anti-join removes old
		// revisions in SQLite, before decoding any record into Go objects.
		rows, err := s.rdb().Query(`WITH RECURSIVE path(node_id,parent_id) AS (
 SELECT node_id,parent_id FROM plot_nodes WHERE node_id=?
 UNION ALL SELECT p.node_id,p.parent_id FROM plot_nodes p JOIN path child ON child.parent_id=p.node_id
 WHERE NOT EXISTS (SELECT 1 FROM memory_projection_snapshots snap WHERE snap.node_id=child.node_id)
), candidates(memory_id) AS (
 SELECT m.memory_id FROM memory_records m JOIN path p ON p.node_id=m.source_node_id
 UNION SELECT CAST(j.value AS TEXT) FROM path p JOIN memory_projection_snapshots snap ON snap.node_id=p.node_id,json_each(snap.memory_ids) j
)
SELECT `+prefixed(memoryCols, "m")+` FROM memory_records m JOIN candidates c ON c.memory_id=m.memory_id
 WHERE m.memory_id NOT IN (SELECT newer.supersedes FROM memory_records newer JOIN candidates c2 ON c2.memory_id=newer.memory_id WHERE newer.supersedes<>'') ORDER BY m.memory_id`, nodeID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			memory, err := scanMemory(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			tips = append(tips, memory)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	if !cachedParent || node.Depth%64 == 0 {
		ids := make([]string, 0, len(tips))
		for _, m := range tips {
			ids = append(ids, m.MemoryID)
		}
		raw, err := json.Marshal(ids)
		if err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO memory_projection_snapshots(node_id,memory_ids) VALUES(?,?)`, nodeID, string(raw)); err != nil {
			return nil, err
		}
	}
	s.cacheMemoryProjection(nodeID, tips)
	return domain.ApplyMemoryOverlays(cloneProjection(tips)), nil
}

func unreplacedMemories(records []*domain.MemoryRecord) []*domain.MemoryRecord {
	replaced := make(map[string]bool, len(records))
	for _, m := range records {
		if m.Supersedes != "" {
			replaced[m.Supersedes] = true
		}
	}
	out := make([]*domain.MemoryRecord, 0, len(records))
	for _, m := range records {
		if !replaced[m.MemoryID] {
			out = append(out, m)
		}
	}
	return out
}

func cloneProjection(records []*domain.MemoryRecord) []*domain.MemoryRecord {
	out := make([]*domain.MemoryRecord, 0, len(records))
	for _, m := range records {
		copy := *m
		copy.OwnerIDs = append([]string(nil), m.OwnerIDs...)
		copy.EntityIDs = append([]string(nil), m.EntityIDs...)
		copy.MergedFrom = append([]string(nil), m.MergedFrom...)
		if m.Evidence != nil {
			evidence := *m.Evidence
			copy.Evidence = &evidence
		}
		out = append(out, &copy)
	}
	return out
}

func (s *Store) cachedMemoryProjection(nodeID string) ([]*domain.MemoryRecord, bool) {
	s.projectionMu.Lock()
	defer s.projectionMu.Unlock()
	records, ok := s.projectionCache[nodeID]
	if !ok {
		return nil, false
	}
	return cloneProjection(records), true
}

func (s *Store) cacheMemoryProjection(nodeID string, records []*domain.MemoryRecord) {
	s.projectionMu.Lock()
	defer s.projectionMu.Unlock()
	if s.projectionCache == nil {
		s.projectionCache = map[string][]*domain.MemoryRecord{}
	}
	if _, ok := s.projectionCache[nodeID]; ok {
		return
	}
	// Bound both head count and record count; private/raw records are never
	// cached as a browser-visible response. Visibility is checked on each read.
	count := len(records)
	for _, cached := range s.projectionCache {
		count += len(cached)
	}
	for len(s.projectionOrder) > 0 && (len(s.projectionOrder) >= 16 || count > 8192) {
		old := s.projectionOrder[0]
		s.projectionOrder = s.projectionOrder[1:]
		count -= len(s.projectionCache[old])
		delete(s.projectionCache, old)
	}
	if len(records) > 8192 {
		return
	}
	s.projectionCache[nodeID] = cloneProjection(records)
	s.projectionOrder = append(s.projectionOrder, nodeID)
}

// Legacy administrative writes can alter an existing node; normal commits
// append immutable nodes and need no invalidation.
func (s *Store) commitMemoryMutation(tx *sql.Tx) error {
	if _, err := tx.Exec(`DELETE FROM memory_projection_snapshots`); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.projectionMu.Lock()
	s.projectionCache = nil
	s.projectionOrder = nil
	s.projectionMu.Unlock()
	return nil
}
