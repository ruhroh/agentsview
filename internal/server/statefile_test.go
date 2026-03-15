package server

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndRemoveStateFile(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteStateFile(dir, "127.0.0.1", 8080, "1.0.0")
	if err != nil {
		t.Fatalf("WriteStateFile: %v", err)
	}

	want := filepath.Join(dir, "server.8080.json")
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading state file: %v", err)
	}

	var sf StateFile
	if err := json.Unmarshal(data, &sf); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if sf.Port != 8080 {
		t.Errorf("port = %d, want 8080", sf.Port)
	}
	if sf.Host != "127.0.0.1" {
		t.Errorf("host = %q, want 127.0.0.1", sf.Host)
	}
	if sf.Version != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", sf.Version)
	}
	if sf.PID != os.Getpid() {
		t.Errorf("pid = %d, want %d", sf.PID, os.Getpid())
	}
	if sf.StartedAt == "" {
		t.Error("started_at is empty")
	}

	RemoveStateFile(dir, 8080)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("state file not removed")
	}
}

func TestFindRunningServer_NoFiles(t *testing.T) {
	dir := t.TempDir()
	if sf := FindRunningServer(dir); sf != nil {
		t.Errorf("expected nil, got %+v", sf)
	}
}

func TestFindRunningServer_StaleFile(t *testing.T) {
	dir := t.TempDir()

	// Write a state file with a PID that doesn't exist.
	sf := StateFile{
		PID:       999999999,
		Port:      9999,
		Host:      "127.0.0.1",
		Version:   "1.0.0",
		StartedAt: "2025-01-01T00:00:00Z",
	}
	data, _ := json.Marshal(sf)
	path := filepath.Join(dir, "server.9999.json")
	os.WriteFile(path, data, 0o644)

	result := FindRunningServer(dir)
	if result != nil {
		t.Errorf("expected nil for stale PID, got %+v", result)
	}

	// Stale file should be cleaned up.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("stale state file not cleaned up")
	}
}

func TestFindRunningServer_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.8080.json")
	os.WriteFile(path, []byte("not json"), 0o644)

	result := FindRunningServer(dir)
	if result != nil {
		t.Errorf("expected nil for invalid JSON, got %+v", result)
	}
}

func TestFindRunningServer_IgnoresNonStateFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(
		filepath.Join(dir, "config.json"),
		[]byte("{}"), 0o644,
	)
	os.WriteFile(
		filepath.Join(dir, "server.txt"),
		[]byte("nope"), 0o644,
	)

	result := FindRunningServer(dir)
	if result != nil {
		t.Errorf("expected nil, got %+v", result)
	}
}

func TestFindRunningServer_LiveProcess(t *testing.T) {
	dir := t.TempDir()

	// Start a real TCP listener so the port probe succeeds.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	// Write a state file for our own PID and the listener port.
	sf := StateFile{
		PID:       os.Getpid(),
		Port:      port,
		Host:      "127.0.0.1",
		Version:   "1.0.0",
		StartedAt: "2025-01-01T00:00:00Z",
	}
	data, _ := json.Marshal(sf)
	path := filepath.Join(dir, fmt.Sprintf("server.%d.json", port))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	result := FindRunningServer(dir)
	if result == nil {
		t.Fatal("expected running server, got nil")
	}
	if result.Port != port {
		t.Errorf("port = %d, want %d", result.Port, port)
	}
	if result.PID != os.Getpid() {
		t.Errorf("pid = %d, want %d", result.PID, os.Getpid())
	}
}

func TestFindRunningServer_BindAll(t *testing.T) {
	dir := t.TempDir()

	// Listener on 0.0.0.0 — probe should normalize to 127.0.0.1.
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	sf := StateFile{
		PID:       os.Getpid(),
		Port:      port,
		Host:      "0.0.0.0",
		Version:   "1.0.0",
		StartedAt: "2025-01-01T00:00:00Z",
	}
	data, _ := json.Marshal(sf)
	path := filepath.Join(dir, fmt.Sprintf("server.%d.json", port))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	result := FindRunningServer(dir)
	if result == nil {
		t.Fatal("expected running server for 0.0.0.0 host, got nil")
	}
	if result.Port != port {
		t.Errorf("port = %d, want %d", result.Port, port)
	}
}

func TestProbeHostForDial(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"", "127.0.0.1"},
		{"0.0.0.0", "127.0.0.1"},
		{"::", "::1"},
		{"127.0.0.1", "127.0.0.1"},
		{"192.168.1.100", "192.168.1.100"},
	}
	for _, tt := range tests {
		got := probeHostForDial(tt.host)
		if got != tt.want {
			t.Errorf(
				"probeHostForDial(%q) = %q, want %q",
				tt.host, got, tt.want,
			)
		}
	}
}

func TestStateFileName(t *testing.T) {
	tests := []struct {
		port int
		want string
	}{
		{8080, "server.8080.json"},
		{3000, "server.3000.json"},
		{443, "server.443.json"},
	}
	for _, tt := range tests {
		got := stateFileName(tt.port)
		if got != tt.want {
			t.Errorf(
				"stateFileName(%d) = %q, want %q",
				tt.port, got, tt.want,
			)
		}
	}
}
