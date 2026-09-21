// Package search provides lexical, phonetic, and hybrid retrieval algorithms.
// It is a leaf package with zero internal dependencies.
package search

import (
	"context"
	"math"
	"sort"
	"sync"
)

// Vector represents a normalized embedding vector of float32 values.
type Vector []float32

// VectorMatch represents an entity retrieved and scored by vector similarity.
type VectorMatch struct {
	ID       string            `json:"id"`
	Score    float64           `json:"score"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// VectorIndex defines the in-memory or persistent vector index interface.
type VectorIndex interface {
	Upsert(ctx context.Context, id string, vec Vector, metadata map[string]string) error
	Search(ctx context.Context, query Vector, topK int, filter map[string]string) ([]VectorMatch, error)
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int, error)
}

// CosineSimilarity computes the cosine similarity between two float32 vectors.
// Returns a value in [-1.0, 1.0], or 0.0 if either vector has zero norm or dimensions mismatch.
func CosineSimilarity(a, b Vector) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0.0
	}
	var dot, normA, normB float64
	for i := range a {
		va := float64(a[i])
		vb := float64(b[i])
		dot += va * vb
		normA += va * va
		normB += vb * vb
	}
	if normA <= 0 || normB <= 0 {
		return 0.0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// RankedItem represents an entity scored by either lexical BM25 or vector search.
type RankedItem struct {
	ID       string            `json:"id"`
	Score    float64           `json:"score"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ReciprocalRankFusion combines lexical (e.g. BM25) and dense vector ranked lists
// using the standard Reciprocal Rank Fusion (RRF) formula:
// RRF(d) = sum( w_s / (k + rank_s(d)) )
//
// k is typically 60.0. Higher rank (index 0) gets higher weight.
func ReciprocalRankFusion(lexical []RankedItem, vector []RankedItem, k float64, lexicalWeight, vectorWeight float64) []RankedItem {
	if k <= 0 {
		k = 60.0
	}
	if lexicalWeight <= 0 && vectorWeight <= 0 {
		lexicalWeight = 0.5
		vectorWeight = 0.5
	}

	scores := make(map[string]float64)
	metadata := make(map[string]map[string]string)

	for rank, item := range lexical {
		scores[item.ID] += lexicalWeight / (k + float64(rank+1))
		if _, exists := metadata[item.ID]; !exists && item.Metadata != nil {
			metadata[item.ID] = item.Metadata
		}
	}

	for rank, item := range vector {
		scores[item.ID] += vectorWeight / (k + float64(rank+1))
		if _, exists := metadata[item.ID]; !exists && item.Metadata != nil {
			metadata[item.ID] = item.Metadata
		}
	}

	var results []RankedItem
	for id, score := range scores {
		results = append(results, RankedItem{
			ID:       id,
			Score:    score,
			Metadata: metadata[id],
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].ID < results[j].ID
		}
		return results[i].Score > results[j].Score
	})

	return results
}

// InMemoryVectorIndex is a concurrent in-memory implementation of VectorIndex.
// It is ideal for local memory candidate search and automated unit testing.
type InMemoryVectorIndex struct {
	mu      sync.RWMutex
	vectors map[string]Vector
	meta    map[string]map[string]string
}

// NewInMemoryVectorIndex creates an empty in-memory vector index.
func NewInMemoryVectorIndex() *InMemoryVectorIndex {
	return &InMemoryVectorIndex{
		vectors: make(map[string]Vector),
		meta:    make(map[string]map[string]string),
	}
}

var _ VectorIndex = (*InMemoryVectorIndex)(nil)

func (idx *InMemoryVectorIndex) Upsert(_ context.Context, id string, vec Vector, metadata map[string]string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	vCopy := make(Vector, len(vec))
	copy(vCopy, vec)
	idx.vectors[id] = vCopy

	if metadata != nil {
		mCopy := make(map[string]string, len(metadata))
		for k, v := range metadata {
			mCopy[k] = v
		}
		idx.meta[id] = mCopy
	} else {
		delete(idx.meta, id)
	}
	return nil
}

func (idx *InMemoryVectorIndex) Search(_ context.Context, query Vector, topK int, filter map[string]string) ([]VectorMatch, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var matches []VectorMatch
	for id, vec := range idx.vectors {
		// Filter matching
		if len(filter) > 0 {
			m := idx.meta[id]
			mismatch := false
			for fk, fv := range filter {
				if m == nil || m[fk] != fv {
					mismatch = true
					break
				}
			}
			if mismatch {
				continue
			}
		}

		sim := CosineSimilarity(query, vec)
		matches = append(matches, VectorMatch{
			ID:       id,
			Score:    sim,
			Metadata: idx.meta[id],
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].ID < matches[j].ID
		}
		return matches[i].Score > matches[j].Score
	})

	if topK > 0 && len(matches) > topK {
		matches = matches[:topK]
	}

	return matches, nil
}

func (idx *InMemoryVectorIndex) Delete(_ context.Context, id string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.vectors, id)
	delete(idx.meta, id)
	return nil
}

func (idx *InMemoryVectorIndex) Count(_ context.Context) (int, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.vectors), nil
}
