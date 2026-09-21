package search_test

import (
	"context"
	"math"
	"testing"

	"tavernagent/internal/search"
)

func TestCosineSimilarity(t *testing.T) {
	vecA := search.Vector{1.0, 0.0, 0.0}
	vecB := search.Vector{1.0, 0.0, 0.0}
	if sim := search.CosineSimilarity(vecA, vecB); math.Abs(sim-1.0) > 1e-6 {
		t.Errorf("expected similarity 1.0 for identical vectors, got %f", sim)
	}

	vecOrthogonal := search.Vector{0.0, 1.0, 0.0}
	if sim := search.CosineSimilarity(vecA, vecOrthogonal); math.Abs(sim-0.0) > 1e-6 {
		t.Errorf("expected similarity 0.0 for orthogonal vectors, got %f", sim)
	}

	vecOpposite := search.Vector{-1.0, 0.0, 0.0}
	if sim := search.CosineSimilarity(vecA, vecOpposite); math.Abs(sim-(-1.0)) > 1e-6 {
		t.Errorf("expected similarity -1.0 for opposite vectors, got %f", sim)
	}
}

func TestReciprocalRankFusion(t *testing.T) {
	lexical := []search.RankedItem{
		{ID: "doc-A", Score: 5.2},
		{ID: "doc-B", Score: 3.1},
		{ID: "doc-C", Score: 1.0},
	}
	vector := []search.RankedItem{
		{ID: "doc-B", Score: 0.92},
		{ID: "doc-A", Score: 0.85},
		{ID: "doc-D", Score: 0.70},
	}

	fused := search.ReciprocalRankFusion(lexical, vector, 60.0, 0.5, 0.5)
	if len(fused) != 4 {
		t.Fatalf("expected 4 unique docs, got %d", len(fused))
	}

	topTwo := map[string]bool{fused[0].ID: true, fused[1].ID: true}
	if !topTwo["doc-A"] || !topTwo["doc-B"] {
		t.Errorf("expected doc-A and doc-B at top, got %v", fused)
	}
}

func TestInMemoryVectorIndex(t *testing.T) {
	ctx := context.Background()
	idx := search.NewInMemoryVectorIndex()

	_ = idx.Upsert(ctx, "mem-1", search.Vector{1.0, 0.0, 0.0}, map[string]string{"type": "episodic"})
	_ = idx.Upsert(ctx, "mem-2", search.Vector{0.8, 0.6, 0.0}, map[string]string{"type": "semantic"})
	_ = idx.Upsert(ctx, "mem-3", search.Vector{0.0, 1.0, 0.0}, map[string]string{"type": "episodic"})

	count, _ := idx.Count(ctx)
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}

	// Search closest to [1.0, 0.0, 0.0]
	matches, err := idx.Search(ctx, search.Vector{1.0, 0.0, 0.0}, 2, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].ID != "mem-1" {
		t.Errorf("expected mem-1 at top, got %s", matches[0].ID)
	}

	// Search with filter
	filtered, err := idx.Search(ctx, search.Vector{1.0, 0.0, 0.0}, 10, map[string]string{"type": "semantic"})
	if err != nil {
		t.Fatalf("Filtered search failed: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "mem-2" {
		t.Errorf("expected only mem-2 in filtered search, got %v", filtered)
	}

	// Delete
	_ = idx.Delete(ctx, "mem-1")
	countAfter, _ := idx.Count(ctx)
	if countAfter != 2 {
		t.Errorf("expected count 2 after delete, got %d", countAfter)
	}
}
