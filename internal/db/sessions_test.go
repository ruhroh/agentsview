package db

import (
	"context"
	"testing"
)

func TestUpsertSession_DisplayNameInsertOnly(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	displayName := "My Chat Title"
	err := d.UpsertSession(Session{
		ID:           "claude-ai:dn-test",
		Project:      "claude.ai",
		Machine:      "local",
		Agent:        "claude-ai",
		DisplayName:  &displayName,
		MessageCount: 1,
	})
	requireNoError(t, err, "UpsertSession insert")

	// Verify display_name was set.
	s, err := d.GetSession(ctx, "claude-ai:dn-test")
	requireNoError(t, err, "GetSession after insert")
	if s == nil {
		t.Fatal("GetSession returned nil after insert")
	}
	if s.DisplayName == nil {
		t.Fatal("DisplayName is nil after insert, want non-nil")
	}
	if *s.DisplayName != "My Chat Title" {
		t.Errorf("DisplayName = %q, want %q", *s.DisplayName, "My Chat Title")
	}

	// Re-upsert with a different display_name.
	newName := "Updated Title"
	err = d.UpsertSession(Session{
		ID:           "claude-ai:dn-test",
		Project:      "claude.ai",
		Machine:      "local",
		Agent:        "claude-ai",
		DisplayName:  &newName,
		MessageCount: 2,
	})
	requireNoError(t, err, "UpsertSession update")

	// display_name should NOT be overwritten by re-upsert.
	s, err = d.GetSession(ctx, "claude-ai:dn-test")
	requireNoError(t, err, "GetSession after re-upsert")
	if s == nil {
		t.Fatal("GetSession returned nil after re-upsert")
	}
	if s.DisplayName == nil {
		t.Fatal("DisplayName is nil after re-upsert, want non-nil")
	}
	if *s.DisplayName != "My Chat Title" {
		t.Errorf(
			"DisplayName = %q after re-upsert, want %q (should be preserved)",
			*s.DisplayName, "My Chat Title",
		)
	}
	// But other fields should update.
	if s.MessageCount != 2 {
		t.Errorf("MessageCount = %d, want 2", s.MessageCount)
	}
}
