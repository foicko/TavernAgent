package http

import (
	"net/http"
	"strings"
)

// getCatalog 下发展示目录（架构设计 §2.1）。
//
// 目录是进程内常量，构建一次即可；用内容哈希做 ETag，客户端缓存命中时
// 只回 304，避免每次启动都传一整份枚举。
func (s *Server) getCatalog(w http.ResponseWriter, r *http.Request) {
	version := s.catalog.Version
	if version == "" {
		writeError(w, 503, "CATALOG_UNAVAILABLE", "展示目录不可用", true, "")
		return
	}
	w.Header().Set("ETag", `"`+version+`"`)
	w.Header().Set("Cache-Control", "private, max-age=300")
	if match := strings.TrimSpace(r.Header.Get("If-None-Match")); match != "" && strings.Contains(match, version) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, 200, s.catalog)
}
