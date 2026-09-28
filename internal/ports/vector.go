// Package ports defines embedding port contracts for TavernAgent.
//
// The embedding provider abstraction keeps the application layer decoupled from
// specific embedding backends (OpenAI-compatible REST, Ollama, vLLM, ...).
package ports

import "context"

// Vector is a dense embedding vector.
type Vector []float32

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
