package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListShared_Empty(t *testing.T) {
	te := setup(t)
	w := te.get(t, "/api/v1/shared")
	assertStatus(t, w, http.StatusOK)

	var resp struct {
		SessionIDs []string `json:"session_ids"`
	}
	resp = decode[struct {
		SessionIDs []string `json:"session_ids"`
	}](t, w)
	if len(resp.SessionIDs) != 0 {
		t.Errorf("expected empty shared list, got %v", resp.SessionIDs)
	}
}

func TestListShared_WithRecords(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "proj", 5)
	te.seedSession(t, "s2", "proj", 3)

	if err := te.db.RecordShare("s1", "pub:s1", "https://example.com"); err != nil {
		t.Fatalf("RecordShare: %v", err)
	}
	if err := te.db.RecordShare("s2", "pub:s2", "https://example.com"); err != nil {
		t.Fatalf("RecordShare: %v", err)
	}

	w := te.get(t, "/api/v1/shared")
	assertStatus(t, w, http.StatusOK)

	var resp struct {
		SessionIDs []string `json:"session_ids"`
	}
	resp = decode[struct {
		SessionIDs []string `json:"session_ids"`
	}](t, w)
	if len(resp.SessionIDs) != 2 {
		t.Errorf("expected 2 shared sessions, got %d", len(resp.SessionIDs))
	}
}

func TestShareSession_MissingConfig(t *testing.T) {
	te := setup(t) // No share config set
	te.seedSession(t, "s1", "proj", 5)
	te.seedMessages(t, "s1", 5)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/sessions/s1/share", nil)
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusBadRequest)
}

func TestShareSession_NotFound(t *testing.T) {
	// Even with config, sharing a nonexistent session should 404.
	// But since config is empty, it will 400 first.
	// This test verifies the error path.
	te := setup(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/sessions/nonexistent/share", nil)
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)

	// 400 because share config is missing (checked before session lookup)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestUnshareSession_NotShared(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "proj", 5)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/s1/share", nil)
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusNotFound)
}

func TestUnshareSession_Success(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "proj", 5)

	// Directly record a share in DB (bypassing the remote push).
	if err := te.db.RecordShare("s1", "pub:s1", "https://example.com"); err != nil {
		t.Fatalf("RecordShare: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/s1/share", nil)
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusNoContent)

	// Verify share record is gone.
	share, err := te.db.GetShare(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetShare: %v", err)
	}
	if share != nil {
		t.Errorf("expected nil share after unshare, got %+v", share)
	}
}
