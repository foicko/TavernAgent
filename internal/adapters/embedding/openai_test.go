package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "http://127.0.0.1:11434/v1/embeddings"},
		{"http://127.0.0.1:11434", "http://127.0.0.1:11434/v1/embeddings"},
		{"http://127.0.0.1:11434/", "http://127.0.0.1:11434/v1/embeddings"},
		{"http://127.0.0.1:11434/v1", "http://127.0.0.1:11434/v1/embeddings"},
		{"https://api.siliconflow.cn/v1", "https://api.siliconflow.cn/v1/embeddings"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1/embeddings"},
		{"https://api.openai.com/v1/embeddings", "https://api.openai.com/v1/embeddings"},
		{"http://localhost:8000/custom/embeddings", "http://localhost:8000/custom/embeddings"},
	}

	for _, tc := range tests {
		got := NormalizeEndpoint(tc.input)
		if got != tc.expected {
			t.Errorf("NormalizeEndpoint(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestOpenAIProvider_Embed(t *testing.T) {
	const mockDim = 512
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, `{"error":{"message":"unauthorized"}}`, http.StatusUnauthorized)
			return
		}

		var req openAIEmbeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		data := make([]openAIEmbeddingDataItem, len(req.Input))
		for i := range req.Input {
			vec := make([]float32, mockDim)
			vec[0] = float32(i + 1)
			vec[mockDim-1] = 0.5
			data[i] = openAIEmbeddingDataItem{
				Index:     i,
				Embedding: vec,
			}
		}

		resp := openAIEmbeddingResponse{
			Object: "list",
			Data:   data,
			Model:  req.Model,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider, err := NewOpenAIProvider(Config{
		BaseURL:   server.URL,
		APIKey:    "test-key",
		Model:     "bge-small-zh-v1.5",
		BatchSize: 2,
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider failed: %v", err)
	}

	// 1. Single text embed
	singleVec, err := provider.EmbedText(ctx, "测试文本一")
	if err != nil {
		t.Fatalf("EmbedText failed: %v", err)
	}
	if len(singleVec) != mockDim {
		t.Errorf("expected vector dimension %d, got %d", mockDim, len(singleVec))
	}
	if provider.Dimension() != mockDim {
		t.Errorf("provider dimension expected %d, got %d", mockDim, provider.Dimension())
	}

	// 2. Batch embed spanning multiple chunks (batchSize = 2, total = 5)
	texts := []string{"一", "二", "三", "四", "五"}
	batchVecs, err := provider.EmbedBatch(ctx, texts)
	if err != nil {
		t.Fatalf("EmbedBatch failed: %v", err)
	}
	if len(batchVecs) != 5 {
		t.Fatalf("expected 5 vectors, got %d", len(batchVecs))
	}
	for i, vec := range batchVecs {
		if len(vec) != mockDim {
			t.Errorf("vector %d has length %d, want %d", i, len(vec), mockDim)
		}
	}
}

func TestOpenAIProvider_Errors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded","type":"insufficient_quota"}}`))
	}))
	defer server.Close()

	provider, err := NewOpenAIProvider(Config{
		BaseURL: server.URL,
		Model:   "bge-small-zh-v1.5",
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider failed: %v", err)
	}

	_, err = provider.EmbedText(context.Background(), "hello")
	if err == nil {
		t.Fatalf("expected error from 403 response, got nil")
	}
}

func TestProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vec := make([]float32, 512)
		vec[0] = 0.123
		resp := openAIEmbeddingResponse{
			Object: "list",
			Data: []openAIEmbeddingDataItem{
				{Index: 0, Embedding: vec},
			},
			Model: "bge-small-zh-v1.5",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := Probe(ctx, Config{
		BaseURL: server.URL,
		Model:   "bge-small-zh-v1.5",
	}, "连通性测试")
	if err != nil {
		t.Fatalf("Probe returned unexpected error: %v", err)
	}
	if !result.OK {
		t.Fatalf("Probe expected OK=true, got false: %s", result.Error)
	}
	if result.Dimension != 512 {
		t.Errorf("expected dimension 512, got %d", result.Dimension)
	}
	if len(result.Preview) == 0 {
		t.Errorf("expected non-empty preview slice")
	}
}
