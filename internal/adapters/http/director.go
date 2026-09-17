package http

import (
	"net/http"
	"tavernagent/internal/application"
)

func (s *Server) directorRoute(handler http.HandlerFunc) http.HandlerFunc {
	return s.security(func(w http.ResponseWriter, r *http.Request) {
		if s.director == nil {
			writeError(w, 503, "DIRECTOR_UNAVAILABLE", "导演服务尚未启动", true, "")
			return
		}
		handler(w, r)
	})
}

func decodeDirectorBody(w http.ResponseWriter, r *http.Request, out any) bool {
	if err := decodeLimitedJSON(w, r, out, maxRequestBytes, true); err != nil {
		writeBodyError(w, err)
		return false
	}
	return true
}

func (s *Server) getDirector(w http.ResponseWriter, r *http.Request) {
	v, err := s.director.View(r.PathValue("id"), r.PathValue("branchId"), r.URL.Query().Get("viewNodeId"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) saveDirectorDraft(w http.ResponseWriter, r *http.Request) {
	var req application.SaveDirectorDraftRequest
	if !decodeDirectorBody(w, r, &req) {
		return
	}
	v, err := s.director.SaveDraft(r.PathValue("id"), r.PathValue("branchId"), req)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) directorCommand(w http.ResponseWriter, r *http.Request) {
	var req application.DirectorCommandRequest
	if !decodeDirectorBody(w, r, &req) {
		return
	}
	v, err := s.director.Command(r.Context(), r.PathValue("id"), r.PathValue("branchId"), req)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) directorMessage(w http.ResponseWriter, r *http.Request) {
	var req application.DirectorMessageRequest
	if !decodeDirectorBody(w, r, &req) {
		return
	}
	v, err := s.director.Discuss(r.PathValue("id"), r.PathValue("branchId"), req)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	base := "/api/v1/director-requests/" + v.RequestID
	writeJSON(w, 202, map[string]any{"request": v, "requestId": v.RequestID, "status": v.Status, "statusUrl": base, "eventsUrl": base + "/events"})
}
func (s *Server) getDirectorRequest(w http.ResponseWriter, r *http.Request) {
	v, err := s.director.GetRequest(r.PathValue("requestId"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) cancelDirectorRequest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExpectedCharacterID string `json:"expectedCharacterId"`
	}
	if !decodeDirectorBody(w, r, &req) {
		return
	}
	v, err := s.director.CancelRequest(r.PathValue("requestId"), req.ExpectedCharacterID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) directorEvents(w http.ResponseWriter, r *http.Request) {
	if _, err := s.director.GetRequest(r.PathValue("requestId")); err != nil {
		writeAPIError(w, err)
		return
	}
	s.eventStream(w, r, r.PathValue("requestId"))
}
