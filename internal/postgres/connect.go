package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// RedactDSN returns the host portion of the DSN for diagnostics.
func RedactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// CheckSSL rejects non-loopback PG connections that permit plaintext.
func CheckSSL(dsn string) error {
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parsing pg connection string: %w", err)
	}
	if isLoopback(cfg.Host) {
		return nil
	}
	if hasPlaintextPath(cfg) {
		return fmt.Errorf(
			"pg connection to %s permits plaintext; set sslmode=require (or verify-full) for non-local hosts, or set allow_insecure = true under [pg] to override",
			cfg.Host,
		)
	}
	return nil
}

// WarnInsecureSSL logs a warning for insecure non-loopback PG connections.
func WarnInsecureSSL(dsn string) {
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return
	}
	if isLoopback(cfg.Host) {
		return
	}
	if hasPlaintextPath(cfg) {
		log.Printf(
			"warning: pg connection to %s permits plaintext; consider sslmode=require or verify-full for non-local hosts",
			cfg.Host,
		)
	}
}

func hasPlaintextPath(cfg *pgconn.Config) bool {
	if cfg.TLSConfig == nil {
		return true
	}
	for _, fb := range cfg.Fallbacks {
		if fb.TLSConfig == nil {
			return true
		}
	}
	return false
}

func isLoopback(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return true
	}
	if len(host) > 0 && host[0] == '/' {
		return true
	}
	return false
}

var validIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func quoteIdentifier(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("schema name must not be empty")
	}
	if !validIdentifier.MatchString(name) {
		return "", fmt.Errorf("invalid schema name: %q", name)
	}
	return `"` + name + `"`, nil
}

// Open opens a PG pool, validates SSL, and sets search_path + UTC timezone.
func Open(dsn, schema string, allowInsecure bool) (*sql.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("postgres URL is required")
	}
	quoted, err := quoteIdentifier(schema)
	if err != nil {
		return nil, fmt.Errorf("invalid pg schema: %w", err)
	}

	if allowInsecure {
		WarnInsecureSSL(dsn)
	} else if err := CheckSSL(dsn); err != nil {
		return nil, err
	}

	connStr, err := appendConnParams(dsn, map[string]string{
		"search_path": quoted,
		"TimeZone":    "UTC",
	})
	if err != nil {
		return nil, fmt.Errorf("setting connection params: %w", err)
	}

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, fmt.Errorf("opening pg (host=%s): %w", RedactDSN(dsn), err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pg ping (host=%s): %w", RedactDSN(dsn), err)
	}
	return db, nil
}

func appendConnParams(dsn string, params map[string]string) (string, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", fmt.Errorf("parsing pg URI: %w", err)
		}
		q := u.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		return u.String(), nil
	}

	result := dsn
	for k, v := range params {
		if result != "" {
			result += " "
		}
		result += k + "=" + v
	}
	return result, nil
}
