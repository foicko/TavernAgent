package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"tavernagent/internal/adapters/image"
)

func (s *Server) generateImage(w http.ResponseWriter, r *http.Request) {
	var req image.GenerateOptions
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body", false, "")
		return
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "prompt cannot be empty", false, "")
		return
	}

	imgSvc := s.imageService
	if imgSvc == nil {
		var err error
		imgSvc, err = image.NewService("data/images")
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "IMAGE_SERVICE_UNAVAILABLE", err.Error(), true, "")
			return
		}
	}

	res, err := imgSvc.Generate(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "IMAGE_GENERATION_FAILED", err.Error(), true, "")
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (s *Server) serveImageFile(w http.ResponseWriter, r *http.Request) {
	filename := strings.TrimPrefix(r.URL.Path, "/api/v1/images/")
	filename = strings.TrimSpace(filename)
	if filename == "" {
		http.NotFound(w, r)
		return
	}

	imgSvc := s.imageService
	if imgSvc == nil {
		var err error
		imgSvc, err = image.NewService("data/images")
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}

	data, mimeType, err := imgSvc.GetImageFile(filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
