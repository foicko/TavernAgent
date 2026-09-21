// Package embedding provides embedding model adapters for TavernAgent.
// It implements ports.EmbeddingProvider using OpenAI-compatible /v1/embeddings endpoints,
// supporting local Ollama, SiliconFlow, OpenAI, vLLM, and other standard embedding providers.
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"tavernagent/internal/ports"
)

const (
	// DefaultModel is the recommended lightweight, high-performance Chinese embedding model.
	DefaultModel = "bge-small-zh-v1.5"
	// DefaultBaseURL is the default local Ollama OpenAI-compatible endpoint.
	DefaultBaseURL = "http://127.0.0.1:11434/v1"
	// DefaultDimension is the output embedding vector dimension for bge-small-zh-v1.5.
	DefaultDimension = 512
	// DefaultBatchSize is the maximum number of texts to embed in a single HTTP request.
	DefaultBatchSize = 32
	// DefaultTimeout is the HTTP request timeout.
	DefaultTimeout = 30 * time.Second
)

// Config configures an OpenAI-compatible embedding provider.
type Config struct {
	BaseURL   string        `json:"baseUrl"`
	APIKey    string        `json:"apiKey,omitempty"`
	Model     string        `json:"model"`
	Dimension int           `json:"dimension,omitempty"`
	BatchSize int           `json:"batchSize,omitempty"`
	Timeout   time.Duration `json:"timeout,omitempty"`
}

// ProbeResult contains diagnostic information from a model connectivity test.
type ProbeResult struct {
	OK        bool      `json:"ok"`
	Model     string    `json:"model"`
	Dimension int       `json:"dimension"`
	LatencyMs int64     `json:"latencyMs"`
	Preview   []float32 `json:"preview,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// NormalizeEndpoint normalizes any raw base URL to an /embeddings endpoint.
func NormalizeEndpoint(rawURL string) string {
	u := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if u == "" {
		return "http://127.0.0.1:11434/v1/embeddings"
	}
	if strings.HasSuffix(u, "/embeddings") {
		return u
	}
	if strings.HasSuffix(u, "/v1") {
		return u + "/embeddings"
	}
	// For bare host or domain (e.g. http://127.0.0.1:11434 or api.siliconflow.cn)
	if strings.HasSuffix(u, ":11434") || strings.HasSuffix(u, "openai.com") || strings.HasSuffix(u, "siliconflow.cn") {
		return u + "/v1/embeddings"
	}
	return u + "/embeddings"
}

// OpenAIProvider implements ports.EmbeddingProvider via OpenAI-compatible REST API.
type OpenAIProvider struct {
	client    *http.Client
	endpoint  string
	apiKey    string
	model     string
	batchSize int

	mu        sync.RWMutex
	dimension int
}

var _ ports.EmbeddingProvider = (*OpenAIProvider)(nil)

// NewOpenAIProvider creates an initialized OpenAI-compatible embedding provider.
func NewOpenAIProvider(cfg Config) (*OpenAIProvider, error) {
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultModel
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	dimension := cfg.Dimension
	if dimension <= 0 && model == DefaultModel {
		dimension = DefaultDimension
	}

	return &OpenAIProvider{
		client: &http.Client{
			Timeout: timeout,
		},
		endpoint:  NormalizeEndpoint(cfg.BaseURL),
		apiKey:    strings.TrimSpace(cfg.APIKey),
		model:     model,
		dimension: dimension,
		batchSize: batchSize,
	}, nil
}

// Dimension returns the vector dimension.
func (p *OpenAIProvider) Dimension() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.dimension
}

// setDimension updates the dimension discovered dynamically from response.
func (p *OpenAIProvider) setDimension(dim int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dimension <= 0 {
		p.dimension = dim
	}
}

// Model returns the active model name.
func (p *OpenAIProvider) Model() string {
	return p.model
}

// Endpoint returns the target /embeddings URL.
func (p *OpenAIProvider) Endpoint() string {
	return p.endpoint
}

type openAIEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbeddingDataItem struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type openAIEmbeddingResponse struct {
	Object string                    `json:"object"`
	Data   []openAIEmbeddingDataItem `json:"data"`
	Model  string                    `json:"model"`
	Error  *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// EmbedText generates a vector embedding for a single text.
func (p *OpenAIProvider) EmbedText(ctx context.Context, text string) (ports.Vector, error) {
	vecs, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("embedding API returned empty results")
	}
	return vecs[0], nil
}

// EmbedBatch generates vector embeddings for a slice of texts, chunking into batchSize if necessary.
func (p *OpenAIProvider) EmbedBatch(ctx context.Context, texts []string) ([]ports.Vector, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	allVectors := make([]ports.Vector, len(texts))

	for start := 0; start < len(texts); start += p.batchSize {
		end := start + p.batchSize
		if end > len(texts) {
			end = len(texts)
		}
		chunk := texts[start:end]

		reqBody := openAIEmbeddingRequest{
			Model: p.model,
			Input: chunk,
		}

		payload, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshal embedding request: %w", err)
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("create embedding request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if p.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		}

		resp, err := p.client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("embedding request failed: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read embedding response: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			var errResp openAIEmbeddingResponse
			if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil && errResp.Error.Message != "" {
				return nil, fmt.Errorf("embedding provider error (%d): %s", resp.StatusCode, errResp.Error.Message)
			}
			return nil, fmt.Errorf("embedding provider HTTP %d: %s", resp.StatusCode, string(respBody))
		}

		var parsed openAIEmbeddingResponse
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return nil, fmt.Errorf("unmarshal embedding response: %w", err)
		}

		if len(parsed.Data) != len(chunk) {
			return nil, fmt.Errorf("embedding count mismatch: expected %d, got %d", len(chunk), len(parsed.Data))
		}

		for _, item := range parsed.Data {
			if item.Index < 0 || item.Index >= len(chunk) {
				return nil, fmt.Errorf("invalid embedding index %d for chunk size %d", item.Index, len(chunk))
			}
			vec := ports.Vector(item.Embedding)
			allVectors[start+item.Index] = vec
			p.setDimension(len(vec))
		}
	}

	return allVectors, nil
}

// Probe tests connectivity with the embedding provider, measures latency, and returns vector dimension.
func Probe(ctx context.Context, cfg Config, sampleText string) (ProbeResult, error) {
	if strings.TrimSpace(sampleText) == "" {
		sampleText = "TavernAgent 记忆向量检索连通性探测测试"
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultModel
	}

	provider, err := NewOpenAIProvider(cfg)
	if err != nil {
		return ProbeResult{
			OK:    false,
			Model: model,
			Error: err.Error(),
		}, err
	}

	start := time.Now()
	vec, err := provider.EmbedText(ctx, sampleText)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return ProbeResult{
			OK:        false,
			Model:     model,
			LatencyMs: latency,
			Error:     err.Error(),
		}, nil
	}

	dim := len(vec)
	previewLen := 5
	if dim < previewLen {
		previewLen = dim
	}

	var preview []float32
	if previewLen > 0 {
		preview = make([]float32, previewLen)
		copy(preview, vec[:previewLen])
	}

	return ProbeResult{
		OK:        true,
		Model:     model,
		Dimension: dim,
		LatencyMs: latency,
		Preview:   preview,
	}, nil
}
