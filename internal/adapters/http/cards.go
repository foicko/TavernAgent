// Package http 的角色卡库 HTTP 面：导入即入库、列表、取用与移除。
//
// 卡库是本次改造的核心：在此之前「导入」只解析不落盘，前端把卡片存进浏览器
// localStorage，于是跨设备/跨 origin 互相看不见、清缓存即丢。现在服务端成为
// 卡库唯一真相，localStorage 仅作离线缓存。
package http

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"tavernagent/internal/application"
	"tavernagent/internal/domain"
)

// registerCardRoutes 注册卡库路由。
//
// /cards/import 是兼容别名：旧前端与既有契约测试走这条路径，其语义已升级为
// 「导入并入库」，与新路径完全一致。
func (s *Server) registerCardRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/cards", s.security(s.importCard))
	mux.HandleFunc("POST /api/v1/cards/import", s.security(s.importCard))
	mux.HandleFunc("GET /api/v1/cards", s.security(s.listCards))
	mux.HandleFunc("GET /api/v1/cards/{cardId}", s.security(s.getCard))
	// 资源 GET 不加 security：浏览器 <img> 无法携带 Bearer Token，
	// 与 /api/v1/images 保持一致的只读公开策略。
	mux.HandleFunc("GET /api/v1/cards/{cardId}/assets/{name}", s.serveCardAsset)
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}", s.security(s.deleteCard))
}

// importCard 解析角色卡并写入卡库，返回标准化预览与 cardId。
// 支持 multipart/form-data 文件上传，也支持原始二进制或 JSON 请求体。
func (s *Server) importCard(w http.ResponseWriter, r *http.Request) {
	tmpPath, ok := s.readCardUpload(w, r)
	if !ok {
		return
	}
	defer os.Remove(tmpPath)
	rep, err := parseCardFile(tmpPath)
	if err != nil {
		writeError(w, 422, "CARD_INVALID", "角色卡解析失败: "+err.Error(), false, "")
		return
	}
	cardJSON := rep.CardJSON()
	// 归一化后的卡片 JSON 必须能原样交给建会话接口；超出额度就当场说清楚，
	// 而不是让玩家在「开启冒险」那一步收到 413。
	if len(cardJSON) > maxCardJSONBytes {
		writeError(w, 422, "CARD_TOO_LARGE",
			fmt.Sprintf("角色卡体积过大（%.1f MB，上限 %d MB）；请精简世界书条目或改用较小的 PNG",
				float64(len(cardJSON))/(1<<20), maxCardJSONBytes>>20), false, "")
		return
	}
	if s.cards == nil {
		writeError(w, 503, "STORAGE_UNAVAILABLE", "角色卡库服务不可用", true, "")
		return
	}
	entry, err := s.cards.SaveReport(rep)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, cardPreviewBody(rep, entry))
}

// listCards 返回卡库摘要列表（不含兆级 characterJson）。
func (s *Server) listCards(w http.ResponseWriter, r *http.Request) {
	if s.cards == nil {
		writeError(w, 503, "STORAGE_UNAVAILABLE", "角色卡库服务不可用", true, "")
		return
	}
	cards, err := s.cards.List()
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if cards == nil {
		cards = []*domain.CharacterCardEntry{}
	}
	writeJSON(w, 200, map[string]any{"cards": cards})
}

// getCard 返回单张卡的完整内容（含 characterJson）。
func (s *Server) getCard(w http.ResponseWriter, r *http.Request) {
	if s.cards == nil {
		writeError(w, 503, "STORAGE_UNAVAILABLE", "角色卡库服务不可用", true, "")
		return
	}
	entry, err := s.cards.Get(r.PathValue("cardId"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	// 重新解析已归一化的卡，直接返回与导入一致的 CardPreview：
	// 前端「用库里的卡开新局」不必再理解卡 JSON 的内部结构。
	body, perr := cardPreviewFromEntry(entry)
	if perr != nil {
		writeJSON(w, 200, entry)
		return
	}
	writeJSON(w, 200, body)
}

// cardPreviewFromEntry 把卡库条目还原成导入预览。
func cardPreviewFromEntry(entry *domain.CharacterCardEntry) (map[string]any, error) {
	rep, err := application.ImportCardJSON(entry.CharacterJSON)
	if err != nil || rep == nil || rep.Card == nil {
		if err == nil {
			err = fmt.Errorf("卡片内容为空")
		}
		return nil, err
	}
	body := cardPreviewBody(rep, entry)
	// 归一化后的 JSON 会丢失原始 spec 标签，用卡库里的记录保留展示文案。
	if entry.Format != "" {
		body["format"] = entry.Format
	}
	return body, nil
}

// deleteCard 从卡库移除一张卡。不影响任何已有会话（故事内容在模板里）。
func (s *Server) deleteCard(w http.ResponseWriter, r *http.Request) {
	if s.cards == nil {
		writeError(w, 503, "STORAGE_UNAVAILABLE", "角色卡库服务不可用", true, "")
		return
	}
	if err := s.cards.Delete(r.PathValue("cardId")); err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// readCardUpload 把卡片上传体（multipart 或原始字节）流式写入临时文件。
//
// charx 归档可达数百 MB，因此不在内存里整体读取：常量内存 + 落盘，
// 由调用方负责在解析/入库完成后删除返回的临时文件。
// 返回的 bool 表示是否可以继续处理。
func (s *Server) readCardUpload(w http.ResponseWriter, r *http.Request) (string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCardUploadBytes)
	var src io.Reader
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(maxCardUploadMemory); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeError(w, 413, "REQUEST_TOO_LARGE", "角色卡文件超过允许的上限", false, "")
				return "", false
			}
			writeError(w, 400, "BAD_REQUEST", "解析上传表单失败，请重试", false, "")
			return "", false
		}
		file, ok := cardFormFile(r)
		if !ok {
			writeError(w, 400, "BAD_REQUEST", "表单未包含 file 或 card 文件字段", false, "")
			return "", false
		}
		defer file.Close()
		src = file
	} else {
		src = r.Body
	}

	tmp, err := os.CreateTemp("", "tavern-card-*")
	if err != nil {
		writeError(w, 500, "STORAGE_UNAVAILABLE", "无法创建上传临时文件", true, "")
		return "", false
	}
	n, err := io.Copy(tmp, src)
	closeErr := tmp.Close()
	if err != nil {
		_ = os.Remove(tmp.Name())
		// Raw (non-multipart) bodies are bounded by MaxBytesReader above; report the
		// same 413 the multipart path produces instead of a misleading 400.
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, 413, "REQUEST_TOO_LARGE", "角色卡文件超过允许的上限", false, "")
			return "", false
		}
		writeError(w, 400, "BAD_REQUEST", "读取卡片文件失败，请重试", false, "")
		return "", false
	}
	if closeErr != nil {
		_ = os.Remove(tmp.Name())
		writeError(w, 500, "STORAGE_UNAVAILABLE", "写入上传临时文件失败", true, "")
		return "", false
	}
	if n == 0 {
		_ = os.Remove(tmp.Name())
		writeError(w, 400, "BAD_REQUEST", "角色卡内容为空", false, "")
		return "", false
	}
	return tmp.Name(), true
}

// cardFormFile 依次尝试 file / card 两个字段名。
func cardFormFile(r *http.Request) (io.ReadCloser, bool) {
	if f, _, err := r.FormFile("file"); err == nil {
		return f, true
	}
	if f, _, err := r.FormFile("card"); err == nil {
		return f, true
	}
	return nil, false
}

// parseCardFile 按文件魔数选择解析器：PNG / charx(ZIP) / JSON。
func parseCardFile(path string) (*application.ImportReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	head := make([]byte, 8)
	n, _ := io.ReadFull(f, head)
	f.Close()
	switch {
	case n >= 8 && bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		if err := enforceInlineCardLimit(path); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return application.ImportCardPNG(data)
	case n >= 4 && bytes.HasPrefix(head, []byte("PK\x03\x04")):
		return application.ImportCardCharx(path)
	default:
		// 图片头 + ZIP 多形态的 .charx（首字节不是 PK，但尾部有 ZIP 中央目录）：
		// 必须按内容识别，且要先于内存上限判断，否则几十 MB 的归档会被
		// enforceInlineCardLimit 误拒，或作为二进制被当成 JSON 解析失败。
		if application.IsCharxArchive(path) {
			return application.ImportCardCharx(path)
		}
		if err := enforceInlineCardLimit(path); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return application.ImportCardJSON(string(data))
	}
}

// enforceInlineCardLimit 对需要在内存中解析的 PNG/JSON 卡设上限：
// charx 走流式路径不受此限，但把上百 MB 的 PNG/JSON 整体读进内存并不安全。
func enforceInlineCardLimit(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() > maxInlineCardBytes {
		return fmt.Errorf("卡片文件超过 %d MB，请改用 charx 归档或精简内容", maxInlineCardBytes>>20)
	}
	return nil
}

// cardPreviewBody 组装前端 CardPreview（键名必须与 web/src/app/types.ts 对齐）。
func cardPreviewBody(rep *application.ImportReport, entry *domain.CharacterCardEntry) map[string]any {
	openings := make([]map[string]string, 0, len(rep.Card.OpeningVariants))
	for _, ov := range rep.Card.OpeningVariants {
		openings = append(openings, map[string]string{
			"variantId": ov.VariantID,
			"title":     ov.Title,
			"text":      ov.Text,
		})
	}
	avatar := rep.Card.Avatar
	if avatar == "" && len(rep.Card.Characters) > 0 {
		avatar = rep.Card.Characters[0].Avatar
	}
	body := map[string]any{
		"format":       rep.Format,
		"specVersion":  rep.SpecVersion,
		"source":       rep.Source,
		"name":         rep.Card.Name,
		"avatar":       avatar,
		"description":  rep.Card.Description,
		"personality":  rep.Card.Personality,
		"scenario":     rep.Card.Scenario,
		"firstMes":     rep.Card.FirstMes,
		"mesExample":   rep.Card.MesExample,
		"systemPrompt": rep.Card.SystemPrompt,
		// 键名与前端 CardPreview 一致：错位会让预览里的这两个字段恒为空。
		"postHistoryInstructions": rep.Card.PostHistory,
		"creatorNotes":            rep.Card.CreatorNotes,
		"tags":                    rep.Card.Tags,
		"creator":                 rep.Card.Creator,
		"characterVersion":        rep.Card.Version,
		"nickname":                rep.Card.Nickname,
		"supported":               rep.Supported,
		"ignored":                 rep.Ignored,
		"warnings":                rep.Warnings,
		"characterJson":           rep.CardJSON(),
		"card":                    rep.Card,
		"openings":                openings,
	}
	if rep.Card.UIProfile != nil {
		body["capabilities"] = rep.Card.UIProfile.Capabilities
		body["varDefaults"] = rep.Card.UIProfile.VarDefaults
		body["themeTokens"] = rep.Card.UIProfile.ThemeTokens
		body["sdData"] = rep.Card.UIProfile.SdData
	}
	if entry != nil {
		body["cardId"] = entry.CardID
		body["shortName"] = entry.ShortName
		body["role"] = entry.Role
	}
	body["assets"] = cardAssetsBody(rep, entry)
	return body
}

// cardAssetsBody 组装前端渲染 <img cmd> 所需的 name→url 清单。
// 只回传引用名与 URL，绝不回传资源字节（体量留在服务端）。
func cardAssetsBody(rep *application.ImportReport, entry *domain.CharacterCardEntry) []map[string]string {
	if rep == nil || rep.Card == nil || len(rep.Card.Assets) == 0 {
		return []map[string]string{}
	}
	cardID := ""
	if entry != nil {
		cardID = entry.CardID
	}
	out := make([]map[string]string, 0, len(rep.Card.Assets))
	for _, a := range rep.Card.Assets {
		assetURL := strings.TrimSpace(a.URI)
		if a.Ref != "" && cardID != "" {
			assetURL = "/api/v1/cards/" + url.PathEscape(cardID) + "/assets/" + url.PathEscape(a.Ref)
		}
		out = append(out, map[string]string{
			"name": a.Name,
			"type": a.Type,
			"url":  assetURL,
		})
	}
	return out
}

// serveCardAsset 提供卡内嵌资源（charx 立绘/图标）的读取。
func (s *Server) serveCardAsset(w http.ResponseWriter, r *http.Request) {
	if s.cardAssets == nil {
		http.NotFound(w, r)
		return
	}
	rc, mime, err := s.cardAssets.OpenAsset(r.PathValue("cardId"), r.PathValue("name"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}
