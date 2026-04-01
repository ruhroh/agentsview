package postgres

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/server"
)

// Compile-time check: *Store satisfies the hosted viewer contract.
var _ server.Store = (*Store)(nil)

// Store wraps a PostgreSQL connection for the hosted viewer runtime.
type Store struct {
	pg           *sql.DB
	cursorMu     sync.RWMutex
	cursorSecret []byte
}

// NewStore opens a PostgreSQL connection using the shared Open helper.
func NewStore(pgURL, schema string, allowInsecure bool) (*Store, error) {
	pg, err := Open(pgURL, schema, allowInsecure)
	if err != nil {
		return nil, err
	}
	return &Store{pg: pg}, nil
}

// DB returns the underlying *sql.DB.
func (s *Store) DB() *sql.DB { return s.pg }

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.pg.Close() }

// SetCursorSecret sets the HMAC key used for cursor signing.
func (s *Store) SetCursorSecret(secret []byte) {
	s.cursorMu.Lock()
	defer s.cursorMu.Unlock()
	s.cursorSecret = append([]byte(nil), secret...)
}

// EncodeCursor returns a base64-encoded, HMAC-signed cursor.
func (s *Store) EncodeCursor(endedAt, id string, total ...int) string {
	t := 0
	if len(total) > 0 {
		t = total[0]
	}
	c := db.SessionCursor{EndedAt: endedAt, ID: id, Total: t}
	data, _ := json.Marshal(c)

	s.cursorMu.RLock()
	secret := append([]byte(nil), s.cursorSecret...)
	s.cursorMu.RUnlock()

	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	sig := mac.Sum(nil)

	return base64.RawURLEncoding.EncodeToString(data) + "." +
		base64.RawURLEncoding.EncodeToString(sig)
}

// DecodeCursor parses a base64-encoded cursor string.
func (s *Store) DecodeCursor(raw string) (db.SessionCursor, error) {
	parts := strings.Split(raw, ".")
	if len(parts) == 1 {
		data, err := base64.RawURLEncoding.DecodeString(parts[0])
		if err != nil {
			return db.SessionCursor{}, fmt.Errorf("%w: %v", db.ErrInvalidCursor, err)
		}
		var c db.SessionCursor
		if err := json.Unmarshal(data, &c); err != nil {
			return db.SessionCursor{}, fmt.Errorf("%w: %v", db.ErrInvalidCursor, err)
		}
		c.Total = 0
		return c, nil
	}
	if len(parts) != 2 {
		return db.SessionCursor{}, fmt.Errorf("%w: invalid format", db.ErrInvalidCursor)
	}

	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return db.SessionCursor{}, fmt.Errorf("%w: invalid payload: %v", db.ErrInvalidCursor, err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return db.SessionCursor{}, fmt.Errorf("%w: invalid signature encoding: %v", db.ErrInvalidCursor, err)
	}

	s.cursorMu.RLock()
	secret := append([]byte(nil), s.cursorSecret...)
	s.cursorMu.RUnlock()

	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return db.SessionCursor{}, fmt.Errorf("%w: signature mismatch", db.ErrInvalidCursor)
	}

	var c db.SessionCursor
	if err := json.Unmarshal(data, &c); err != nil {
		return db.SessionCursor{}, fmt.Errorf("%w: invalid json: %v", db.ErrInvalidCursor, err)
	}
	return c, nil
}

// HasFTS returns true because PostgreSQL native FTS is available.
func (s *Store) HasFTS() bool { return true }
