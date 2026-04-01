package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/wesm/agentsview/internal/config"
)

func TestPromptForMissingShareConfig_SavesValues(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir}

	var out bytes.Buffer
	input := strings.NewReader("\nhttps://share.example.com/\nsecret-token\nworkstation-1\n")
	if err := promptForMissingShareConfig(&cfg, input, &out); err != nil {
		t.Fatal(err)
	}

	if cfg.Share.URL != "https://share.example.com" {
		t.Errorf("URL = %q, want %q", cfg.Share.URL, "https://share.example.com")
	}
	if cfg.Share.Token != "secret-token" {
		t.Errorf("Token = %q, want %q", cfg.Share.Token, "secret-token")
	}
	if cfg.Share.Publisher != "workstation-1" {
		t.Errorf("Publisher = %q, want %q", cfg.Share.Publisher, "workstation-1")
	}

	var file struct {
		Share config.ShareConfig `toml:"share"`
	}
	if _, err := toml.DecodeFile(filepath.Join(dir, "config.toml"), &file); err != nil {
		t.Fatalf("parsing config file: %v", err)
	}
	if file.Share.URL != "https://share.example.com" {
		t.Errorf("file share.url = %q, want %q", file.Share.URL, "https://share.example.com")
	}
	if file.Share.Token != "secret-token" {
		t.Errorf("file share.token = %q, want %q", file.Share.Token, "secret-token")
	}
	if file.Share.Publisher != "workstation-1" {
		t.Errorf("file share.publisher = %q, want %q", file.Share.Publisher, "workstation-1")
	}
}

func TestPromptForMissingShareConfig_Skip(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir}

	var out bytes.Buffer
	if err := promptForMissingShareConfig(&cfg, strings.NewReader("n\n"), &out); err != nil {
		t.Fatal(err)
	}

	if cfg.Share.URL != "" || cfg.Share.Token != "" || cfg.Share.Publisher != "" {
		t.Fatalf("share config unexpectedly updated: %+v", cfg.Share)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("config.toml should not exist, stat err = %v", err)
	}
}

func TestEnsureShareConfigOnStartup_PersistsEffectiveEnvValues(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		DataDir: dir,
		Share: config.ShareConfig{
			URL:       "https://share.example.com",
			Token:     "env-token",
			Publisher: "env-publisher",
		},
	}

	if err := cfg.SaveSettings(map[string]any{"github_token": "keep-me"}); err != nil {
		t.Fatal(err)
	}

	if err := ensureShareConfigOnStartupWithIO(&cfg, strings.NewReader(""), &bytes.Buffer{}, false); err != nil {
		t.Fatal(err)
	}

	var file struct {
		GithubToken string             `toml:"github_token"`
		Share       config.ShareConfig `toml:"share"`
	}
	if _, err := toml.DecodeFile(filepath.Join(dir, "config.toml"), &file); err != nil {
		t.Fatalf("parsing config file: %v", err)
	}
	if file.GithubToken != "keep-me" {
		t.Errorf("github_token = %q, want %q", file.GithubToken, "keep-me")
	}
	if file.Share.URL != "https://share.example.com" {
		t.Errorf("share.url = %q, want %q", file.Share.URL, "https://share.example.com")
	}
	if file.Share.Token != "env-token" {
		t.Errorf("share.token = %q, want %q", file.Share.Token, "env-token")
	}
	if file.Share.Publisher != "env-publisher" {
		t.Errorf("share.publisher = %q, want %q", file.Share.Publisher, "env-publisher")
	}
}
