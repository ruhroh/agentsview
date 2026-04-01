package postgres

import "time"

var sqliteFormats = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.000Z",
	"2006-01-02T15:04:05Z",
	"2006-01-02 15:04:05",
}

// ParseSQLiteTimestamp parses SQLite-style ISO timestamps used by the existing branch.
func ParseSQLiteTimestamp(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, f := range sqliteFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// FormatISO8601 formats a time in UTC for JSON responses.
func FormatISO8601(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
