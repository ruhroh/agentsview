package server

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/wesm/agentsview/internal/db"
)

// incomingShare is the JSON body received from a local instance
// pushing a shared session to this hosted server.
type incomingShare struct {
	ShareID  string          `json:"share_id"`
	Session  incomingSession `json:"session"`
	Messages []db.Message    `json:"messages"`
}

// incomingSession is the session metadata in a share push.
// Local-only file metadata (file_path, file_size, file_mtime,
// file_hash) is intentionally absent.
type incomingSession struct {
	ID                string  `json:"id"`
	Project           string  `json:"project"`
	Machine           string  `json:"machine"`
	Agent             string  `json:"agent"`
	FirstMessage      *string `json:"first_message"`
	DisplayName       *string `json:"display_name"`
	StartedAt         *string `json:"started_at"`
	EndedAt           *string `json:"ended_at"`
	MessageCount      int     `json:"message_count"`
	UserMessageCount  int     `json:"user_message_count"`
	ParentSessionID   *string `json:"parent_session_id"`
	RelationshipType  string  `json:"relationship_type"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	PeakContextTokens int     `json:"peak_context_tokens"`
}

// handleUpsertShare receives a shared conversation from a local
// instance and stores it. The session is stored under id = shareId
// so it is namespaced by publisher. Messages are replaced
// atomically. Local-only file metadata is always null.
func (s *Server) handleUpsertShare(
	w http.ResponseWriter, r *http.Request,
) {
	shareID := r.PathValue("shareId")
	if shareID == "" {
		writeError(w, http.StatusBadRequest, "missing share_id")
		return
	}

	var body incomingShare
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Validate the share_id matches the URL path.
	if body.ShareID != "" && body.ShareID != shareID {
		writeError(w, http.StatusBadRequest,
			"share_id in body does not match URL")
		return
	}

	// Store the session with id = shareID. This namespaces sessions
	// by publisher and avoids collisions with sessions from other
	// publishers.
	sess := db.Session{
		ID:                shareID,
		Project:           body.Session.Project,
		Machine:           body.Session.Machine,
		Agent:             body.Session.Agent,
		FirstMessage:      body.Session.FirstMessage,
		DisplayName:       body.Session.DisplayName,
		StartedAt:         body.Session.StartedAt,
		EndedAt:           body.Session.EndedAt,
		MessageCount:      body.Session.MessageCount,
		UserMessageCount:  body.Session.UserMessageCount,
		ParentSessionID:   body.Session.ParentSessionID,
		RelationshipType:  body.Session.RelationshipType,
		TotalOutputTokens: body.Session.TotalOutputTokens,
		PeakContextTokens: body.Session.PeakContextTokens,
		// Explicitly nil: FilePath, FileSize, FileMtime, FileHash
	}

	if err := s.db.UpsertSession(sess); err != nil {
		if handleReadOnly(w, err) {
			return
		}
		log.Printf("upsert share session %s: %v", shareID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Replace messages atomically. Rewrite session_id to shareID.
	for i := range body.Messages {
		body.Messages[i].SessionID = shareID
	}
	if err := s.db.ReplaceSessionMessages(shareID, body.Messages); err != nil {
		if handleReadOnly(w, err) {
			return
		}
		log.Printf("replace share messages %s: %v", shareID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteShare removes a shared session from the hosted server.
func (s *Server) handleDeleteShare(
	w http.ResponseWriter, r *http.Request,
) {
	shareID := r.PathValue("shareId")
	if shareID == "" {
		writeError(w, http.StatusBadRequest, "missing share_id")
		return
	}

	// Check the session exists.
	session, err := s.db.GetSession(r.Context(), shareID)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		log.Printf("delete share lookup %s: %v", shareID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "share not found")
		return
	}

	// Soft-delete the session.
	if err := s.db.SoftDeleteSession(shareID); err != nil {
		if handleReadOnly(w, err) {
			return
		}
		log.Printf("delete share %s: %v", shareID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
