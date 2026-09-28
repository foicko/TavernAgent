package sqlite

import (
	"testing"

	"tavernagent/internal/domain"
)

func TestShouldMaterializeProjectionIsSparse(t *testing.T) {
	for _, depth := range []int{0, 64, 128} {
		if !shouldMaterializeProjection(depth) {
			t.Errorf("depth %d should materialize", depth)
		}
	}
	for _, depth := range []int{1, 63, 65, 127} {
		if shouldMaterializeProjection(depth) {
			t.Errorf("depth %d should not materialize", depth)
		}
	}
}

// M29：读路径不得再对单写连接写库。此前的实现会在最热路径上 INSERT 投影检查点。
func TestProjectedMemoriesIsReadOnly(t *testing.T) {
	s := newTestStore(t)
	_, root, _ := seedSession(t, s)

	if err := s.CreateMemory(&domain.MemoryRecord{
		MemoryID:     "mem_readonly",
		SourceNodeID: root.NodeID,
		Kind:         domain.MemoryObserved,
		Content:      "读路径不应写投影快照",
	}); err != nil {
		t.Fatalf("create memory: %v", err)
	}

	// CreateMemory 会清空全部投影快照；随后的读取不得把它们写回来。
	if _, err := s.ProjectedMemories(root.NodeID); err != nil {
		t.Fatalf("projected memories: %v", err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM memory_projection_snapshots`).Scan(&n); err != nil {
		t.Fatalf("count projections: %v", err)
	}
	if n != 0 {
		t.Fatalf("read path wrote %d projection snapshot(s); must stay read-only (M29)", n)
	}
}
