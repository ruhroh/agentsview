package server

import (
	"context"

	"github.com/wesm/agentsview/internal/db"
)

// Store is the storage contract the hosted viewer server needs.
// It is intentionally narrower than the historical db.Store surface.
type Store interface {
	SetCursorSecret(secret []byte)

	ListSessions(ctx context.Context, f db.SessionFilter) (db.SessionPage, error)
	GetSession(ctx context.Context, id string) (*db.Session, error)
	GetChildSessions(ctx context.Context, parentID string) ([]db.Session, error)

	GetMessages(ctx context.Context, sessionID string, from, limit int, asc bool) ([]db.Message, error)
	GetAllMessages(ctx context.Context, sessionID string) ([]db.Message, error)

	HasFTS() bool
	Search(ctx context.Context, f db.SearchFilter) (db.SearchPage, error)
	SearchSession(ctx context.Context, sessionID, query string) ([]int, error)

	GetStats(ctx context.Context, excludeOneShot bool) (db.Stats, error)
	GetProjects(ctx context.Context, excludeOneShot bool) ([]db.ProjectInfo, error)
	GetAgents(ctx context.Context, excludeOneShot bool) ([]db.AgentInfo, error)
	GetMachines(ctx context.Context, excludeOneShot bool) ([]string, error)

	StarSession(sessionID string) (bool, error)
	UnstarSession(sessionID string) error
	ListStarredSessionIDs(ctx context.Context) ([]string, error)
	BulkStarSessions(sessionIDs []string) error

	RecordShare(sessionID, shareID, serverURL string) error
	RemoveShare(sessionID string) error
	GetShare(ctx context.Context, sessionID string) (*db.SharedSession, error)
	ListSharedSessionIDs(ctx context.Context) ([]string, error)

	UpsertSession(s db.Session) error
	ReplaceSessionMessages(sessionID string, msgs []db.Message) error
	SoftDeleteSession(id string) error
}
