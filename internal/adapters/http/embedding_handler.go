package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"tavernagent/internal/adapters/embedding"
)

type embeddingProbeRequest struct {
	BaseURL    string `json:"baseUrl,omitempty"`
	APIKey     string `json:"apiKey,omitempty"`
	Model      string `json:"model,omitempty"`
	SampleText string `json:"sampleText,omitempty"`
}

func (s *Server) probeEmbedding(w http.ResponseWriter, r *http.Request) {
	var req embeddingProbeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body", false, "")
		return
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = embedding.DefaultModel
	}

	res, err := embedding.Probe(r.Context(), embedding.Config{
		BaseURL: req.BaseURL,
		APIKey:  req.APIKey,
		Model:   model,
	}, req.SampleText)

	if err != nil {
		writeJSON(w, http.StatusOK, res)
		return
	}

	writeJSON(w, http.StatusOK, res)
}

type embeddingsComputeRequest struct {
	Texts     []string `json:"texts"`
	BaseURL   string   `json:"baseUrl,omitempty"`
	APIKey    string   `json:"apiKey,omitempty"`
	Model     string   `json:"model,omitempty"`
	Dimension int      `json:"dimension,omitempty"`
}

func (s *Server) computeEmbeddings(w http.ResponseWriter, r *http.Request) {
	var req embeddingsComputeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body", false, "")
		return
	}

	if len(req.Texts) == 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "texts array cannot be empty", false, "")
		return
	}
	if len(req.Texts) > 128 {
		writeError(w, http.StatusBadRequest, "TOO_MANY_TEXTS", "maximum 128 texts per embedding request", false, "")
		return
	}

	provider, err := embedding.NewOpenAIProvider(embedding.Config{
		BaseURL:   req.BaseURL,
		APIKey:    req.APIKey,
		Model:     req.Model,
		Dimension: req.Dimension,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "PROVIDER_INIT_FAILED", err.Error(), false, "")
		return
	}

	vecs, err := provider.EmbedBatch(r.Context(), req.Texts)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EMBEDDING_FAILED", err.Error(), true, "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"model":      provider.Model(),
		"dimension":  provider.Dimension(),
		"embeddings": vecs,
	})
}
