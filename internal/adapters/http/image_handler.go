package http

import (
	"net/http"
	"strings"

	"tavernagent/internal/adapters/image"
)

func (s *Server) generateImage(w http.ResponseWriter, r *http.Request) {
	var req image.GenerateOptions
	if err := decodeJSON(w, r, &req, maxRequestBytes); err != nil {
		writeBodyError(w, err)
		return
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "prompt 不能为空，请提供画面描述", false, "")
		return
	}

	imgSvc := s.imageService
	if imgSvc == nil {
		var err error
		imgSvc, err = image.NewService("data/images")
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "IMAGE_SERVICE_UNAVAILABLE", "图像服务初始化失败："+err.Error(), true, "")
			return
		}
	}

	res, err := imgSvc.Generate(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "IMAGE_GENERATION_FAILED", "图像生成失败："+err.Error(), true, "")
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
