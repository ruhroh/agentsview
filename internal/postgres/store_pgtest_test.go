//go:build pgtest

package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wesm/agentsview/internal/db"
)

func testPGURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_PG_URL")
	if url == "" {
		t.Skip("TEST_PG_URL not set; skipping PG tests")
	}
	return url
}

func testStore(t *testing.T) *Store {
	t.Helper()
	schema := fmt.Sprintf("agentsview_pgtest_%d", time.Now().UnixNano())
	store, err := NewStore(testPGURL(t), schema, true)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	store.SetCursorSecret([]byte("pgtest-secret"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := EnsureSchema(ctx, store.DB(), schema); err != nil {
		_ = store.Close()
		t.Fatalf("EnsureSchema: %v", err)
	}

	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		quoted, _ := quoteIdentifier(schema)
		_, _ = store.DB().ExecContext(dropCtx, "DROP SCHEMA IF EXISTS "+quoted+" CASCADE")
		_ = store.Close()
	})
	return store
}

func putSession(t *testing.T, store *Store, session db.Session, msgs []db.Message) {
	t.Helper()
	if session.MessageCount == 0 {
		session.MessageCount = len(msgs)
	}
	if session.UserMessageCount == 0 {
		for _, msg := range msgs {
			if msg.Role == "user" && !msg.IsSystem {
				session.UserMessageCount++
			}
		}
	}
	if session.CreatedAt == "" {
		session.CreatedAt = "2025-01-15T10:00:00Z"
	}
	if err := store.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession(%s): %v", session.ID, err)
	}
	if err := store.ReplaceSessionMessages(session.ID, msgs); err != nil {
		t.Fatalf("ReplaceSessionMessages(%s): %v", session.ID, err)
	}
}

func TestEnsureSchemaCreatesSearchObjects(t *testing.T) {
	store := testStore(t)

	ctx := context.Background()

	var extName string
	if err := store.DB().QueryRowContext(ctx,
		`SELECT extname FROM pg_extension WHERE extname = 'pg_trgm'`,
	).Scan(&extName); err != nil {
		t.Fatalf("pg_trgm extension missing: %v", err)
	}
	if extName != "pg_trgm" {
		t.Fatalf("extname = %q, want pg_trgm", extName)
	}

	var generated string
	if err := store.DB().QueryRowContext(ctx, `
		SELECT is_generated
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'messages'
		  AND column_name = 'search_tsv'`,
	).Scan(&generated); err != nil {
		t.Fatalf("search_tsv column metadata: %v", err)
	}
	if generated != "ALWAYS" {
		t.Fatalf("messages.search_tsv is_generated = %q, want ALWAYS", generated)
	}

	wantIndexes := []string{
		"idx_messages_search_tsv",
		"idx_sessions_display_name_trgm",
		"idx_sessions_first_message_trgm",
		"idx_tool_calls_result_content_trgm",
	}
	for _, name := range wantIndexes {
		var got string
		if err := store.DB().QueryRowContext(ctx, `
			SELECT indexname
			FROM pg_indexes
			WHERE schemaname = current_schema()
			  AND indexname = $1`,
			name,
		).Scan(&got); err != nil {
			t.Fatalf("missing index %s: %v", name, err)
		}
	}
}

func TestListGetSessionsAndMessages(t *testing.T) {
	store := testStore(t)

	parentStarted := "2025-01-15T10:00:00Z"
	parentEnded := "2025-01-15T11:00:00Z"
	childStarted := "2025-01-15T10:30:00Z"

	putSession(t, store, db.Session{
		ID:               "parent",
		Project:          "alpha",
		Machine:          "m1",
		Agent:            "claude",
		FirstMessage:     ptr("root question"),
		DisplayName:      ptr("Root Session"),
		StartedAt:        &parentStarted,
		EndedAt:          &parentEnded,
		RelationshipType: "",
	}, []db.Message{
		{
			SessionID:     "parent",
			Ordinal:       0,
			Role:          "user",
			Content:       "root question",
			Timestamp:     parentStarted,
			ContentLength: len("root question"),
		},
		{
			SessionID:     "parent",
			Ordinal:       1,
			Role:          "assistant",
			Content:       "artifact complete",
			Timestamp:     parentEnded,
			ContentLength: len("artifact complete"),
			ToolCalls: []db.ToolCall{{
				ToolName:            "exec",
				Category:            "Run",
				ToolUseID:           "tool-1",
				ResultContent:       "artifact bundle created",
				ResultContentLength: len("artifact bundle created"),
				ResultEvents: []db.ToolResultEvent{{
					Source:        "tool",
					Status:        "completed",
					Content:       "artifact bundle created",
					ContentLength: len("artifact bundle created"),
					Timestamp:     parentEnded,
				}},
			}},
		},
	})

	putSession(t, store, db.Session{
		ID:               "child",
		Project:          "alpha",
		Machine:          "m1",
		Agent:            "claude",
		FirstMessage:     ptr("child question"),
		StartedAt:        &childStarted,
		ParentSessionID:  ptr("parent"),
		RelationshipType: "subagent",
	}, []db.Message{{
		SessionID:     "child",
		Ordinal:       0,
		Role:          "user",
		Content:       "child question",
		Timestamp:     childStarted,
		ContentLength: len("child question"),
	}})

	page, err := store.ListSessions(context.Background(), db.SessionFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(page.Sessions) != 1 || page.Sessions[0].ID != "parent" {
		t.Fatalf("default session list = %+v, want only parent", page.Sessions)
	}

	page, err = store.ListSessions(context.Background(), db.SessionFilter{
		IncludeChildren: true,
		Limit:           10,
	})
	if err != nil {
		t.Fatalf("ListSessions(include children): %v", err)
	}
	if len(page.Sessions) != 2 {
		t.Fatalf("included session count = %d, want 2", len(page.Sessions))
	}

	gotParent, err := store.GetSession(context.Background(), "parent")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if gotParent == nil || gotParent.DisplayName == nil || *gotParent.DisplayName != "Root Session" {
		t.Fatalf("GetSession(parent) = %+v, want display name", gotParent)
	}

	children, err := store.GetChildSessions(context.Background(), "parent")
	if err != nil {
		t.Fatalf("GetChildSessions: %v", err)
	}
	if len(children) != 1 || children[0].ID != "child" {
		t.Fatalf("children = %+v, want child", children)
	}

	msgs, err := store.GetAllMessages(context.Background(), "parent")
	if err != nil {
		t.Fatalf("GetAllMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("message count = %d, want 2", len(msgs))
	}
	if len(msgs[1].ToolCalls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(msgs[1].ToolCalls))
	}
	if len(msgs[1].ToolCalls[0].ResultEvents) != 1 {
		t.Fatalf("tool result event count = %d, want 1", len(msgs[1].ToolCalls[0].ResultEvents))
	}
}

func TestSearchAndSessionSearch(t *testing.T) {
	store := testStore(t)

	loginStarted := "2025-01-15T10:00:00Z"
	loginEnded := "2025-01-15T11:00:00Z"
	nameOnlyEnded := "2025-01-16T11:00:00Z"

	putSession(t, store, db.Session{
		ID:              "content-hit",
		Project:         "alpha",
		Machine:         "m1",
		Agent:           "claude",
		FirstMessage:    ptr("deploy failure"),
		DisplayName:     ptr("Deploy failure"),
		StartedAt:       &loginStarted,
		EndedAt:         &loginEnded,
		MessageCount:    2,
		UserMessageCount: 1,
	}, []db.Message{
		{
			SessionID:     "content-hit",
			Ordinal:       0,
			Role:          "user",
			Content:       "login bug after deploy",
			Timestamp:     loginStarted,
			ContentLength: len("login bug after deploy"),
		},
		{
			SessionID:     "content-hit",
			Ordinal:       1,
			Role:          "assistant",
			Content:       "tracking artifact upload",
			Timestamp:     loginEnded,
			ContentLength: len("tracking artifact upload"),
			ToolCalls: []db.ToolCall{{
				ToolName:            "build",
				Category:            "Run",
				ResultContent:       "artifact bundle created",
				ResultContentLength: len("artifact bundle created"),
			}},
		},
	})

	putSession(t, store, db.Session{
		ID:               "name-hit",
		Project:          "beta",
		Machine:          "m2",
		Agent:            "codex",
		DisplayName:      ptr("login handbook"),
		FirstMessage:     ptr("reference doc"),
		EndedAt:          &nameOnlyEnded,
		MessageCount:     1,
		UserMessageCount: 1,
	}, []db.Message{{
		SessionID:     "name-hit",
		Ordinal:       0,
		Role:          "assistant",
		Content:       "no matching body text here",
		Timestamp:     nameOnlyEnded,
		ContentLength: len("no matching body text here"),
	}})

	putSession(t, store, db.Session{
		ID:               "filtered-out",
		Project:          "gamma",
		Machine:          "m3",
		Agent:            "claude",
		DisplayName:      ptr("other session"),
		MessageCount:     1,
		UserMessageCount: 1,
	}, []db.Message{{
		SessionID:     "filtered-out",
		Ordinal:       0,
		Role:          "assistant",
		Content:       "irrelevant",
		ContentLength: len("irrelevant"),
	}})

	page, err := store.Search(context.Background(), db.SearchFilter{
		Query: `login`,
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(page.Results) != 2 {
		t.Fatalf("search results = %d, want 2", len(page.Results))
	}
	if page.Results[0].SessionID != "content-hit" {
		t.Fatalf("first result = %s, want content-hit", page.Results[0].SessionID)
	}
	if page.Results[1].SessionID != "name-hit" {
		t.Fatalf("second result = %s, want name-hit", page.Results[1].SessionID)
	}
	if !strings.Contains(strings.ToLower(page.Results[0].Snippet), "mark") {
		t.Fatalf("expected highlighted snippet, got %q", page.Results[0].Snippet)
	}

	filtered, err := store.Search(context.Background(), db.SearchFilter{
		Query:   "login",
		Project: "alpha",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("Search(project): %v", err)
	}
	if len(filtered.Results) != 1 || filtered.Results[0].SessionID != "content-hit" {
		t.Fatalf("filtered results = %+v, want only content-hit", filtered.Results)
	}

	ordinals, err := store.SearchSession(context.Background(), "content-hit", "artifact")
	if err != nil {
		t.Fatalf("SearchSession: %v", err)
	}
	if len(ordinals) != 1 || ordinals[0] != 1 {
		t.Fatalf("SearchSession ordinals = %v, want [1]", ordinals)
	}
}

func TestMetadataQueries(t *testing.T) {
	store := testStore(t)

	putSession(t, store, db.Session{
		ID:               "s1",
		Project:          "alpha",
		Machine:          "m1",
		Agent:            "claude",
		FirstMessage:     ptr("alpha root"),
		MessageCount:     2,
		UserMessageCount: 2,
	}, []db.Message{
		{SessionID: "s1", Ordinal: 0, Role: "user", Content: "alpha root", ContentLength: len("alpha root")},
		{SessionID: "s1", Ordinal: 1, Role: "assistant", Content: "done", ContentLength: len("done")},
	})
	putSession(t, store, db.Session{
		ID:               "s2",
		Project:          "beta",
		Machine:          "m2",
		Agent:            "codex",
		FirstMessage:     ptr("beta one shot"),
		MessageCount:     1,
		UserMessageCount: 1,
	}, []db.Message{
		{SessionID: "s2", Ordinal: 0, Role: "user", Content: "beta one shot", ContentLength: len("beta one shot")},
	})

	stats, err := store.GetStats(context.Background(), true)
	if err != nil {
		t.Fatalf("GetStats(true): %v", err)
	}
	if stats.SessionCount != 1 || stats.MessageCount != 2 || stats.ProjectCount != 1 || stats.MachineCount != 1 {
		t.Fatalf("stats(true) = %+v, want root/one-shot filtered counts", stats)
	}

	stats, err = store.GetStats(context.Background(), false)
	if err != nil {
		t.Fatalf("GetStats(false): %v", err)
	}
	if stats.SessionCount != 2 || stats.MessageCount != 3 {
		t.Fatalf("stats(false) = %+v, want both sessions counted", stats)
	}

	projects, err := store.GetProjects(context.Background(), true)
	if err != nil {
		t.Fatalf("GetProjects: %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "alpha" {
		t.Fatalf("projects = %+v, want alpha only", projects)
	}

	agents, err := store.GetAgents(context.Background(), true)
	if err != nil {
		t.Fatalf("GetAgents: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "claude" {
		t.Fatalf("agents = %+v, want claude only", agents)
	}

	machines, err := store.GetMachines(context.Background(), true)
	if err != nil {
		t.Fatalf("GetMachines: %v", err)
	}
	if len(machines) != 1 || machines[0] != "m1" {
		t.Fatalf("machines = %v, want [m1]", machines)
	}
}

func TestStarsSharesAndSharedSessionWrites(t *testing.T) {
	store := testStore(t)

	putSession(t, store, db.Session{
		ID:               "s1",
		Project:          "alpha",
		Machine:          "m1",
		Agent:            "claude",
		FirstMessage:     ptr("alpha"),
		MessageCount:     1,
		UserMessageCount: 1,
	}, []db.Message{{SessionID: "s1", Ordinal: 0, Role: "user", Content: "alpha", ContentLength: len("alpha")}})
	putSession(t, store, db.Session{
		ID:               "s2",
		Project:          "beta",
		Machine:          "m2",
		Agent:            "codex",
		FirstMessage:     ptr("beta"),
		MessageCount:     1,
		UserMessageCount: 1,
	}, []db.Message{{SessionID: "s2", Ordinal: 0, Role: "user", Content: "beta", ContentLength: len("beta")}})

	ok, err := store.StarSession("missing")
	if err != nil {
		t.Fatalf("StarSession(missing): %v", err)
	}
	if ok {
		t.Fatal("StarSession(missing) = true, want false")
	}

	ok, err = store.StarSession("s1")
	if err != nil || !ok {
		t.Fatalf("StarSession(s1) = (%v, %v), want true,nil", ok, err)
	}
	if err := store.BulkStarSessions([]string{"s1", "s2", "missing"}); err != nil {
		t.Fatalf("BulkStarSessions: %v", err)
	}
	starred, err := store.ListStarredSessionIDs(context.Background())
	if err != nil {
		t.Fatalf("ListStarredSessionIDs: %v", err)
	}
	if len(starred) != 2 {
		t.Fatalf("starred IDs = %v, want 2 IDs", starred)
	}
	if err := store.UnstarSession("s1"); err != nil {
		t.Fatalf("UnstarSession: %v", err)
	}

	if err := store.RecordShare("s2", "pub:s2", "https://viewer.example"); err != nil {
		t.Fatalf("RecordShare: %v", err)
	}
	share, err := store.GetShare(context.Background(), "s2")
	if err != nil {
		t.Fatalf("GetShare: %v", err)
	}
	if share == nil || share.ShareID != "pub:s2" {
		t.Fatalf("share = %+v, want pub:s2", share)
	}
	sharedIDs, err := store.ListSharedSessionIDs(context.Background())
	if err != nil {
		t.Fatalf("ListSharedSessionIDs: %v", err)
	}
	if len(sharedIDs) != 1 || sharedIDs[0] != "s2" {
		t.Fatalf("shared IDs = %v, want [s2]", sharedIDs)
	}
	if err := store.RemoveShare("s2"); err != nil {
		t.Fatalf("RemoveShare: %v", err)
	}

	shareStarted := "2025-01-20T10:00:00Z"
	putSession(t, store, db.Session{
		ID:               "pub:s3",
		Project:          "shared",
		Machine:          "publisher",
		Agent:            "claude",
		FirstMessage:     ptr("shared session"),
		StartedAt:        &shareStarted,
		MessageCount:     1,
		UserMessageCount: 1,
	}, []db.Message{{SessionID: "pub:s3", Ordinal: 0, Role: "user", Content: "v1", ContentLength: len("v1")}})

	if err := store.ReplaceSessionMessages("pub:s3", []db.Message{
		{SessionID: "pub:s3", Ordinal: 0, Role: "user", Content: "v2", ContentLength: len("v2")},
	}); err != nil {
		t.Fatalf("ReplaceSessionMessages(shared): %v", err)
	}
	msgs, err := store.GetAllMessages(context.Background(), "pub:s3")
	if err != nil {
		t.Fatalf("GetAllMessages(shared): %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != "v2" {
		t.Fatalf("shared messages = %+v, want single v2 message", msgs)
	}

	if err := store.SoftDeleteSession("pub:s3"); err != nil {
		t.Fatalf("SoftDeleteSession: %v", err)
	}
	got, err := store.GetSession(context.Background(), "pub:s3")
	if err != nil {
		t.Fatalf("GetSession(after soft delete): %v", err)
	}
	if got != nil {
		t.Fatalf("GetSession(after soft delete) = %+v, want nil", got)
	}

	if err := store.UpsertSession(db.Session{
		ID:               "pub:s3",
		Project:          "shared",
		Machine:          "publisher",
		Agent:            "claude",
		FirstMessage:     ptr("reshared"),
		StartedAt:        &shareStarted,
		MessageCount:     1,
		UserMessageCount: 1,
		CreatedAt:        shareStarted,
	}); err != nil {
		t.Fatalf("UpsertSession(reshare): %v", err)
	}
	got, err = store.GetSession(context.Background(), "pub:s3")
	if err != nil {
		t.Fatalf("GetSession(reshare): %v", err)
	}
	if got == nil {
		t.Fatal("reshared session is still hidden")
	}
}

func TestSearchExplainPlansUseIndexes(t *testing.T) {
	store := testStore(t)

	putSession(t, store, db.Session{
		ID:               "plan-session",
		Project:          "alpha",
		Machine:          "m1",
		Agent:            "claude",
		DisplayName:      ptr("login display"),
		FirstMessage:     ptr("login first"),
		MessageCount:     1,
		UserMessageCount: 1,
	}, []db.Message{{
		SessionID:     "plan-session",
		Ordinal:       0,
		Role:          "user",
		Content:       "login body text",
		ContentLength: len("login body text"),
		ToolCalls: []db.ToolCall{{
			ToolName:            "exec",
			Category:            "Run",
			ResultContent:       "artifact result",
			ResultContentLength: len("artifact result"),
		}},
	}})

	assertPlanUses(t, store, `
		SELECT id
		FROM messages
		WHERE search_tsv @@ websearch_to_tsquery('english', 'login')`,
		"idx_messages_search_tsv",
	)
	assertPlanUses(t, store, `
		SELECT id
		FROM sessions
		WHERE display_name ILIKE '%login%'`,
		"idx_sessions_display_name_trgm",
	)
	assertPlanUses(t, store, `
		SELECT id
		FROM tool_calls
		WHERE result_content ILIKE '%artifact%'`,
		"idx_tool_calls_result_content_trgm",
	)
}

func assertPlanUses(t *testing.T, store *Store, query, want string) {
	t.Helper()
	tx, err := store.DB().Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatalf("SET LOCAL enable_seqscan: %v", err)
	}

	rows, err := tx.Query(`EXPLAIN (COSTS OFF) ` + query)
	if err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	defer rows.Close()

	var planLines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan EXPLAIN row: %v", err)
		}
		planLines = append(planLines, line)
	}
	plan := strings.Join(planLines, "\n")
	if !strings.Contains(plan, want) {
		t.Fatalf("expected plan to use %s, got:\n%s", want, plan)
	}
}

func ptr[T any](v T) *T { return &v }
