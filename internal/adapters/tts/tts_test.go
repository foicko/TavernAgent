package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestSynthesizeMiMoEndpoint(t *testing.T) {
	receivedApiKey := ""
	receivedModel := ""
	receivedVoice := ""
	receivedText := ""

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected path /v1/chat/completions, got %s", r.URL.Path)
		}
		receivedApiKey = r.Header.Get("api-key")

		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Audio struct {
				Voice  string `json:"voice"`
				Format string `json:"format"`
			} `json:"audio"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		receivedModel = body.Model
		receivedVoice = body.Audio.Voice
		if len(body.Messages) > 0 {
			receivedText = body.Messages[0].Content
		}

		audioBase64 := base64.StdEncoding.EncodeToString([]byte("mimo_audio_bytes"))
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"audio": map[string]any{
							"data": audioBase64,
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	svc := NewService()
	ctx := context.Background()
	opts := SynthesizeOptions{
		Engine:  "mimo",
		BaseURL: ts.URL + "/v1",
		APIKey:  "sk-mimokey456",
		Model:   "mimo-v2.5-tts",
		Voice:   "冰糖",
		Text:    "欢迎使用小米MiMo语音合成",
	}

	data, err := svc.SynthesizeWithOptions(ctx, opts)
	if err != nil {
		t.Fatalf("unexpected synthesize error: %v", err)
	}
	if string(data) != "mimo_audio_bytes" {
		t.Errorf("expected 'mimo_audio_bytes', got '%s'", string(data))
	}
	if receivedApiKey != "sk-mimokey456" {
		t.Errorf("expected api-key 'sk-mimokey456', got '%s'", receivedApiKey)
	}
	if receivedModel != "mimo-v2.5-tts" {
		t.Errorf("expected model mimo-v2.5-tts, got %s", receivedModel)
	}
	if receivedVoice != "冰糖" {
		t.Errorf("expected voice 冰糖, got %s", receivedVoice)
	}
	if receivedText != "欢迎使用小米MiMo语音合成" {
		t.Errorf("expected text '欢迎使用小米MiMo语音合成', got '%s'", receivedText)
	}
}

func TestSynthesizeMiMoError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":{"code":"402","message":"Insufficient account balance","type":"insufficient_balance"}}`))
	}))
	defer ts.Close()

	svc := NewService()
	ctx := context.Background()
	opts := SynthesizeOptions{
		BaseURL: ts.URL + "/v1",
		APIKey:  "sk-empty-balance",
		Model:   "mimo-v2.5-tts",
		Text:    "测试余额不足",
	}

	_, err := svc.SynthesizeWithOptions(ctx, opts)
	if err == nil {
		t.Fatalf("expected error from 402, got nil")
	}
	if !strings.Contains(err.Error(), "Insufficient account balance") {
		t.Errorf("expected Insufficient account balance in error, got %v", err)
	}
}

func TestSynthesizeMiMoWithInstruction(t *testing.T) {
	var receivedMessages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		receivedMessages = body.Messages

		audioBase64 := base64.StdEncoding.EncodeToString([]byte("director_mimo_audio"))
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"audio": map[string]any{
							"data": audioBase64,
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	svc := NewService()
	ctx := context.Background()
	opts := SynthesizeOptions{
		Engine:      "mimo",
		BaseURL:     ts.URL + "/v1",
		APIKey:      "sk-mimokey",
		Model:       "mimo-v2.5-tts",
		Voice:       "mimo_default",
		Instruction: "【角色】岑家大当家\n【场景】祠堂深处阴影里\n【指导】极慢，实音重，上位者傲慢",
		Text:        "(冷峻)你想带我走？[轻蔑地笑]凭你也配……[停顿片刻]滚出去。",
	}

	data, err := svc.SynthesizeWithOptions(ctx, opts)
	if err != nil {
		t.Fatalf("unexpected synthesize error: %v", err)
	}
	if string(data) != "director_mimo_audio" {
		t.Errorf("expected 'director_mimo_audio', got '%s'", string(data))
	}
	if len(receivedMessages) != 2 {
		t.Fatalf("expected 2 messages (user instruction + assistant dialogue), got %d", len(receivedMessages))
	}
	if receivedMessages[0].Role != "user" || receivedMessages[0].Content != opts.Instruction {
		t.Errorf("expected message[0] to be user instruction, got %+v", receivedMessages[0])
	}
	if receivedMessages[1].Role != "assistant" || receivedMessages[1].Content != opts.Text {
		t.Errorf("expected message[1] to be assistant text, got %+v", receivedMessages[1])
	}
}
