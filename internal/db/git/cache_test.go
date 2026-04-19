package git

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// cacheSchema matches the `git_cache` DDL in internal/db/schema.sql. We keep
// it inline so these tests don't depend on loading the full server schema.
const cacheSchema = `
CREATE TABLE IF NOT EXISTS git_cache (
    cache_key   TEXT PRIMARY KEY,
    kind        TEXT NOT NULL,
    payload     TEXT NOT NULL,
    computed_at TEXT NOT NULL
);
`

// newCacheDB returns a file-backed SQLite DB seeded with the git_cache
// table. A file (rather than `:memory:`) keeps the pool stable across
// multiple connection acquisitions by the *sql.DB.
func newCacheDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cache.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(cacheSchema); err != nil {
		t.Fatalf("init git_cache schema: %v", err)
	}
	return db
}

func TestCache_GetOrCompute_FirstCallInvokesCompute(t *testing.T) {
	db := newCacheDB(t)
	cache := NewCache(db)

	var calls int
	got, err := cache.GetOrCompute(
		context.Background(), "k1", "log", time.Hour,
		func() ([]byte, error) {
			calls++
			return []byte(`{"commits":3}`), nil
		},
	)
	if err != nil {
		t.Fatalf("GetOrCompute: %v", err)
	}
	if calls != 1 {
		t.Fatalf("compute called %d times, want 1", calls)
	}
	if string(got) != `{"commits":3}` {
		t.Fatalf("payload = %q, want %q", got, `{"commits":3}`)
	}

	// Verify the row landed in git_cache with the expected kind.
	var kind, payload string
	err = db.QueryRow(
		`SELECT kind, payload FROM git_cache WHERE cache_key = ?`, "k1",
	).Scan(&kind, &payload)
	if err != nil {
		t.Fatalf("row not persisted: %v", err)
	}
	if kind != "log" || payload != `{"commits":3}` {
		t.Fatalf("row = (%q, %q), want (log, {...})", kind, payload)
	}
}

func TestCache_GetOrCompute_WithinTTLReturnsCached(t *testing.T) {
	db := newCacheDB(t)
	cache := NewCache(db)

	var calls int
	compute := func() ([]byte, error) {
		calls++
		return []byte(`{"n":1}`), nil
	}

	if _, err := cache.GetOrCompute(
		context.Background(), "k1", "log", time.Hour, compute,
	); err != nil {
		t.Fatalf("first GetOrCompute: %v", err)
	}
	if calls != 1 {
		t.Fatalf("after first call, calls = %d, want 1", calls)
	}

	got, err := cache.GetOrCompute(
		context.Background(), "k1", "log", time.Hour, compute,
	)
	if err != nil {
		t.Fatalf("second GetOrCompute: %v", err)
	}
	if calls != 1 {
		t.Fatalf("compute called again within TTL: calls = %d, want 1", calls)
	}
	if string(got) != `{"n":1}` {
		t.Fatalf("cached payload = %q, want %q", got, `{"n":1}`)
	}
}

func TestCache_GetOrCompute_PastTTLRecomputes(t *testing.T) {
	db := newCacheDB(t)
	cache := NewCache(db)

	var calls int
	compute := func() ([]byte, error) {
		calls++
		return []byte(`{"call":` + strconv.Itoa(calls) + `}`), nil
	}

	// Seed the cache with a timestamp well in the past so the second call
	// sees an expired row.
	if _, err := cache.GetOrCompute(
		context.Background(), "k1", "log", time.Hour, compute,
	); err != nil {
		t.Fatalf("first GetOrCompute: %v", err)
	}
	oldTime := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(
		`UPDATE git_cache SET computed_at = ? WHERE cache_key = ?`,
		oldTime, "k1",
	); err != nil {
		t.Fatalf("backdating row: %v", err)
	}

	got, err := cache.GetOrCompute(
		context.Background(), "k1", "log", time.Hour, compute,
	)
	if err != nil {
		t.Fatalf("second GetOrCompute: %v", err)
	}
	if calls != 2 {
		t.Fatalf("compute invocations = %d, want 2 (past TTL)", calls)
	}
	if string(got) != `{"call":2}` {
		t.Fatalf("payload = %q, want recomputed %q", got, `{"call":2}`)
	}
}

func TestCache_GetOrCompute_ErrorDoesNotWriteRow(t *testing.T) {
	db := newCacheDB(t)
	cache := NewCache(db)

	boom := errors.New("compute blew up")
	_, err := cache.GetOrCompute(
		context.Background(), "k1", "log", time.Hour,
		func() ([]byte, error) { return nil, boom },
	)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrap of %v", err, boom)
	}

	var n int
	if err := db.QueryRow(
		`SELECT count(*) FROM git_cache WHERE cache_key = ?`, "k1",
	).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("row count after error = %d, want 0", n)
	}
}

func TestCacheKey_DeterministicAndSensitiveToEachField(t *testing.T) {
	base := CacheKey("log", "/r", "a@x", "2026-01-01", "2026-02-01")
	if base == "" {
		t.Fatal("CacheKey returned empty string")
	}
	if got := CacheKey("log", "/r", "a@x", "2026-01-01", "2026-02-01"); got != base {
		t.Fatalf("CacheKey non-deterministic: %q vs %q", got, base)
	}

	cases := []struct {
		name                     string
		kind, repo, author, s, u string
	}{
		{"kind", "pr", "/r", "a@x", "2026-01-01", "2026-02-01"},
		{"repo", "log", "/r2", "a@x", "2026-01-01", "2026-02-01"},
		{"author", "log", "/r", "b@x", "2026-01-01", "2026-02-01"},
		{"since", "log", "/r", "a@x", "2026-01-02", "2026-02-01"},
		{"until", "log", "/r", "a@x", "2026-01-01", "2026-02-02"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CacheKey(c.kind, c.repo, c.author, c.s, c.u)
			if got == base {
				t.Fatalf("CacheKey did not change when %s differed: %q", c.name, got)
			}
		})
	}
}
