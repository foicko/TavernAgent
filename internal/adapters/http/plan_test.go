package http

// 演播/插画计划端点：走真实 HTTP 路径，覆盖路由接线、安全层与响应形状。

import (
	"encoding/json"
	"strings"
	"testing"
)

// planSession 建一个带开场白的会话，返回 sessionId / 分支 / 根节点。
func planSession(t *testing.T, base string) (sessionID, rootNodeID string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"title": "演播接口", "playerName": "旅人",
		"openingText":   "「你终于来了。」外面下着雨。",
		"characterJson": testCard,
	})
	status, resp := postJSON(t, base+"/api/v1/sessions", string(body))
	if status != 201 {
		t.Fatalf("create session status = %d body=%s", status, resp)
	}
	var created struct {
		SessionID string `json:"sessionId"`
		RootNode  string `json:"rootNodeId"`
	}
	if err := json.Unmarshal([]byte(resp), &created); err != nil || created.SessionID == "" || created.RootNode == "" {
		t.Fatalf("解析建会话响应失败: %s (%v)", resp, err)
	}
	return created.SessionID, created.RootNode
}

func TestTTSPlanEndpoint(t *testing.T) {
	_, base := newTestServer(t)
	sessionID, rootID := planSession(t, base)

	// 只读台词：从纯文本开场白里按引号提取对白。
	status, resp := postJSON(t, base+"/api/v1/tts/plan",
		`{"sessionId":"`+sessionID+`","nodeId":"`+rootID+`","mode":"dialogue"}`)
	if status != 200 {
		t.Fatalf("tts/plan status = %d body=%s", status, resp)
	}
	var plan struct {
		Segments []struct {
			SegmentID      string `json:"segmentId"`
			SpeakerName    string `json:"speakerName"`
			Text           string `json:"text"`
			Emotion        string `json:"emotion"`
			ContextSummary string `json:"contextSummary"`
			CacheKey       string `json:"cacheKey"`
		} `json:"segments"`
		VoiceHint struct {
			PresetID string  `json:"presetId"`
			Speed    float64 `json:"speed"`
		} `json:"voiceHint"`
	}
	if err := json.Unmarshal([]byte(resp), &plan); err != nil {
		t.Fatalf("解析计划: %v (%s)", err, resp)
	}
	if len(plan.Segments) != 1 || plan.Segments[0].Text != "你终于来了。" {
		t.Fatalf("dialogue 分段 = %+v", plan.Segments)
	}
	if plan.Segments[0].SegmentID == "" || !strings.HasPrefix(plan.Segments[0].CacheKey, "sha256:") {
		t.Fatalf("分段缺少标识/缓存键: %+v", plan.Segments[0])
	}
	if plan.VoiceHint.PresetID == "" || plan.VoiceHint.Speed != 1.0 {
		t.Fatalf("voiceHint = %+v", plan.VoiceHint)
	}

	// 整段朗读：开场白原文一段。
	status, resp = postJSON(t, base+"/api/v1/tts/plan",
		`{"sessionId":"`+sessionID+`","nodeId":"`+rootID+`","mode":"full"}`)
	if status != 200 {
		t.Fatalf("tts/plan(full) status = %d body=%s", status, resp)
	}
	if !strings.Contains(resp, "外面下着雨") {
		t.Fatalf("full 模式应包含旁白: %s", resp)
	}

	// 缺少 nodeId → 400（应用层业务校验，不是 500）。
	if status, _ := postJSON(t, base+"/api/v1/tts/plan", `{"sessionId":"`+sessionID+`"}`); status != 400 {
		t.Fatalf("缺少 nodeId 应 400, 实得 %d", status)
	}
}

func TestImagePlanEndpoint(t *testing.T) {
	_, base := newTestServer(t)
	sessionID, rootID := planSession(t, base)

	status, resp := postJSON(t, base+"/api/v1/images/plan",
		`{"sessionId":"`+sessionID+`","nodeId":"`+rootID+`","stylePreset":"anime"}`)
	if status != 200 {
		t.Fatalf("images/plan status = %d body=%s", status, resp)
	}
	var plan struct {
		Prompt         string `json:"prompt"`
		NegativePrompt string `json:"negativePrompt"`
		CardID         string `json:"cardId"`
		SdDataBase     struct {
			Prompts []string `json:"prompts"`
			Text    string   `json:"text"`
		} `json:"sdDataBase"`
		Refs []struct {
			AssetKey string `json:"assetKey"`
		} `json:"refs"`
	}
	if err := json.Unmarshal([]byte(resp), &plan); err != nil {
		t.Fatalf("解析计划: %v (%s)", err, resp)
	}
	if !strings.Contains(plan.Prompt, "你终于来了") ||
		!strings.Contains(plan.Prompt, "masterpiece, best quality, anime visual novel CG") {
		t.Fatalf("prompt = %s", plan.Prompt)
	}
	if plan.NegativePrompt == "" || plan.CardID != "card_elena" {
		t.Fatalf("negativePrompt/cardId = %q / %q", plan.NegativePrompt, plan.CardID)
	}
	if plan.SdDataBase.Prompts == nil || plan.Refs == nil {
		t.Fatalf("空基底/引用必须是空数组: %s", resp)
	}

	if status, _ := postJSON(t, base+"/api/v1/images/plan", `{"nodeId":""}`); status != 400 {
		t.Fatalf("缺少 nodeId 应 400, 实得 %d", status)
	}
}
