package http

// 会话的 HTTP 面：创建 / 列表 / 视图 / 分支 / 删除 / 剧情包与角色卡导入。
//
// 从 server.go 抽出来：server.go 的体量早已需要用文件划分来收敛（架构门禁的
// file-lines 棘轮），而这些 handler 正好是一个内聚的整体。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"tavernagent/internal/application"
	"tavernagent/internal/pack"
)

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	// 建会话会把整张角色卡（含头像与世界书）一起提交，额度必须与导入接口对齐。
	if err := decodeJSON(w, r, &req, maxCardJSONBytes); err != nil {
		writeBodyError(w, err)
		return
	}
	if req.CharacterJSON == "" {
		writeError(w, 422, "CARD_MISSING", "缺少角色卡", false, "")
		return
	}
	res, err := s.sessions.Setup(r.Context(), &application.SessionSetupRequest{
		IdempotencyKey:   req.IdempotencyKey,
		Title:            req.Title,
		CharacterJSON:    req.CharacterJSON,
		Player:           application.Player{Name: req.PlayerName, Role: req.PlayerRole, Backpack: req.PlayerBackpack},
		OpeningVariantID: req.OpeningVariantID,
		OpeningText:      req.OpeningText,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	stateJSON, _ := res.State.Marshal()
	writeJSON(w, 201, map[string]any{
		"sessionId":     res.Session.SessionID,
		"title":         res.Session.Title,
		"rootNodeId":    res.RootNode.NodeID,
		"branchId":      res.Branch.BranchID,
		"branchName":    res.Branch.Name,
		"branchVersion": res.Branch.Version,
		"openingText":   res.OpeningText,
		"state":         json.RawMessage(stateJSON),
	})
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.sessions.Sessions()
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"sessions": sessions})
}

// deleteSession 永久删除一个会话及其全部故事数据。不可撤销，前端负责与用户确认；
// 仍有进行中的回合时返回 409 SESSION_BUSY。

// deleteSession 永久删除一个会话及其全部故事数据。不可撤销，前端负责与用户确认；
// 仍有进行中的回合时返回 409 SESSION_BUSY。
func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	if err := s.sessions.DeleteSession(r.Context(), r.PathValue("id")); err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// getGraph 返回以目标节点为中心的子图（M4e）。
// 万级会话不能整树下发——"图谱可交互 p95 ≤ 500ms"的前提就是只取局部。
func (s *Server) getLorebook(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	offset, limit := 0, 40
	for _, field := range []struct {
		name  string
		value *int
	}{{"offset", &offset}, {"limit", &limit}} {
		if raw := q.Get(field.name); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 {
				writeError(w, 400, "INVALID_QUERY", "世界书分页参数无效", false, "")
				return
			}
			*field.value = n
		}
	}
	page, err := s.sessions.Lorebook(r.PathValue("id"), q.Get("q"), offset, limit)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, page)
}

func (s *Server) getGraph(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	up, down := 6, 2
	if raw := q.Get("up"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			up = n
		}
	}
	if raw := q.Get("down"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			down = n
		}
	}
	target := q.Get("nodeId")
	if target == "" {
		// 默认以当前查看/写入位置为中心。
		view, err := s.sessions.View(r.PathValue("id"), application.ViewQuery{BranchID: q.Get("branchId")})
		if err != nil {
			writeAPIError(w, err)
			return
		}
		target = view.ViewNodeID
	}
	g, err := s.sessions.Graph(r.PathValue("id"), target, up, down)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, g)
}

// getSession 返回会话视图。
// 写入位置（branchId）与查看位置（viewNodeId）分离（技术契约 §7）；
// 组装逻辑在应用层，handler 只做协议转换。

// getSession 返回会话视图。
// 写入位置（branchId）与查看位置（viewNodeId）分离（技术契约 §7）；
// 组装逻辑在应用层，handler 只做协议转换。
func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 0
	if raw := q.Get("limit"); raw != "" {
		n, perr := strconv.Atoi(raw)
		if perr != nil || n < 0 {
			writeError(w, 400, "BAD_REQUEST", "limit 必须是非负整数", false, "")
			return
		}
		limit = n
	}
	view, err := s.sessions.View(r.PathValue("id"), application.ViewQuery{
		BranchID:   q.Get("branchId"),
		ViewNodeID: q.Get("viewNodeId"),
		Limit:      limit,
		Before:     q.Get("before"),
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, view)
}

func (s *Server) listBranches(w http.ResponseWriter, r *http.Request) {
	branches, err := s.sessions.Branches(r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"branches": branches})
}

// importSession 导入剧情包并返回新会话。
//
// 校验与写入都在应用层完成：任何一步失败都不会留下半截数据（T22）。
func (s *Server) importSession(w http.ResponseWriter, r *http.Request) {
	if s.archive == nil {
		writeError(w, 503, "STORAGE_UNAVAILABLE", "剧情包服务不可用", true, "")
		return
	}
	// 先按硬上限截断读取：不能因为包声称自己很大就把内存吃满。
	body, err := io.ReadAll(io.LimitReader(r.Body, pack.MaxPackBytes+1))
	if err != nil {
		writeError(w, 400, "BAD_REQUEST", "读取请求体失败: "+err.Error(), false, "")
		return
	}
	if int64(len(body)) > pack.MaxPackBytes {
		writeError(w, 413, "PACK_TOO_LARGE", "剧情包超过大小上限", false, "")
		return
	}
	res, err := s.archive.Import(r.Context(), body)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{
		"sessionId":  res.Session.SessionID,
		"title":      res.Session.Title,
		"rootNodeId": res.Session.RootNodeID,
		"scope":      res.Manifest.Scope,
		"counts":     res.Manifest.Counts,
		"assets":     res.Assets,
	})
}

// importCard 服务端统一角色卡导入接口（G2 技术债治理）。
// 支持 multipart/form-data 文件上传，也支持原始二进制或 JSON 请求体。
// 纯 Go 解析 PNG chunk (tEXt/iTXt/zTXt) 或原生/V2/V3 JSON，返回标准化角色卡与兼容报告。

// importCard 服务端统一角色卡导入接口（G2 技术债治理）。
// 支持 multipart/form-data 文件上传，也支持原始二进制或 JSON 请求体。
// 纯 Go 解析 PNG chunk (tEXt/iTXt/zTXt) 或原生/V2/V3 JSON，返回标准化角色卡与兼容报告。
func (s *Server) importCard(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCardUploadBytes)

	var data []byte
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(maxCardUploadBytes); err != nil {
			writeError(w, 400, "BAD_REQUEST", "解析表单数据失败: "+err.Error(), false, "")
			return
		}
		var file io.Reader
		if f, _, err := r.FormFile("file"); err == nil {
			defer f.Close()
			file = f
		} else if f, _, err := r.FormFile("card"); err == nil {
			defer f.Close()
			file = f
		} else {
			writeError(w, 400, "BAD_REQUEST", "表单未包含 file 或 card 文件字段", false, "")
			return
		}
		var err error
		data, err = io.ReadAll(file)
		if err != nil {
			writeError(w, 400, "BAD_REQUEST", "读取卡片文件失败: "+err.Error(), false, "")
			return
		}
	} else {
		var err error
		data, err = io.ReadAll(r.Body)
		if err != nil {
			writeError(w, 400, "BAD_REQUEST", "读取请求体失败: "+err.Error(), false, "")
			return
		}
	}

	if len(bytes.TrimSpace(data)) == 0 {
		writeError(w, 400, "BAD_REQUEST", "角色卡内容为空", false, "")
		return
	}

	var rep *application.ImportReport
	var err error
	if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		rep, err = application.ImportCardPNG(data)
	} else {
		rep, err = application.ImportCardJSON(string(data))
	}
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

	writeJSON(w, 200, map[string]any{
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
		"characterJson":           cardJSON,
		"card":                    rep.Card,
		"openings":                openings,
	})
}

// sanitizeFileName 生成安全的下载文件名（剔除路径分隔符与控制字符并限长）。
func sanitizeFileName(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return "session"
	}
	repl := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_", "\n", "", "\r", "",
	)
	out := []rune(repl.Replace(t))
	if len(out) > 40 {
		out = out[:40]
	}
	return string(out)
}

// contentDisposition 同时给出 ASCII 回退名与 RFC 5987 的 UTF-8 名：
// 只写中文文件名会被部分客户端直接丢弃。
func contentDisposition(name string) string {
	ascii := make([]rune, 0, len(name))
	for _, r := range name {
		if r < 128 && r != '"' && r != '\\' {
			ascii = append(ascii, r)
		} else {
			ascii = append(ascii, '_')
		}
	}
	return "attachment; filename=\"" + string(ascii) + "\"; filename*=UTF-8''" + url.PathEscape(name)
}

// requireBranch 校验分支存在且属于该会话（防止跨会话操作）。
// requireBranch 的实现已移到应用层（SessionService.RequireBranch）：
// 分支归属是业务规则，不该由适配器自己查库判断。

// getNode 读取单个节点（inspect X）：只读，不推进任何分支（技术契约 §7）。
