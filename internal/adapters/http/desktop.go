package http

import (
	"net/http"
	"strings"
)

// 桌面壳与网页前端之间的最小桥。
//
// 为什么需要这条桥：桌面壳（cmd/tavernagent-desktop）把窗口加载在本地 loopback
// 服务上，而不是 Wails 的资源服务主机名上。原因是 WebView2 的资源响应必须**整包
// 交付**（wails 的 responsewriter_windows.go 用 bytes.Buffer 攒完，等 handler 返回
// 才 PutByteContent），SSE 在那里根本无法流式送达：fetch 连响应头都收不到，既不
// 报错也不重连，于是"生成中"的增量与"后台已更新"的通知全部静默丢失。
//
// 代价是页面里没有 Wails 注入的 window.runtime，原生窗口配色只能由前端通过同源
// 接口回传。浏览器部署没有这个回调，端点依旧存在但拒绝服务——前端据此判断自己
// 是否跑在桌面壳里。
func (s *Server) registerDesktopRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/desktop", s.security(s.desktopInfo))
	mux.HandleFunc("POST /api/v1/desktop/theme", s.security(s.setDesktopTheme))
}

// desktopInfo 让前端认出自己跑在桌面壳里（纯浏览器部署返回 desktop=false）。
func (s *Server) desktopInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"desktop": s.setNativeTheme != nil})
}

// setDesktopTheme 切换原生标题栏/边框配色。
//
// 只接受 light / dark 两个值：窗口配色是显示偏好，不是可扩展配置面，
// 放开取值只会把"未知模式"的判断题丢给下游。
func (s *Server) setDesktopTheme(w http.ResponseWriter, r *http.Request) {
	if s.setNativeTheme == nil {
		writeError(w, 404, "NOT_FOUND", "当前部署没有原生窗口", false, "")
		return
	}
	var body struct {
		Mode string `json:"mode"`
	}
	if err := decodeJSON(w, r, &body, maxRequestBytes); err != nil {
		writeBodyError(w, err)
		return
	}
	mode := strings.ToLower(strings.TrimSpace(body.Mode))
	if mode != "light" && mode != "dark" {
		writeError(w, 400, "BAD_REQUEST", "mode 只能是 light 或 dark", false, "")
		return
	}
	s.setNativeTheme(mode)
	w.WriteHeader(http.StatusNoContent)
}
