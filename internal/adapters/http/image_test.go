package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	httppkg "tavernagent/internal/adapters/http"
	"tavernagent/internal/adapters/image"
)

func TestImageGenerateEndpoint(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tavernagent_http_img_test_*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	imgSvc, err := image.NewService(tempDir)
	if err != nil {
		t.Fatalf("create img service: %v", err)
	}

	imgSvc.SetMock(func(opts image.GenerateOptions) (*image.GeneratedImage, error) {
		return &image.GeneratedImage{
			ID:        "img_mock_123",
			URL:       "/api/v1/images/img_mock_123.png",
			Prompt:    opts.Prompt,
			MimeType:  "image/png",
			CreatedAt: "2026-09-21T12:00:00Z",
		}, nil
	})

	srv, err := httppkg.New(httppkg.Deps{
		ImageService: imgSvc,
		Addr:         "127.0.0.1:8890",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	payload := []byte(`{"prompt":"anime girl in tavern","engine":"openai"}`)
	req := httptest.NewRequest("POST", "/api/v1/images/generate", bytes.NewReader(payload))
	req.Host = "127.0.0.1:8890"
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:8890")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp image.GeneratedImage
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.ID != "img_mock_123" {
		t.Errorf("expected ID img_mock_123, got %s", resp.ID)
	}
	if resp.URL != "/api/v1/images/img_mock_123.png" {
		t.Errorf("expected URL /api/v1/images/img_mock_123.png, got %s", resp.URL)
	}
}

func TestServeImageFileEndpoint(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tavernagent_http_img_serve_test_*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Write a test image file
	testFilePath := filepath.Join(tempDir, "sample_img.png")
	if err := os.WriteFile(testFilePath, []byte("fake_png_content"), 0o644); err != nil {
		t.Fatalf("write sample image: %v", err)
	}

	imgSvc, err := image.NewService(tempDir)
	if err != nil {
		t.Fatalf("create img service: %v", err)
	}

	srv, err := httppkg.New(httppkg.Deps{
		ImageService: imgSvc,
		Addr:         "127.0.0.1:8890",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/images/sample_img.png", nil)
	req.Host = "127.0.0.1:8890"
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("expected Content-Type image/png, got %s", ct)
	}
	if rec.Body.String() != "fake_png_content" {
		t.Errorf("expected body 'fake_png_content', got '%s'", rec.Body.String())
	}

	// 404 for nonexistent image
	req404 := httptest.NewRequest("GET", "/api/v1/images/not_exists.png", nil)
	rec404 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec404, req404)
	if rec404.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing image, got %d", rec404.Code)
	}
}
