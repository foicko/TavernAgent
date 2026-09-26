package http

// 演播与插画计划端点（架构设计 §2.4、§2.5）。
//
// HTTP 层只做协议转换：解析请求、调用应用层的计划服务、回写结果。
// "哪些块要念、情绪从哪来、缓存键怎么算、提示词怎么拼"全部在 application 里。
import (
	"net/http"

	"tavernagent/internal/application"
)

// ttsPlanRequest 是 POST /api/v1/tts/plan 的请求体。
//
// blockIndexes 与 mode 之外的字段都是"裁剪依据"必需的：
//   - mode（dialogue/full）：决定来源文本范围，对应客户端 dialogueOnly 设置；
//   - director：是否走 MiMo 导演模式（注入标签与指令）；
//   - engine/voice/baseUrl/speed：客户端 TTS 设置，只参与 cacheKey 计算，
//     使两端与后端合成缓存共用同一个键（此前三处各写一遍该元组）。
type ttsPlanRequest struct {
	SessionID    string  `json:"sessionId"`
	BranchID     string  `json:"branchId,omitempty"`
	NodeID       string  `json:"nodeId"`
	BlockIndexes []int   `json:"blockIndexes,omitempty"`
	Mode         string  `json:"mode,omitempty"`
	Director     bool    `json:"director,omitempty"`
	Engine       string  `json:"engine,omitempty"`
	Voice        string  `json:"voice,omitempty"`
	BaseURL      string  `json:"baseUrl,omitempty"`
	Speed        float64 `json:"speed,omitempty"`
}

func (s *Server) planTTS(w http.ResponseWriter, r *http.Request) {
	if s.ttsPlans == nil {
		writeError(w, 501, "NOT_IMPLEMENTED", "未启用 TTS 演播计划服务", false, "")
		return
	}
	var req ttsPlanRequest
	if err := decodeJSON(w, r, &req, maxRequestBytes); err != nil {
		writeBodyError(w, err)
		return
	}
	plan, err := s.ttsPlans.Plan(application.TTSPlanRequest{
		SessionID: req.SessionID, BranchID: req.BranchID, NodeID: req.NodeID,
		BlockIndexes: req.BlockIndexes, Mode: req.Mode, Director: req.Director,
		Engine: req.Engine, Voice: req.Voice, BaseURL: req.BaseURL, Speed: req.Speed,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// imagePlanRequest 是 POST /api/v1/images/plan 的请求体。
type imagePlanRequest struct {
	SessionID     string `json:"sessionId"`
	BranchID      string `json:"branchId,omitempty"`
	NodeID        string `json:"nodeId"`
	BlockIndexes  []int  `json:"blockIndexes,omitempty"`
	StylePreset   string `json:"stylePreset,omitempty"`
	CharacterName string `json:"characterName,omitempty"`
}

func (s *Server) planImage(w http.ResponseWriter, r *http.Request) {
	if s.imagePlans == nil {
		writeError(w, 501, "NOT_IMPLEMENTED", "未启用插画提示词计划服务", false, "")
		return
	}
	var req imagePlanRequest
	if err := decodeJSON(w, r, &req, maxRequestBytes); err != nil {
		writeBodyError(w, err)
		return
	}
	plan, err := s.imagePlans.Plan(application.ImagePlanRequest{
		SessionID: req.SessionID, BranchID: req.BranchID, NodeID: req.NodeID,
		BlockIndexes: req.BlockIndexes, StylePreset: req.StylePreset, CharacterName: req.CharacterName,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}
