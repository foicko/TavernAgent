// Package ports defines embedding and vector retrieval port contracts for TavernAgent.
// These interfaces support hybrid lexical-semantic retrieval (BM25 + Dense Vector Embeddings)
// while keeping the domain and application layers decoupled from specific embedding backends.
package ports

import (
	"context"

	"tavernagent/internal/search"
)

// Vector aliases the core vector slice type from search.
type Vector = search.Vector

// VectorMatch aliases the scored match type from search.
type VectorMatch = search.VectorMatch

// VectorIndex aliases the vector storage and nearest-neighbor search port.
type VectorIndex = search.VectorIndex

// EmbeddingProvider transforms raw text into vector representations.
// Implementations can wrap OpenAI embeddings, Ollama, ONNX Runtime, or local microservices.
type EmbeddingProvider interface {
	// EmbedText computes the vector embedding for a single text document or query.
	EmbedText(ctx context.Context, text string) (Vector, error)
	// EmbedBatch computes embeddings for a slice of texts in a batch.
	EmbedBatch(ctx context.Context, texts []string) ([]Vector, error)
	// Dimension returns the fixed output dimension of the embedding space.
	Dimension() int
}
