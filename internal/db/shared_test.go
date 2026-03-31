//go:build cgo

package db

import (
	"context"
	"testing"
)

func TestRecordShare(t *testing.T) {
	d := testDB(t)
	insertSession(t, d, "s1", "project-a")

	err := d.RecordShare("s1", "pub:s1", "https://share.example.com")
	requireNoError(t, err, "RecordShare")

	share, err := d.GetShare(context.Background(), "s1")
	requireNoError(t, err, "GetShare")
	if share == nil {
		t.Fatal("expected share record, got nil")
	}
	if share.ShareID != "pub:s1" {
		t.Errorf("share_id = %q, want %q", share.ShareID, "pub:s1")
	}
	if share.ServerURL != "https://share.example.com" {
		t.Errorf("server_url = %q, want %q", share.ServerURL, "https://share.example.com")
	}
}

func TestRecordShare_Upsert(t *testing.T) {
	d := testDB(t)
	insertSession(t, d, "s1", "project-a")

	err := d.RecordShare("s1", "pub:s1", "https://old.example.com")
	requireNoError(t, err, "RecordShare initial")

	err = d.RecordShare("s1", "pub:s1", "https://new.example.com")
	requireNoError(t, err, "RecordShare upsert")

	share, err := d.GetShare(context.Background(), "s1")
	requireNoError(t, err, "GetShare")
	if share.ServerURL != "https://new.example.com" {
		t.Errorf("server_url after upsert = %q, want %q",
			share.ServerURL, "https://new.example.com")
	}
}

func TestRemoveShare(t *testing.T) {
	d := testDB(t)
	insertSession(t, d, "s1", "project-a")

	err := d.RecordShare("s1", "pub:s1", "https://share.example.com")
	requireNoError(t, err, "RecordShare")

	err = d.RemoveShare("s1")
	requireNoError(t, err, "RemoveShare")

	share, err := d.GetShare(context.Background(), "s1")
	requireNoError(t, err, "GetShare after remove")
	if share != nil {
		t.Errorf("expected nil share after remove, got %+v", share)
	}
}

func TestRemoveShare_NotShared(t *testing.T) {
	d := testDB(t)
	// Removing a non-existent share should not error.
	err := d.RemoveShare("nonexistent")
	requireNoError(t, err, "RemoveShare nonexistent")
}

func TestGetShare_NotFound(t *testing.T) {
	d := testDB(t)
	share, err := d.GetShare(context.Background(), "nonexistent")
	requireNoError(t, err, "GetShare nonexistent")
	if share != nil {
		t.Errorf("expected nil for nonexistent share, got %+v", share)
	}
}

func TestListSharedSessionIDs(t *testing.T) {
	d := testDB(t)
	insertSession(t, d, "s1", "project-a")
	insertSession(t, d, "s2", "project-b")

	ids, err := d.ListSharedSessionIDs(context.Background())
	requireNoError(t, err, "ListSharedSessionIDs empty")
	if len(ids) != 0 {
		t.Errorf("expected empty list, got %v", ids)
	}

	err = d.RecordShare("s1", "pub:s1", "https://share.example.com")
	requireNoError(t, err, "RecordShare s1")
	err = d.RecordShare("s2", "pub:s2", "https://share.example.com")
	requireNoError(t, err, "RecordShare s2")

	ids, err = d.ListSharedSessionIDs(context.Background())
	requireNoError(t, err, "ListSharedSessionIDs")
	if len(ids) != 2 {
		t.Fatalf("expected 2 shared sessions, got %d", len(ids))
	}
}

func TestSharedSession_CascadeDelete(t *testing.T) {
	d := testDB(t)
	insertSession(t, d, "s1", "project-a")

	err := d.RecordShare("s1", "pub:s1", "https://share.example.com")
	requireNoError(t, err, "RecordShare")

	// Permanently delete the session - share record should cascade.
	d.mu.Lock()
	_, err = d.getWriter().Exec("DELETE FROM sessions WHERE id = ?", "s1")
	d.mu.Unlock()
	requireNoError(t, err, "delete session")

	share, err := d.GetShare(context.Background(), "s1")
	requireNoError(t, err, "GetShare after cascade")
	if share != nil {
		t.Errorf("expected nil share after cascade delete, got %+v", share)
	}
}
