package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	httppkg "tavernagent/internal/adapters/http"
	"tavernagent/internal/adapters/tts"
)

func TestTTSVoicesEndpoint(t *testing.T) {
	ttsSvc := tts.NewService()
	srv, err := httppkg.New(httppkg.Deps{
		TTSService: ttsSvc,
		Addr:       "127.0.0.1:8890",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/tts/voices", nil)
	req.Host = "127.0.0.1:8890"
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Voices []tts.Voice `json:"voices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse voices response: %v", err)
	}
	if len(resp.Voices) == 0 {
		t.Errorf("expected voices in response, got 0")
	}
}

func TestTTSSynthesizeEndpoint(t *testing.T) {
	ttsSvc := tts.NewService()
	ttsSvc.SetMock(func(voice, text string) ([]byte, error) {
		return []byte("mp3_mock_data_for_" + text), nil
	})

	srv, err := httppkg.New(httppkg.Deps{
		TTSService: ttsSvc,
		Addr:       "127.0.0.1:8890",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	payload := []byte(`{"voice":"zh-CN-XiaoxiaoNeural","text":"你好"}`)
	req := httptest.NewRequest("POST", "/api/v1/tts/synthesize", bytes.NewReader(payload))
	req.Host = "127.0.0.1:8890"
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:8890")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "audio/mpeg" {
		t.Errorf("expected Content-Type audio/mpeg, got %s", ct)
	}
	if rec.Body.String() != "mp3_mock_data_for_你好" {
		t.Errorf("unexpected audio body: %s", rec.Body.String())
	}
}

func TestTTSSynthesizeEmptyText(t *testing.T) {
	ttsSvc := tts.NewService()
	srv, err := httppkg.New(httppkg.Deps{
		TTSService: ttsSvc,
		Addr:       "127.0.0.1:8890",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	payload := []byte(`{"voice":"zh-CN-XiaoxiaoNeural","text":"   "}`)
	req := httptest.NewRequest("POST", "/api/v1/tts/synthesize", bytes.NewReader(payload))
	req.Host = "127.0.0.1:8890"
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:8890")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTTSSynthesizeWithOptions(t *testing.T) {
	ttsSvc := tts.NewService()
	ttsSvc.SetMock(func(voice, text string) ([]byte, error) {
		return []byte("custom_voice_" + voice + ":" + text), nil
	})

	srv, err := httppkg.New(httppkg.Deps{
		TTSService: ttsSvc,
		Addr:       "127.0.0.1:8890",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	payload := []byte(`{"engine":"openai","voice":"custom-voice-1","baseUrl":"http://localhost:9880/v1","text":"角色台词测试","speed":1.2}`)
	req := httptest.NewRequest("POST", "/api/v1/tts/synthesize", bytes.NewReader(payload))
	req.Host = "127.0.0.1:8890"
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:8890")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "custom_voice_custom-voice-1:角色台词测试" {
		t.Errorf("unexpected audio body: %s", rec.Body.String())
	}
}

