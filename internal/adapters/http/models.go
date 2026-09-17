package http

// 模型配置 API：模型实例 CRUD + 槽位指派 + 探测。
//
// 契约：实例是唯一携带连接信息的地方；槽位只接受 {slot, enabled, modelId}。
// 密钥只回脱敏值，留空或回传掩码都表示保留原密钥。
import (
	"context"
	"net/http"
	"time"

	"tavernagent/internal/ports"
)

// ---- 模型配置（M1-5，契约缺口补齐：脱敏读取 / 保存 / 连通+格式探测） ----

func (s *Server) listProviderConfigs(w http.ResponseWriter, r *http.Request) {
	if s.manager == nil {
		writeError(w, 501, "NOT_IMPLEMENTED", "未启用运行时模型配置", false, "")
		return
	}
	catalog, err := s.manager.Catalog(true)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"providers": catalog.Slots})
}

func (s *Server) saveProviderConfig(w http.ResponseWriter, r *http.Request) {
	if s.manager == nil {
		writeError(w, 501, "NOT_IMPLEMENTED", "未启用运行时模型配置", false, "")
		return
	}
	var body struct {
		Slot    string  `json:"slot"`
		Enabled bool    `json:"enabled"`
		ModelID string  `json:"modelId"`
		Kind    *string `json:"kind"`
		BaseURL *string `json:"baseUrl"`
		Model   *string `json:"model"`
		APIKey  *string `json:"apiKey"`
	}
	if err := decodeJSON(w, r, &body, maxRequestBytes); err != nil {
		writeBodyError(w, err)
		return
	}
	// 槽位只存引用：连接信息属于模型实例，带旧字段一律拒绝并指路。
	if body.Kind != nil || body.BaseURL != nil || body.Model != nil || body.APIKey != nil {
		writeError(w, 422, "SLOT_TAKES_REFERENCE_ONLY",
			"槽位只接受 enabled 与 modelId；协议、地址、模型名与密钥请保存到模型实例（/api/v1/config/models）", false, "")
		return
	}
	if !validSlot(body.Slot) {
		writeError(w, 422, "BAD_SLOT", "槽位必须是 primary/assist/reflection 之一", false, "")
		return
	}
	if err := s.manager.SaveSlot(body.Slot, body.Enabled, body.ModelID); err != nil {
		writeAPIError(w, err)
		return
	}
	catalog, err := s.manager.Catalog(true)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "providers": catalog.Slots})
}

type probeRequest struct {
	Slot    string `json:"slot"`
	ModelID string `json:"modelId,omitempty"`
	Format  bool   `json:"format,omitempty"`
}

// listModels 返回模型实例列表（密钥脱敏）。
func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	if s.manager == nil {
		writeError(w, 501, "NOT_IMPLEMENTED", "未启用运行时模型配置", false, "")
		return
	}
	catalog, err := s.manager.Catalog(true)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	// 一次返回完整目录（models + slots）：前端 store 需要同时拿到"有哪些实例"
	// 与"三个槽位各自引用谁"，分成两个请求只会让加载时序更难保证。
	writeJSON(w, 200, catalog)
}

// saveModel 新建或更新模型实例（PUT 时以路径 ID 为准）。
func (s *Server) saveModel(w http.ResponseWriter, r *http.Request) {
	if s.manager == nil {
		writeError(w, 501, "NOT_IMPLEMENTED", "未启用运行时模型配置", false, "")
		return
	}
	var instance ports.ModelInstance
	if err := decodeJSON(w, r, &instance, maxRequestBytes); err != nil {
		writeBodyError(w, err)
		return
	}
	if pathID := r.PathValue("modelId"); pathID != "" {
		instance.ID = pathID
	}
	saved, err := s.manager.SaveModel(instance)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, saved)
}

func (s *Server) deleteModel(w http.ResponseWriter, r *http.Request) {
	if s.manager == nil {
		writeError(w, 501, "NOT_IMPLEMENTED", "未启用运行时模型配置", false, "")
		return
	}
	if err := s.manager.DeleteModel(r.PathValue("modelId")); err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) probeModel(w http.ResponseWriter, r *http.Request) {
	if s.manager == nil {
		writeError(w, 501, "NOT_IMPLEMENTED", "未启用运行时模型配置", false, "")
		return
	}
	var req probeRequest
	if r.ContentLength > 0 {
		if err := decodeJSON(w, r, &req, maxRequestBytes); err != nil {
			writeBodyError(w, err)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := s.manager.ProbeModel(ctx, r.PathValue("modelId"), req.Format)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) probeProvider(w http.ResponseWriter, r *http.Request) {
	if s.manager == nil {
		writeError(w, 501, "NOT_IMPLEMENTED", "未启用运行时模型配置", false, "")
		return
	}
	var req probeRequest
	if err := decodeJSON(w, r, &req, maxRequestBytes); err != nil {
		writeBodyError(w, err)
		return
	}
	if !validSlot(req.Slot) {
		req.Slot = string(ports.SlotPrimary)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := s.manager.Probe(ctx, req.Slot, req.Format)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, res)
}
