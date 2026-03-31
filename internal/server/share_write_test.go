package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpsertShare(t *testing.T) {
	te := setup(t)

	body := `{
		"share_id": "pub:s1",
		"session": {
			"id": "original-id",
			"project": "test-project",
			"machine": "publisher-name",
			"agent": "claude",
			"message_count": 2,
			"user_message_count": 1
		},
		"messages": [
			{"session_id": "original-id", "ordinal": 0, "role": "user", "content": "hello", "timestamp": "2025-01-01T00:00:00Z", "content_length": 5},
			{"session_id": "original-id", "ordinal": 1, "role": "assistant", "content": "world", "timestamp": "2025-01-01T00:00:01Z", "content_length": 5}
		]
	}`

	req := httptest.NewRequest(http.MethodPut, "/api/v1/shares/pub:s1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusNoContent)

	// Verify session stored under share_id, not original id.
	session, err := te.db.GetSession(context.Background(), "pub:s1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session == nil {
		t.Fatal("expected session to exist under share_id")
	}
	if session.Project != "test-project" {
		t.Errorf("project = %q, want %q", session.Project, "test-project")
	}
	if session.Machine != "publisher-name" {
		t.Errorf("machine = %q, want %q", session.Machine, "publisher-name")
	}
	// Verify no local file metadata.
	if session.FilePath != nil {
		t.Errorf("file_path should be nil, got %v", session.FilePath)
	}

	// Verify messages stored.
	msgs, err := te.db.GetAllMessages(context.Background(), "pub:s1")
	if err != nil {
		t.Fatalf("GetAllMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].SessionID != "pub:s1" {
		t.Errorf("message session_id = %q, want %q", msgs[0].SessionID, "pub:s1")
	}
}

func TestUpsertShare_Idempotent(t *testing.T) {
	te := setup(t)

	body := `{
		"share_id": "pub:s1",
		"session": {
			"id": "s1",
			"project": "proj",
			"machine": "pub",
			"agent": "claude",
			"message_count": 1,
			"user_message_count": 1
		},
		"messages": [
			{"session_id": "s1", "ordinal": 0, "role": "user", "content": "v1", "content_length": 2}
		]
	}`

	// First upsert.
	req := httptest.NewRequest(http.MethodPut, "/api/v1/shares/pub:s1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusNoContent)

	// Second upsert with updated content.
	body2 := `{
		"share_id": "pub:s1",
		"session": {
			"id": "s1",
			"project": "proj-updated",
			"machine": "pub",
			"agent": "claude",
			"message_count": 1,
			"user_message_count": 1
		},
		"messages": [
			{"session_id": "s1", "ordinal": 0, "role": "user", "content": "v2", "content_length": 2}
		]
	}`
	req2 := httptest.NewRequest(http.MethodPut, "/api/v1/shares/pub:s1", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Origin", "http://127.0.0.1:0")
	w2 := httptest.NewRecorder()
	te.handler.ServeHTTP(w2, req2)
	assertStatus(t, w2, http.StatusNoContent)

	// Verify updated.
	session, err := te.db.GetSession(context.Background(), "pub:s1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Project != "proj-updated" {
		t.Errorf("project = %q, want %q", session.Project, "proj-updated")
	}

	msgs, err := te.db.GetAllMessages(context.Background(), "pub:s1")
	if err != nil {
		t.Fatalf("GetAllMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "v2" {
		t.Errorf("message content = %q, want %q", msgs[0].Content, "v2")
	}
}

func TestUpsertShare_MismatchedShareID(t *testing.T) {
	te := setup(t)

	body := `{"share_id": "wrong:id", "session": {"id": "s1", "project": "p", "machine": "m", "agent": "a"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/shares/pub:s1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusBadRequest)
}

func TestDeleteShare(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "pub:s1", "proj", 5)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/shares/pub:s1", nil)
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusNoContent)
}

func TestDeleteShare_NotFound(t *testing.T) {
	te := setup(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/shares/nonexistent", nil)
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusNotFound)
}

func TestPublicReadAccess(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "pub:s1", "proj", 5)
	te.seedMessages(t, "pub:s1", 5)

	// Public read: session list.
	w := te.get(t, "/api/v1/sessions")
	assertStatus(t, w, http.StatusOK)

	// Public read: session detail.
	w = te.get(t, "/api/v1/sessions/pub:s1")
	assertStatus(t, w, http.StatusOK)

	// Public read: messages.
	w = te.get(t, "/api/v1/sessions/pub:s1/messages")
	assertStatus(t, w, http.StatusOK)

	// Public read: search.
	w = te.get(t, "/api/v1/search?q=test")
	assertStatus(t, w, http.StatusOK)
}

func TestSharedSessionNoLocalPaths(t *testing.T) {
	te := setup(t)

	body := `{
		"share_id": "pub:s1",
		"session": {
			"id": "s1",
			"project": "proj",
			"machine": "publisher",
			"agent": "claude",
			"message_count": 1
		},
		"messages": [
			{"session_id": "s1", "ordinal": 0, "role": "user", "content": "test", "content_length": 4}
		]
	}`

	req := httptest.NewRequest(http.MethodPut, "/api/v1/shares/pub:s1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusNoContent)

	// Fetch the session via the public API and verify no file paths.
	w2 := te.get(t, "/api/v1/sessions/pub:s1")
	assertStatus(t, w2, http.StatusOK)

	var session map[string]any
	if err := json.NewDecoder(w2.Body).Decode(&session); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if session["file_path"] != nil {
		t.Errorf("file_path should be null, got %v", session["file_path"])
	}
	if session["file_size"] != nil {
		t.Errorf("file_size should be null, got %v", session["file_size"])
	}
}
