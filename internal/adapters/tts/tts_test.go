package tts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVoicesList(t *testing.T) {
	svc := NewService()
	voices := svc.Voices()
	if len(voices) == 0 {
		t.Fatalf("expected default voices, got none")
	}
	foundXiaoxiao := false
	for _, v := range voices {
		if v.ID == "zh-CN-XiaoxiaoNeural" {
			foundXiaoxiao = true
			break
		}
	}
	if !foundXiaoxiao {
		t.Errorf("expected zh-CN-XiaoxiaoNeural in voices list")
	}
}

func TestSynthesizeWithMockAndCache(t *testing.T) {
	svc := NewService()
	called := 0
	svc.SetMock(func(voice, text string) ([]byte, error) {
		called++
		return []byte("fake_mp3_data_" + voice + "_" + text), nil
	})

	ctx := context.Background()
	data1, err := svc.Synthesize(ctx, "zh-CN-XiaoxiaoNeural", "你好，冒险者。")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data1) != "fake_mp3_data_zh-CN-XiaoxiaoNeural_你好，冒险者。" {
		t.Errorf("unexpected audio data: %s", string(data1))
	}
	if called != 1 {
		t.Errorf("expected called 1, got %d", called)
	}

	// Second call should hit cache and not invoke mock function
	data2, err := svc.Synthesize(ctx, "zh-CN-XiaoxiaoNeural", "你好，冒险者。")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data2) != string(data1) {
		t.Errorf("expected cached data identical to original")
	}
	if called != 1 {
		t.Errorf("expected called to still be 1 (cache hit), got %d", called)
	}
}

func TestSynthesizeEmptyText(t *testing.T) {
	svc := NewService()
	_, err := svc.Synthesize(context.Background(), "zh-CN-XiaoxiaoNeural", "   ")
	if err == nil {
		t.Errorf("expected error on empty text, got nil")
	}
}

func TestSynthesizeOpenAIEndpoint(t *testing.T) {
	receivedAuth := ""
	receivedModel := ""
	receivedVoice := ""
	receivedText := ""

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Errorf("expected path /v1/audio/speech, got %s", r.URL.Path)
		}
		receivedAuth = r.Header.Get("Authorization")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		receivedModel, _ = body["model"].(string)
		receivedVoice, _ = body["voice"].(string)
		receivedText, _ = body["input"].(string)

		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mock_openai_audio_bytes"))
	}))
	defer ts.Close()

	svc := NewService()
	ctx := context.Background()
	opts := SynthesizeOptions{
		Engine:  "openai",
		BaseURL: ts.URL + "/v1",
		APIKey:  "sk-testkey123",
		Model:   "tts-1-hd",
		Voice:   "shimmer",
		Text:    "台词对白测试",
		Speed:   1.2,
	}

	data, err := svc.SynthesizeWithOptions(ctx, opts)
	if err != nil {
		t.Fatalf("unexpected synthesize error: %v", err)
	}
	if string(data) != "mock_openai_audio_bytes" {
		t.Errorf("expected 'mock_openai_audio_bytes', got '%s'", string(data))
	}
	if receivedAuth != "Bearer sk-testkey123" {
		t.Errorf("expected auth 'Bearer sk-testkey123', got '%s'", receivedAuth)
	}
	if receivedModel != "tts-1-hd" {
		t.Errorf("expected model tts-1-hd, got %s", receivedModel)
	}
	if receivedVoice != "shimmer" {
		t.Errorf("expected voice shimmer, got %s", receivedVoice)
	}
	if receivedText != "台词对白测试" {
		t.Errorf("expected text '台词对白测试', got '%s'", receivedText)
	}
}

