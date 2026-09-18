package http

// 会话的 HTTP 面：创建 / 列表 / 视图 / 分支 / 删除 / 剧情包与角色卡导入。
//
// 从 server.go 抽出来：server.go 的体量早已需要用文件划分来收敛（架构门禁的
// file-lines 棘轮），而这些 handler 正好是一个内聚的整体。

import (
	"encoding/json"
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
	if req.CharacterJSON == "" && req.CardID == "" {
		writeError(w, 422, "CARD_MISSING", "缺少角色卡", false, "")
		return
	}
	res, err := s.sessions.Setup(r.Context(), &application.SessionSetupRequest{
		IdempotencyKey:   req.IdempotencyKey,
		Title:            req.Title,
		CardID:           req.CardID,
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
