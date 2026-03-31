package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/wesm/agentsview/internal/db"
)

// sharePayload is the JSON body pushed to the remote share server.
type sharePayload struct {
	ShareID  string       `json:"share_id"`
	Session  shareSession `json:"session"`
	Messages []db.Message `json:"messages"`
}

// shareSession is the session metadata included in a share push,
// with local-only file metadata stripped.
type shareSession struct {
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

// shareConfig returns the current share configuration (thread-safe).
func (s *Server) shareConfig() (url, token, publisher string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Share.URL, s.cfg.Share.Token, s.cfg.Share.Publisher
}

func (s *Server) handleListShared(
	w http.ResponseWriter, r *http.Request,
) {
	ids, err := s.db.ListSharedSessionIDs(r.Context())
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		log.Printf("list shared: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_ids": ids,
	})
}

func (s *Server) handleShareSession(
	w http.ResponseWriter, r *http.Request,
) {
	serverURL, token, publisher := s.shareConfig()
	if serverURL == "" {
		writeError(w, http.StatusBadRequest,
			"share server not configured (set share.url in config)")
		return
	}
	if token == "" {
		writeError(w, http.StatusBadRequest,
			"share token not configured (set share.token in config)")
		return
	}
	if publisher == "" {
		writeError(w, http.StatusBadRequest,
			"share publisher not configured (set share.publisher in config)")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}

	session, err := s.db.GetSession(r.Context(), id)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		log.Printf("share session lookup: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	msgs, err := s.db.GetAllMessages(r.Context(), id)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		log.Printf("share session messages: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	shareID := publisher + ":" + id

	// Build the share payload with local-only metadata stripped.
	payload := sharePayload{
		ShareID: shareID,
		Session: shareSession{
			ID:                session.ID,
			Project:           session.Project,
			Machine:           publisher,
			Agent:             session.Agent,
			FirstMessage:      session.FirstMessage,
			DisplayName:       session.DisplayName,
			StartedAt:         session.StartedAt,
			EndedAt:           session.EndedAt,
			MessageCount:      session.MessageCount,
			UserMessageCount:  session.UserMessageCount,
			ParentSessionID:   session.ParentSessionID,
			RelationshipType:  session.RelationshipType,
			TotalOutputTokens: session.TotalOutputTokens,
			PeakContextTokens: session.PeakContextTokens,
		},
		Messages: msgs,
	}

	// Push to the remote share server.
	if err := pushShare(r.Context(), serverURL, token, shareID, payload); err != nil {
		log.Printf("share push failed for %s: %v", id, err)
		writeError(w, http.StatusBadGateway,
			fmt.Sprintf("share server error: %v", err))
		return
	}

	// Record the share locally.
	if err := s.db.RecordShare(id, shareID, serverURL); err != nil {
		if handleReadOnly(w, err) {
			return
		}
		log.Printf("record share: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	share, err := s.db.GetShare(r.Context(), id)
	if err != nil {
		log.Printf("share readback: %v", err)
	}
	writeJSON(w, http.StatusOK, share)
}

func (s *Server) handleUnshareSession(
	w http.ResponseWriter, r *http.Request,
) {
	serverURL, token, _ := s.shareConfig()

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}

	// Look up the existing share record.
	share, err := s.db.GetShare(r.Context(), id)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		log.Printf("unshare lookup: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if share == nil {
		writeError(w, http.StatusNotFound, "session is not shared")
		return
	}

	// Best-effort remote delete using the share record's server URL.
	remoteURL := serverURL
	if share.ServerURL != "" {
		remoteURL = share.ServerURL
	}
	if remoteURL != "" && token != "" {
		if err := deleteShare(r.Context(), remoteURL, token, share.ShareID); err != nil {
			log.Printf("remote unshare for %s: %v (continuing with local removal)", id, err)
		}
	}

	// Remove local record.
	if err := s.db.RemoveShare(id); err != nil {
		if handleReadOnly(w, err) {
			return
		}
		log.Printf("remove share: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// bestEffortUnshare attempts to unshare a session before deletion.
// Errors are logged but do not block the caller.
func (s *Server) bestEffortUnshare(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	share, err := s.db.GetShare(ctx, sessionID)
	if err != nil || share == nil {
		return
	}

	serverURL, token, _ := s.shareConfig()
	remoteURL := serverURL
	if share.ServerURL != "" {
		remoteURL = share.ServerURL
	}
	if remoteURL != "" && token != "" {
		if err := deleteShare(ctx, remoteURL, token, share.ShareID); err != nil {
			log.Printf("best-effort unshare %s: %v", sessionID, err)
		}
	}

	if err := s.db.RemoveShare(sessionID); err != nil {
		log.Printf("best-effort remove share record %s: %v", sessionID, err)
	}
}

// pushShare sends the share payload to the remote server.
func pushShare(
	ctx context.Context, serverURL, token, shareID string,
	payload sharePayload,
) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling share payload: %w", err)
	}

	url := serverURL + "/api/v1/shares/" + shareID
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating share request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "agentsview")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("share request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("share server error %d: %s",
			resp.StatusCode, string(respBody))
	}
	return nil
}

// deleteShare removes a share from the remote server.
func deleteShare(
	ctx context.Context, serverURL, token, shareID string,
) error {
	url := serverURL + "/api/v1/shares/" + shareID
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("creating delete share request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "agentsview")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("delete share request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("delete share error %d: %s",
			resp.StatusCode, string(respBody))
	}
	return nil
}
