// Package http 的角色卡库 HTTP 面：导入即入库、列表、取用与移除。
//
// 卡库是本次改造的核心：在此之前「导入」只解析不落盘，前端把卡片存进浏览器
// localStorage，于是跨设备/跨 origin 互相看不见、清缓存即丢。现在服务端成为
// 卡库唯一真相，localStorage 仅作离线缓存。
package http

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
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
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}", s.security(s.deleteCard))
}

// importCard 解析角色卡并写入卡库，返回标准化预览与 cardId。
// 支持 multipart/form-data 文件上传，也支持原始二进制或 JSON 请求体。
func (s *Server) importCard(w http.ResponseWriter, r *http.Request) {
	data, ok := readCardUpload(w, r)
	if !ok {
		return
	}
	rep, err := parseCardData(data)
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

// readCardUpload 读取卡片上传体（multipart 或原始字节），空内容按 400 处理。
// 返回的 bool 表示是否可以继续处理。
func readCardUpload(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCardUploadBytes)
	var data []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(maxCardUploadBytes); err != nil {
			writeError(w, 400, "BAD_REQUEST", "解析表单数据失败: "+err.Error(), false, "")
			return nil, false
		}
		file, ok := cardFormFile(r)
		if !ok {
			writeError(w, 400, "BAD_REQUEST", "表单未包含 file 或 card 文件字段", false, "")
			return nil, false
		}
		defer file.Close()
		var err error
		data, err = io.ReadAll(file)
		if err != nil {
			writeError(w, 400, "BAD_REQUEST", "读取卡片文件失败: "+err.Error(), false, "")
			return nil, false
		}
	} else {
		var err error
		data, err = io.ReadAll(r.Body)
		if err != nil {
			writeError(w, 400, "BAD_REQUEST", "读取请求体失败: "+err.Error(), false, "")
			return nil, false
		}
	}
	if len(bytes.TrimSpace(data)) == 0 {
		writeError(w, 400, "BAD_REQUEST", "角色卡内容为空", false, "")
		return nil, false
	}
	return data, true
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

// parseCardData 按 PNG 魔数选择解析器。
func parseCardData(data []byte) (*application.ImportReport, error) {
	if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return application.ImportCardPNG(data)
	}
	return application.ImportCardJSON(string(data))
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
	if entry != nil {
		body["cardId"] = entry.CardID
		body["shortName"] = entry.ShortName
		body["role"] = entry.Role
	}
	return body
}
