package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tavernagent/internal/adapters/embedding"
	httppkg "tavernagent/internal/adapters/http"
)

func TestEmbeddingProbeEndpoint(t *testing.T) {
	// Mock OpenAI embedding upstream server
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vec := make([]float32, 512)
		vec[0] = 0.05
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"index": 0, "embedding": vec},
			},
			"model": "bge-small-zh-v1.5",
		})
	}))
	defer mockUpstream.Close()

	srv, err := httppkg.New(httppkg.Deps{
		Addr: "127.0.0.1:8890",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"baseUrl": mockUpstream.URL,
		"model":   "bge-small-zh-v1.5",
	})

	req := httptest.NewRequest("POST", "/api/v1/embeddings/probe", bytes.NewReader(payload))
	req.Host = "127.0.0.1:8890"
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res embedding.ProbeResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected probe ok, got error: %s", res.Error)
	}
	if res.Dimension != 512 {
		t.Errorf("expected dimension 512, got %d", res.Dimension)
	}
	if res.Model != "bge-small-zh-v1.5" {
		t.Errorf("expected model bge-small-zh-v1.5, got %s", res.Model)
	}
}

func TestEmbeddingComputeEndpoint(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vec1 := make([]float32, 512)
		vec2 := make([]float32, 512)
		vec1[0] = 0.1
		vec2[0] = 0.2
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"index": 0, "embedding": vec1},
				{"index": 1, "embedding": vec2},
			},
			"model": "bge-small-zh-v1.5",
		})
	}))
	defer mockUpstream.Close()

	srv, err := httppkg.New(httppkg.Deps{
		Addr: "127.0.0.1:8890",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"texts":   []string{"文本一", "文本二"},
		"baseUrl": mockUpstream.URL,
		"model":   "bge-small-zh-v1.5",
	})

	req := httptest.NewRequest("POST", "/api/v1/embeddings", bytes.NewReader(payload))
	req.Host = "127.0.0.1:8890"
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Model      string      `json:"model"`
		Dimension  int         `json:"dimension"`
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Embeddings) != 2 {
		t.Errorf("expected 2 embeddings, got %d", len(resp.Embeddings))
	}
	if resp.Dimension != 512 {
		t.Errorf("expected dimension 512, got %d", resp.Dimension)
	}
}
