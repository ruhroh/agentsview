package server

import "net/http"

// handleGetSessionActivity returns time-bucketed message counts.
func (s *Server) handleGetSessionActivity(
	w http.ResponseWriter, r *http.Request,
) {
	sessionID := r.PathValue("id")

	resp, err := s.db.GetSessionActivity(
		r.Context(), sessionID,
	)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(
			w, http.StatusInternalServerError, err.Error(),
		)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
