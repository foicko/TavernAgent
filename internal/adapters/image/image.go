package image

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GenerateOptions 描述图像生成参数。
type GenerateOptions struct {
	Engine         string `json:"engine,omitempty"` // "openai" | "sd-webui"
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negativePrompt,omitempty"`
	Size           string `json:"size,omitempty"` // "1024x576", "1024x1024", "576x1024"
	BaseURL        string `json:"baseUrl,omitempty"`
	APIKey         string `json:"apiKey,omitempty"`
	Model          string `json:"model,omitempty"`
	Quality        string `json:"quality,omitempty"`
	Style          string `json:"style,omitempty"`
}

// GeneratedImage 描述已生成并落盘的图片元信息。
type GeneratedImage struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Prompt    string `json:"prompt"`
	MimeType  string `json:"mimeType"`
	CreatedAt string `json:"createdAt"`
}

// Service 负责与各生图引擎对接并将图片持久化落盘至本地存储。
type Service struct {
	dataDir string
	mu      sync.RWMutex
	mockFn  func(opts GenerateOptions) (*GeneratedImage, error)
	client  *http.Client
}

// NewService 创建生图服务实例，确保目标图片存储目录存在。
func NewService(dataDir string) (*Service, error) {
	if dataDir == "" {
		dataDir = "data/images"
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create images dir: %w", err)
	}

	return &Service{
		dataDir: dataDir,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

// SetMock 用于单元测试中注入模拟生成行为。
func (s *Service) SetMock(fn func(opts GenerateOptions) (*GeneratedImage, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mockFn = fn
}

// Generate 根据参数调用对应引擎生成图片并保存至本地文件系统。
func (s *Service) Generate(ctx context.Context, opts GenerateOptions) (*GeneratedImage, error) {
	prompt := strings.TrimSpace(opts.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt cannot be empty")
	}

	s.mu.RLock()
	mock := s.mockFn
	s.mu.RUnlock()
	if mock != nil {
		return mock(opts)
	}

	engine := strings.ToLower(strings.TrimSpace(opts.Engine))
	if engine == "" {
		if strings.Contains(strings.ToLower(opts.BaseURL), "8188") || strings.Contains(strings.ToLower(opts.Model), "qwen") {
			engine = "comfyui"
		} else if strings.Contains(strings.ToLower(opts.BaseURL), "7860") || strings.Contains(strings.ToLower(opts.BaseURL), "sdapi") {
			engine = "sd-webui"
		} else if strings.Contains(strings.ToLower(opts.BaseURL), "openai") || strings.Contains(strings.ToLower(opts.BaseURL), "siliconflow") {
			engine = "openai"
		} else {
			engine = "comfyui"
		}
	}

	var imgBytes []byte
	var mimeType string
	var err error

	if engine == "sd-webui" {
		imgBytes, mimeType, err = s.generateSDWebUI(ctx, opts)
	} else if engine == "comfyui" || engine == "comfy" {
		imgBytes, mimeType, err = s.generateComfyUI(ctx, opts)
	} else {
		imgBytes, mimeType, err = s.generateOpenAI(ctx, opts)
	}

	if err != nil {
		return nil, err
	}

	return s.saveImage(imgBytes, mimeType, prompt)
}

func (s *Service) generateOpenAI(ctx context.Context, opts GenerateOptions) ([]byte, string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	endpoint := baseURL
	if !strings.HasSuffix(endpoint, "/images/generations") {
		endpoint = baseURL + "/images/generations"
	}

	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = "dall-e-3"
	}

	size := strings.TrimSpace(opts.Size)
	if size == "" {
		size = "1024x1024"
	}

	payload := map[string]any{
		"model":           model,
		"prompt":          opts.Prompt,
		"n":               1,
		"size":            size,
		"response_format": "b64_json",
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("marshal openai image request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, "", fmt.Errorf("create openai image request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("image provider returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, "", fmt.Errorf("parse image response: %w", err)
	}

	if len(parsed.Data) == 0 {
		return nil, "", fmt.Errorf("image provider returned empty data array")
	}

	item := parsed.Data[0]
	if item.B64JSON != "" {
		decoded, err := base64.StdEncoding.DecodeString(item.B64JSON)
		if err != nil {
			return nil, "", fmt.Errorf("decode b64_json: %w", err)
		}
		return decoded, "image/png", nil
	}

	if item.URL != "" {
		// 拉取图片 URL 转存为本地文件
		getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
		if err != nil {
			return nil, "", fmt.Errorf("create image download request: %w", err)
		}
		getResp, err := s.client.Do(getReq)
		if err != nil {
			return nil, "", fmt.Errorf("download image from url %s: %w", item.URL, err)
		}
		defer getResp.Body.Close()

		if getResp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("download image returned HTTP %d", getResp.StatusCode)
		}

		downloaded, err := io.ReadAll(io.LimitReader(getResp.Body, 32*1024*1024))
		if err != nil {
			return nil, "", fmt.Errorf("read downloaded image: %w", err)
		}
		contentType := getResp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "image/png"
		}
		return downloaded, contentType, nil
	}

	return nil, "", fmt.Errorf("neither b64_json nor url found in response")
}

func (s *Service) generateSDWebUI(ctx context.Context, opts GenerateOptions) ([]byte, string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:7860"
	}

	endpoint := baseURL
	if !strings.HasSuffix(endpoint, "/sdapi/v1/txt2img") {
		endpoint = baseURL + "/sdapi/v1/txt2img"
	}

	width := 1024
	height := 576
	if parts := strings.Split(opts.Size, "x"); len(parts) == 2 {
		if w, err := strconv.Atoi(parts[0]); err == nil && w > 0 {
			width = w
		}
		if h, err := strconv.Atoi(parts[1]); err == nil && h > 0 {
			height = h
		}
	}

	payload := map[string]any{
		"prompt":          opts.Prompt,
		"negative_prompt": opts.NegativePrompt,
		"width":           width,
		"height":          height,
		"steps":           25,
		"cfg_scale":       7.0,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("marshal sd-webui request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, "", fmt.Errorf("create sd-webui request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("sd-webui returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Images []string `json:"images"`
	}

	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, "", fmt.Errorf("parse sd-webui response: %w", err)
	}

	if len(parsed.Images) == 0 {
		return nil, "", fmt.Errorf("sd-webui returned no images")
	}

	imgBase64 := parsed.Images[0]
	// 部分 WebUI 返回带前缀 data:image/png;base64,
	if idx := strings.Index(imgBase64, ","); idx != -1 {
		imgBase64 = imgBase64[idx+1:]
	}

	decoded, err := base64.StdEncoding.DecodeString(imgBase64)
	if err != nil {
		return nil, "", fmt.Errorf("decode sd-webui image: %w", err)
	}

	return decoded, "image/png", nil
}

func (s *Service) generateComfyUI(ctx context.Context, opts GenerateOptions) ([]byte, string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8188"
	}

	aspectRatio := "16:9 (Widescreen)"
	switch opts.Size {
	case "1024x1024":
		aspectRatio = "1:1 (Square)"
	case "576x1024":
		aspectRatio = "9:16 (Portrait Widescreen)"
	case "1024x576":
		aspectRatio = "16:9 (Widescreen)"
	default:
		if opts.Size != "" {
			aspectRatio = "16:9 (Widescreen)"
		}
	}

	var seed int64
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		seed = int64(b[0]) | int64(b[1])<<8 | int64(b[2])<<16 | int64(b[3])<<24 |
			int64(b[4])<<32 | int64(b[5])<<40 | int64(b[6])<<48 | int64(b[7]&0x7f)<<56
		if seed < 0 {
			seed = -seed
		}
	}
	if seed == 0 {
		seed = time.Now().UnixNano() % 1000000000
	}

	promptGraph := map[string]any{
		"13": map[string]any{
			"inputs": map[string]any{
				"aspect_ratio": aspectRatio,
				"megapixels":   1.0,
				"multiple":     8,
			},
			"class_type": "ResolutionSelector",
			"_meta":      map[string]any{"title": "Resolution Selector"},
		},
		"461": map[string]any{
			"inputs": map[string]any{
				"filename_prefix":          "Qwen_image_2.1",
				"format":                   "png",
				"format.bit_depth":         "8-bit",
				"format.input_color_space": "sRGB",
				"images":                   []any{"459:457", 0},
			},
			"class_type": "SaveImageAdvanced",
			"_meta":      map[string]any{"title": "Save Image"},
		},
		"459:451": map[string]any{
			"inputs": map[string]any{
				"unet_name":    "qwen_image_2.1_int8_convrot.safetensors",
				"weight_dtype": "default",
			},
			"class_type": "UNETLoader",
			"_meta":      map[string]any{"title": "UNet Loader"},
		},
		"459:452": map[string]any{
			"inputs": map[string]any{
				"prompt":          opts.Prompt,
				"negative_prompt": opts.NegativePrompt,
				"resolution":      1024,
				"clip":            []any{"459:453", 0},
			},
			"class_type": "TextEncodeQwenImage21",
			"_meta":      map[string]any{"title": "Text Encode Qwen Image 2.1"},
		},
		"459:453": map[string]any{
			"inputs": map[string]any{
				"clip_name": "qwen3vl_8b_w4a8.safetensors",
				"type":      "qwen_image",
				"device":    "default",
			},
			"class_type": "CLIPLoader",
			"_meta":      map[string]any{"title": "CLIP Loader"},
		},
		"459:454": map[string]any{
			"inputs": map[string]any{
				"vae_name": "qwen_image_2.1_vae_bf16.safetensors",
			},
			"class_type": "VAELoader",
			"_meta":      map[string]any{"title": "VAE Loader"},
		},
		"459:456": map[string]any{
			"inputs": map[string]any{
				"width":      []any{"13", 0},
				"height":     []any{"13", 1},
				"batch_size": 1,
			},
			"class_type": "EmptyLatentImage",
			"_meta":      map[string]any{"title": "Empty Latent Image"},
		},
		"459:457": map[string]any{
			"inputs": map[string]any{
				"samples": []any{"459:458", 0},
				"vae":     []any{"459:454", 0},
			},
			"class_type": "VAEDecode",
			"_meta":      map[string]any{"title": "VAE Decode"},
		},
		"459:458": map[string]any{
			"inputs": map[string]any{
				"seed":         seed,
				"steps":        18,
				"cfg":          1.0,
				"sampler_name": "euler",
				"scheduler":    "simple",
				"denoise":      1.0,
				"model":        []any{"459:451", 0},
				"positive":     []any{"459:452", 0},
				"negative":     []any{"459:452", 1},
				"latent_image": []any{"459:456", 0},
			},
			"class_type": "KSampler",
			"_meta":      map[string]any{"title": "KSampler"},
		},
	}

	clientID := "tavernagent-" + hex.EncodeToString(b[:4])
	queueReq := map[string]any{
		"prompt":    promptGraph,
		"client_id": clientID,
	}

	queueBytes, err := json.Marshal(queueReq)
	if err != nil {
		return nil, "", fmt.Errorf("marshal comfyui prompt request: %w", err)
	}

	promptURL := baseURL + "/prompt"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, promptURL, bytes.NewReader(queueBytes))
	if err != nil {
		return nil, "", fmt.Errorf("create comfyui prompt request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("request to %s failed: %w", promptURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		return nil, "", fmt.Errorf("comfyui returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var promptResp struct {
		PromptID   string         `json:"prompt_id"`
		NodeErrors map[string]any `json:"node_errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&promptResp); err != nil {
		return nil, "", fmt.Errorf("decode comfyui prompt response: %w", err)
	}

	if len(promptResp.NodeErrors) > 0 {
		errBytes, _ := json.Marshal(promptResp.NodeErrors)
		return nil, "", fmt.Errorf("comfyui workflow error: %s", string(errBytes))
	}

	if promptResp.PromptID == "" {
		return nil, "", fmt.Errorf("comfyui returned empty prompt_id")
	}

	// 轮询 /history/{prompt_id} 等待生成完成
	historyURL := baseURL + "/history/" + promptResp.PromptID
	pollTicker := time.NewTicker(800 * time.Millisecond)
	defer pollTicker.Stop()

	pollTimeout := time.After(180 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-pollTimeout:
			return nil, "", fmt.Errorf("comfyui generation timed out after 180s")
		case <-pollTicker.C:
			histReq, err := http.NewRequestWithContext(ctx, http.MethodGet, historyURL, nil)
			if err != nil {
				continue
			}
			if opts.APIKey != "" {
				histReq.Header.Set("Authorization", "Bearer "+opts.APIKey)
			}
			hResp, err := s.client.Do(histReq)
			if err != nil {
				continue
			}
			var histData map[string]struct {
				Outputs map[string]struct {
					Images []struct {
						Filename  string `json:"filename"`
						Subfolder string `json:"subfolder"`
						Type      string `json:"type"`
					} `json:"images"`
				} `json:"outputs"`
				Status struct {
					StatusStr string `json:"status_str"`
					Completed bool   `json:"completed"`
				} `json:"status"`
			}
			decodeErr := json.NewDecoder(hResp.Body).Decode(&histData)
			hResp.Body.Close()
			if decodeErr != nil {
				continue
			}

			item, ok := histData[promptResp.PromptID]
			if !ok {
				continue
			}

			if item.Status.StatusStr == "error" {
				return nil, "", fmt.Errorf("comfyui generation execution failed")
			}

			var foundFilename, foundSubfolder, foundType string
			for _, nodeOutput := range item.Outputs {
				if len(nodeOutput.Images) > 0 {
					img := nodeOutput.Images[0]
					foundFilename = img.Filename
					foundSubfolder = img.Subfolder
					foundType = img.Type
					break
				}
			}

			if foundFilename == "" {
				if item.Status.Completed {
					return nil, "", fmt.Errorf("comfyui generation completed but no output image found")
				}
				continue
			}

			viewURL := fmt.Sprintf("%s/view?filename=%s&subfolder=%s&type=%s",
				baseURL, foundFilename, foundSubfolder, foundType)
			viewReq, err := http.NewRequestWithContext(ctx, http.MethodGet, viewURL, nil)
			if err != nil {
				return nil, "", fmt.Errorf("create comfyui view request: %w", err)
			}
			if opts.APIKey != "" {
				viewReq.Header.Set("Authorization", "Bearer "+opts.APIKey)
			}

			vResp, err := s.client.Do(viewReq)
			if err != nil {
				return nil, "", fmt.Errorf("fetch comfyui image from %s: %w", viewURL, err)
			}
			defer vResp.Body.Close()

			if vResp.StatusCode != http.StatusOK {
				return nil, "", fmt.Errorf("fetch comfyui image returned HTTP %d", vResp.StatusCode)
			}

			imgBytes, err := io.ReadAll(io.LimitReader(vResp.Body, 64*1024*1024))
			if err != nil {
				return nil, "", fmt.Errorf("read comfyui image body: %w", err)
			}

			mimeType := "image/png"
			if strings.HasSuffix(strings.ToLower(foundFilename), ".webp") {
				mimeType = "image/webp"
			} else if strings.HasSuffix(strings.ToLower(foundFilename), ".jpg") || strings.HasSuffix(strings.ToLower(foundFilename), ".jpeg") {
				mimeType = "image/jpeg"
			}

			return imgBytes, mimeType, nil
		}
	}
}

func (s *Service) saveImage(data []byte, mimeType, prompt string) (*GeneratedImage, error) {
	var randomBuf [8]byte
	_, _ = rand.Read(randomBuf[:])
	id := fmt.Sprintf("img_%d_%s", time.Now().Unix(), hex.EncodeToString(randomBuf[:]))

	ext := ".png"
	if strings.Contains(mimeType, "webp") {
		ext = ".webp"
	} else if strings.Contains(mimeType, "jpeg") || strings.Contains(mimeType, "jpg") {
		ext = ".jpg"
	}

	filename := id + ext
	filePath := filepath.Join(s.dataDir, filename)

	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return nil, fmt.Errorf("write image file: %w", err)
	}

	return &GeneratedImage{
		ID:        id,
		URL:       "/api/v1/images/" + filename,
		Prompt:    prompt,
		MimeType:  mimeType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// GetImageFile 根据文件名读取已保存的图片文件，附带路径安全校验。
func (s *Service) GetImageFile(filename string) ([]byte, string, error) {
	cleanName := filepath.Base(filename)
	if cleanName == "." || cleanName == "/" || cleanName == "\\" || strings.Contains(filename, "..") {
		return nil, "", fmt.Errorf("invalid filename")
	}

	filePath := filepath.Join(s.dataDir, cleanName)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", err
	}

	mimeType := "image/png"
	if strings.HasSuffix(cleanName, ".webp") {
		mimeType = "image/webp"
	} else if strings.HasSuffix(cleanName, ".jpg") || strings.HasSuffix(cleanName, ".jpeg") {
		mimeType = "image/jpeg"
	}

	return data, mimeType, nil
}
