package tts

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Voice 表示可用的 TTS 音色配置。
type Voice struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Gender      string `json:"gender"`
	Locale      string `json:"locale"`
	Description string `json:"description"`
}

// DefaultVoices 返回推荐的高品质预设音色列表。
func DefaultVoices() []Voice {
	return []Voice{
		{
			ID:          "zh-CN-XiaoxiaoNeural",
			Name:        "晓晓 (温柔女声)",
			Gender:      "female",
			Locale:      "zh-CN",
			Description: "温暖亲切、抑扬顿挫，极适于对白与细腻情感演绎",
		},
		{
			ID:          "zh-CN-YunxiNeural",
			Name:        "云希 (少年/青年男声)",
			Gender:      "male",
			Locale:      "zh-CN",
			Description: "清爽活泼、富有张力，适合少年或年轻角色",
		},
		{
			ID:          "zh-CN-YunjianNeural",
			Name:        "云健 (沉稳旁白男声)",
			Gender:      "male",
			Locale:      "zh-CN",
			Description: "浑厚沉着、庄重大气，适合宏大世界叙述与长篇旁白",
		},
		{
			ID:          "zh-CN-XiaoyiNeural",
			Name:        "晓伊 (抒情女声)",
			Gender:      "female",
			Locale:      "zh-CN",
			Description: "柔美宁静、娓娓道来，适合心声独白与诗意情境",
		},
		{
			ID:          "en-US-AvaMultilingualNeural",
			Name:        "Ava (多语言英文女声)",
			Gender:      "female",
			Locale:      "en-US",
			Description: "现代自然、清晰流畅的多语言通用女声",
		},
		{
			ID:          "en-US-AndrewMultilingualNeural",
			Name:        "Andrew (多语言英文男声)",
			Gender:      "male",
			Locale:      "en-US",
			Description: "成熟沉稳的多语言通用男声",
		},
	}
}

const (
	edgeWSEndpoint   = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1?TrustedClientToken=6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	edgeOrigin       = "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold"
	edgeUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0"
	defaultMaxTokens = 2000
)

// Service 负责 TTS 语音合成与内存缓存。
type Service struct {
	mu       sync.RWMutex
	cache    map[string][]byte
	mockFn   func(voice, text string) ([]byte, error)
	wsDialer *websocket.Dialer
}

// NewService 创建 TTS 语音服务实例。
func NewService() *Service {
	return &Service{
		cache: make(map[string][]byte),
		wsDialer: &websocket.Dialer{
			HandshakeTimeout: 8 * time.Second,
		},
	}
}

// SetMock 用于单元测试注入固定音频或模拟离线错误。
func (s *Service) SetMock(fn func(voice, text string) ([]byte, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mockFn = fn
}

// Voices 返回当前支持的音色列表。
func (s *Service) Voices() []Voice {
	return DefaultVoices()
}

// SynthesizeOptions 包含 TTS 引擎选择、自定义地址、模型与语速。
type SynthesizeOptions struct {
	Engine  string  `json:"engine,omitempty"`  // "edge" | "openai"
	Voice   string  `json:"voice,omitempty"`
	Text    string  `json:"text"`
	BaseURL string  `json:"baseUrl,omitempty"` // 例如 "http://127.0.0.1:9880/v1" 或 "https://api.openai.com/v1"
	APIKey  string  `json:"apiKey,omitempty"`
	Model   string  `json:"model,omitempty"`   // 例如 "tts-1" 或本地模型标识
	Speed   float64 `json:"speed,omitempty"`   // 0.5 ~ 2.0
}

// Synthesize 将指定文本合成为 MP3 音频字节流（默认使用 Edge-TTS）。
func (s *Service) Synthesize(ctx context.Context, voice, text string) ([]byte, error) {
	return s.SynthesizeWithOptions(ctx, SynthesizeOptions{
		Voice: voice,
		Text:  text,
	})
}

// SynthesizeWithOptions 根据配置选择 Edge-TTS 或 OpenAI 兼容本地/在线服务。
func (s *Service) SynthesizeWithOptions(ctx context.Context, opts SynthesizeOptions) ([]byte, error) {
	text := strings.TrimSpace(opts.Text)
	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	engine := strings.ToLower(strings.TrimSpace(opts.Engine))
	if engine == "" {
		if strings.TrimSpace(opts.BaseURL) != "" {
			engine = "openai"
		} else {
			engine = "edge"
		}
	}

	voice := opts.Voice
	if voice == "" {
		if engine == "openai" {
			voice = "alloy"
		} else {
			voice = "zh-CN-XiaoxiaoNeural"
		}
	}

	speed := opts.Speed
	if speed <= 0 {
		speed = 1.0
	}

	cachePrefix := fmt.Sprintf("%s:%s:%.2f:%s", engine, voice, speed, opts.BaseURL)
	h := md5.Sum([]byte(cachePrefix + ":" + text))
	key := hex.EncodeToString(h[:])

	s.mu.RLock()
	if cached, ok := s.cache[key]; ok {
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	s.mu.RLock()
	mock := s.mockFn
	s.mu.RUnlock()
	if mock != nil {
		data, err := mock(voice, text)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.cache[key] = data
		s.mu.Unlock()
		return data, nil
	}

	var data []byte
	var err error
	if engine == "openai" {
		data, err = s.synthesizeOpenAI(ctx, opts, voice, text, speed)
	} else {
		data, err = s.synthesizeEdgeWS(ctx, voice, text)
	}
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	if len(s.cache) > 500 { // 简单的防溢出清理
		s.cache = make(map[string][]byte)
	}
	s.cache[key] = data
	s.mu.Unlock()
	return data, nil
}

func (s *Service) synthesizeOpenAI(ctx context.Context, opts SynthesizeOptions, voice, text string, speed float64) ([]byte, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	endpoint := baseURL
	if !strings.HasSuffix(endpoint, "/audio/speech") {
		endpoint = baseURL + "/audio/speech"
	}

	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = "tts-1"
	}

	payload := map[string]any{
		"model": model,
		"input": text,
		"voice": voice,
		"speed": speed,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("tts request returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
}

func (s *Service) synthesizeEdgeWS(ctx context.Context, voice, text string) ([]byte, error) {
	reqHeader := http.Header{}
	reqHeader.Set("Origin", edgeOrigin)
	reqHeader.Set("User-Agent", edgeUserAgent)

	conn, _, err := s.wsDialer.DialContext(ctx, edgeWSEndpoint, reqHeader)
	if err != nil {
		return nil, fmt.Errorf("connect to Edge-TTS service failed: %w", err)
	}
	defer conn.Close()

	// 1. 发送 speech.config
	configMsg := "Content-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n" +
		`{"context":{"synthesis":{"audio":{"metadataoptions":{"sentenceBoundaryEnabled":"false","wordBoundaryEnabled":"false"},"outputFormat":"audio-24khz-48kbitrate-mono-mp3"}}}}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(configMsg)); err != nil {
		return nil, fmt.Errorf("send speech.config failed: %w", err)
	}

	// 2. 发送 SSML
	requestID := fmt.Sprintf("%032x", time.Now().UnixNano())
	ssml := fmt.Sprintf("<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='zh-CN'>"+
		"<voice name='%s'><prosody pitch='+0Hz' rate='+0%%'>%s</prosody></voice></speak>",
		html.EscapeString(voice), html.EscapeString(text))

	ssmlMsg := fmt.Sprintf("X-RequestId:%s\r\nContent-Type:application/ssml+xml\r\nPath:ssml\r\n\r\n%s", requestID, ssml)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(ssmlMsg)); err != nil {
		return nil, fmt.Errorf("send ssml failed: %w", err)
	}

	// 3. 接收分块音频流直到 turn.end
	var audioBuf bytes.Buffer
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			if audioBuf.Len() > 0 {
				return audioBuf.Bytes(), nil
			}
			return nil, fmt.Errorf("read tts message failed: %w", err)
		}

		if msgType == websocket.TextMessage {
			str := string(msg)
			if strings.Contains(str, "Path:turn.end") {
				break
			}
		} else if msgType == websocket.BinaryMessage {
			if len(msg) >= 2 {
				headerLen := int(binary.BigEndian.Uint16(msg[:2]))
				if len(msg) > 2+headerLen {
					audioData := msg[2+headerLen:]
					audioBuf.Write(audioData)
				}
			}
		}
	}

	if audioBuf.Len() == 0 {
		return nil, fmt.Errorf("no audio stream received from TTS service")
	}
	return audioBuf.Bytes(), nil
}
