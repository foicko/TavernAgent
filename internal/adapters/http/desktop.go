package http

import (
	"net/http"
	"strings"
)

// 桌面壳与网页前端之间的最小桥（扩展点，当前无壳层消费）。
//
// 为什么保留这条桥：桌面壳若把界面加载在本地 loopback 服务上（而不是自带一套资源
// 主机名），流式（SSE）才能直接送达；由壳层整包交付页面资源时，SSE 的增量会在
// 缓冲里静默丢失——收不到响应头，既不报错也不重连。
//
// 代价是页面里没有壳层注入的原生桥，原生窗口配色只能由前端通过同源接口回传。
// 浏览器部署没有这个回调，端点依旧存在但拒绝服务——前端据此判断自己是否跑在
// 桌面壳里。历史消费方是已移除的 Wails 壳；当前 Avalonia 端（desktop/）自行处理
// 原生材质，不调用它，因此实际总是返回 desktop=false。
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
