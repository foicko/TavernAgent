package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"tavernagent/internal/adapters/tts"
)

type ttsSynthesizeRequest struct {
	Voice       string  `json:"voice"`
	Text        string  `json:"text"`
	Instruction string  `json:"instruction,omitempty"`
	Engine      string  `json:"engine,omitempty"`
	BaseURL     string  `json:"baseUrl,omitempty"`
	APIKey      string  `json:"apiKey,omitempty"`
	Model       string  `json:"model,omitempty"`
	Speed       float64 `json:"speed,omitempty"`
}

func (s *Server) listTTSVoices(w http.ResponseWriter, r *http.Request) {
	if s.ttsService == nil {
		writeJSON(w, http.StatusOK, map[string]any{"voices": tts.DefaultVoices()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"voices": s.ttsService.Voices()})
}

func (s *Server) synthesizeTTS(w http.ResponseWriter, r *http.Request) {
	var req ttsSynthesizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body", false, "")
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "text cannot be empty", false, "")
		return
	}
	if len([]rune(text)) > 1000 {
		writeError(w, http.StatusBadRequest, "TEXT_TOO_LONG", "text length exceeds maximum limit of 1000 characters", false, "")
		return
	}

	ttsSvc := s.ttsService
	if ttsSvc == nil {
		ttsSvc = tts.NewService()
	}

	audio, err := ttsSvc.SynthesizeWithOptions(r.Context(), tts.SynthesizeOptions{
		Engine:      req.Engine,
		Voice:       req.Voice,
		Text:        text,
		Instruction: req.Instruction,
		BaseURL:     req.BaseURL,
		APIKey:      req.APIKey,
		Model:       req.Model,
		Speed:       req.Speed,
	})
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "TTS_UNAVAILABLE", err.Error(), true, "")
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio)
}
