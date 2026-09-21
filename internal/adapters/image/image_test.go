package image

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateOpenAIBase64(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tavernagent_image_test_*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mockBase64 := base64.StdEncoding.EncodeToString([]byte("fake_png_binary_data"))
	receivedAuth := ""
	receivedModel := ""

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Errorf("expected path /v1/images/generations, got %s", r.URL.Path)
		}
		receivedAuth = r.Header.Get("Authorization")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		receivedModel, _ = body["model"].(string)

		resp := map[string]any{
			"created": 12345678,
			"data": []map[string]any{
				{"b64_json": mockBase64},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	svc, err := NewService(tempDir)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	res, err := svc.Generate(context.Background(), GenerateOptions{
		Engine:  "openai",
		BaseURL: ts.URL + "/v1",
		APIKey:  "sk-testkey",
		Model:   "dall-e-3",
		Prompt:  "1girl, fantasy anime scenery",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ID == "" {
		t.Errorf("expected generated image ID")
	}
	if receivedAuth != "Bearer sk-testkey" {
		t.Errorf("expected auth 'Bearer sk-testkey', got '%s'", receivedAuth)
	}
	if receivedModel != "dall-e-3" {
		t.Errorf("expected model 'dall-e-3', got '%s'", receivedModel)
	}

	// Verify file was saved and can be retrieved
	filename := filepath.Base(res.URL)
	data, mime, err := svc.GetImageFile(filename)
	if err != nil {
		t.Fatalf("failed to get image file: %v", err)
	}
	if string(data) != "fake_png_binary_data" {
		t.Errorf("unexpected image content: %s", string(data))
	}
	if mime != "image/png" {
		t.Errorf("expected image/png, got %s", mime)
	}
}

func TestGenerateSDWebUI(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tavernagent_image_sd_test_*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mockBase64 := base64.StdEncoding.EncodeToString([]byte("fake_sd_png_data"))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sdapi/v1/txt2img" {
			t.Errorf("expected path /sdapi/v1/txt2img, got %s", r.URL.Path)
		}
		resp := map[string]any{
			"images": []string{mockBase64},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	svc, err := NewService(tempDir)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	res, err := svc.Generate(context.Background(), GenerateOptions{
		Engine:  "sd-webui",
		BaseURL: ts.URL,
		Prompt:  "masterpiece, tavern scene",
		Size:    "1024x576",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	filename := filepath.Base(res.URL)
	data, _, err := svc.GetImageFile(filename)
	if err != nil {
		t.Fatalf("failed to get image file: %v", err)
	}
	if string(data) != "fake_sd_png_data" {
		t.Errorf("unexpected image content: %s", string(data))
	}
}

func TestGetImageFileSecurity(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "tavernagent_sec_test_*")
	defer os.RemoveAll(tempDir)

	svc, _ := NewService(tempDir)

	// Attempt path traversal
	_, _, err := svc.GetImageFile("../../etc/passwd")
	if err == nil {
		t.Errorf("expected error for path traversal attempt, got nil")
	}

	_, _, err = svc.GetImageFile("..\\windows\\system32")
	if err == nil {
		t.Errorf("expected error for windows path traversal, got nil")
	}
}

func TestGenerateComfyUI(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tavernagent_image_comfy_test_*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/prompt" && r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["prompt"] == nil {
				t.Errorf("expected prompt field in request")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"prompt_id":   "prompt-test-abc",
				"node_errors": map[string]any{},
			})
		case r.URL.Path == "/history/prompt-test-abc" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"prompt-test-abc": map[string]any{
					"outputs": map[string]any{
						"461": map[string]any{
							"images": []map[string]any{
								{"filename": "Qwen_image_test_001.png", "subfolder": "", "type": "output"},
							},
						},
					},
					"status": map[string]any{
						"status_str": "success",
						"completed":  true,
					},
				},
			})
		case r.URL.Path == "/view" && r.Method == http.MethodGet:
			if r.URL.Query().Get("filename") != "Qwen_image_test_001.png" {
				t.Errorf("expected filename Qwen_image_test_001.png, got %s", r.URL.Query().Get("filename"))
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("fake_comfy_png_data"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	svc, err := NewService(tempDir)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	res, err := svc.Generate(context.Background(), GenerateOptions{
		Engine:  "comfyui",
		BaseURL: ts.URL,
		Prompt:  "1girl, fantasy anime scenery, qwen image 2.1",
		Size:    "1024x576",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	filename := filepath.Base(res.URL)
	data, _, err := svc.GetImageFile(filename)
	if err != nil {
		t.Fatalf("failed to get image file: %v", err)
	}
	if string(data) != "fake_comfy_png_data" {
		t.Errorf("unexpected image content: %s", string(data))
	}
}

