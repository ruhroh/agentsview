package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/wesm/agentsview/internal/config"
	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/server"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = ""
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			runServe(os.Args[2:])
			return
		case "version", "--version", "-v":
			fmt.Printf("agentsview %s (commit %s, built %s)\n",
				version, commit, buildDate)
			return
		case "help", "--help", "-h":
			printUsage()
			return
		}
	}

	runServe(os.Args[1:])
}

func printUsage() {
	fmt.Printf(`agentsview %s - hosted web viewer for AI agent sessions

Serves a read-only analytics dashboard and session browser backed by
SQLite. No local file syncing or agent parsing is performed.

Usage:
  agentsview [flags]          Start the server (default command)
  agentsview serve [flags]    Start the server (explicit)
  agentsview version          Show version information
  agentsview help             Show this help

Server flags:
  -host string        Host to bind to (default "127.0.0.1")
  -port int           Port to listen on (default 8080)
  -public-url str     Public URL to trust and open for hostname/proxy access
  -public-origin str  Trusted browser origin to allow for remote/proxied access
  -proxy string       Managed reverse proxy mode (currently: caddy)
  -caddy-bin string   Caddy binary to use when -proxy=caddy
  -proxy-bind-host    Local interface/IP for managed Caddy to bind
  -public-port int    External port for managed Caddy/public URL (default 8443)
  -tls-cert string    TLS certificate path for managed Caddy HTTPS mode
  -tls-key string     TLS key path for managed Caddy HTTPS mode
  -allowed-subnet str Client CIDR allowed to connect to the managed proxy
  -no-browser         Don't open browser on startup

Environment variables:
  AGENT_VIEWER_DATA_DIR   Data directory (database, config)

Data is stored in ~/.agentsview/ by default.
`, version)
}

func runServe(args []string) {
	start := time.Now()
	cfg := mustLoadConfig(args)
	setupLogFile(cfg.DataDir)

	if err := validateServeConfig(cfg); err != nil {
		fatal("invalid serve config: %v", err)
	}

	// Write the startup lock immediately after config setup,
	// before opening the DB, so external tooling never sees a
	// window with no lock and no state file during startup.
	server.WriteStartupLock(cfg.DataDir)
	defer server.RemoveStartupLock(cfg.DataDir)

	database := mustOpenDB(cfg)
	defer database.Close()

	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()

	// Auto-enable remote access and bind to 0.0.0.0 so the
	// hosted server is reachable from the network.
	cfg.RemoteAccess = true
	if !cfg.HostExplicit && cfg.Host == "127.0.0.1" {
		cfg.Host = "0.0.0.0"
	}

	// Ensure an auth token exists so the API is never exposed
	// on the network without authentication.
	if err := cfg.EnsureAuthToken(); err != nil {
		log.Fatalf("Failed to generate auth token: %v", err)
	}
	if cfg.AuthToken != "" {
		fmt.Printf("Remote access enabled. Auth token: %s\n", cfg.AuthToken)
	}

	rtOpts := serveRuntimeOptions{
		Mode:          "serve",
		RequestedPort: cfg.Port,
	}
	preparedCfg, prepErr := prepareServeRuntimeConfig(cfg, rtOpts)
	if prepErr != nil {
		fatal("%v", prepErr)
	}
	cfg = preparedCfg

	srv := server.New(cfg, database,
		server.WithVersion(server.VersionInfo{
			Version:   version,
			Commit:    commit,
			BuildDate: buildDate,
		}),
		server.WithDataDir(cfg.DataDir),
		server.WithBaseContext(ctx),
	)

	rt, err := startServerWithOptionalCaddy(ctx, cfg, srv, rtOpts)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		fatal("%v", err)
	}

	// Server is ready — write the definitive state file with the
	// final port and remove the startup lock. If the state file
	// write fails, keep the startup lock as a fallback "server
	// is active" marker.
	if _, sfErr := server.WriteStateFile(
		rt.Cfg.DataDir, rt.Cfg.Host, rt.Cfg.Port, version,
	); sfErr != nil {
		log.Printf(
			"warning: could not write state file: %v"+
				" (keeping startup lock as fallback)",
			sfErr,
		)
	} else {
		defer server.RemoveStateFile(rt.Cfg.DataDir, rt.Cfg.Port)
		server.RemoveStartupLock(rt.Cfg.DataDir)
	}

	if rt.PublicURL == rt.LocalURL {
		fmt.Printf(
			"agentsview %s listening at %s (started in %s)\n",
			version, rt.LocalURL,
			time.Since(start).Round(time.Millisecond),
		)
	} else {
		fmt.Printf(
			"agentsview %s backend at %s, public at %s (started in %s)\n",
			version, rt.LocalURL, rt.PublicURL,
			time.Since(start).Round(time.Millisecond),
		)
	}

	if err := waitForServerRuntime(ctx, srv, rt); err != nil {
		fatal("%v", err)
	}
}

func mustLoadConfig(args []string) config.Config {
	fs := flag.NewFlagSet("agentsview", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(),
			"Usage: agentsview [serve] [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	config.RegisterServeFlags(fs)
	if err := fs.Parse(args); err != nil {
		log.Fatalf("parsing flags: %v", err)
	}

	cfg, err := config.Load(fs)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("creating data dir: %v", err)
	}
	return cfg
}

// maxLogSize is the threshold at which the debug log file is
// truncated on startup to prevent unbounded growth.
const maxLogSize = 10 * 1024 * 1024 // 10 MB

func setupLogFile(dataDir string) {
	logPath := filepath.Join(dataDir, "debug.log")
	truncateLogFile(logPath, maxLogSize)
	f, err := os.OpenFile(
		logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644,
	)
	if err != nil {
		log.Printf("warning: cannot open log file: %v", err)
		return
	}
	log.SetOutput(f)
}

// truncateLogFile truncates the log file if it exceeds limit
// bytes. Symlinks are skipped to avoid truncating unrelated
// files. Errors are silently ignored since logging is
// best-effort.
func truncateLogFile(path string, limit int64) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	if info.Size() <= limit {
		return
	}
	_ = os.Truncate(path, 0)
}

func mustOpenDB(cfg config.Config) *db.DB {
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		fatal("opening database: %v", err)
	}

	if cfg.CursorSecret != "" {
		secret, err := base64.StdEncoding.DecodeString(cfg.CursorSecret)
		if err != nil {
			fatal("invalid cursor secret: %v", err)
		}
		database.SetCursorSecret(secret)
	}

	return database
}

// fatal prints a formatted error to stderr and exits.
// Use instead of log.Fatalf after setupLogFile redirects
// log output to the debug log file.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "fatal: "+format+"\n", args...)
	os.Exit(1)
}
