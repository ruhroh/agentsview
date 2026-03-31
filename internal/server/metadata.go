package server

import (
	"net/http"
)

func (s *Server) handleGetStats(
	w http.ResponseWriter, r *http.Request,
) {
	excludeOneShot := r.URL.Query().Get("include_one_shot") != "true"
	stats, err := s.db.GetStats(r.Context(), excludeOneShot)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleListProjects(
	w http.ResponseWriter, r *http.Request,
) {
	excludeOneShot := r.URL.Query().Get("include_one_shot") != "true"
	projects, err := s.db.GetProjects(r.Context(), excludeOneShot)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"projects": projects,
	})
}

func (s *Server) handleListMachines(
	w http.ResponseWriter, r *http.Request,
) {
	excludeOneShot := r.URL.Query().Get("include_one_shot") != "true"
	machines, err := s.db.GetMachines(r.Context(), excludeOneShot)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"machines": machines,
	})
}

func (s *Server) handleListAgents(
	w http.ResponseWriter, r *http.Request,
) {
	excludeOneShot := r.URL.Query().Get("include_one_shot") != "true"
	agents, err := s.db.GetAgents(r.Context(), excludeOneShot)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"agents": agents,
	})
}
