package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigValid(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	os.WriteFile(cfg, []byte("bastion_url = ws://example.com/ws/daemon\nhosted_path = /home/user/files\n"), 0600)

	url, path, err := loadConfig(cfg)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if url != "ws://example.com/ws/daemon" {
		t.Errorf("bastion_url = %q, want ws://example.com/ws/daemon", url)
	}
	if path != "/home/user/files" {
		t.Errorf("hosted_path = %q, want /home/user/files", path)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	url, path, err := loadConfig("/nonexistent/path/config")
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if url != "" || path != "" {
		t.Errorf("missing file should return empty strings, got url=%q path=%q", url, path)
	}
}

func TestLoadConfigCommentsAndBlankLines(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	content := "# comment\n\nbastion_url = ws://host\n# another comment\nhosted_path = /data\n"
	os.WriteFile(cfg, []byte(content), 0600)

	url, path, err := loadConfig(cfg)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if url != "ws://host" {
		t.Errorf("bastion_url = %q, want ws://host", url)
	}
	if path != "/data" {
		t.Errorf("hosted_path = %q, want /data", path)
	}
}

func TestLoadConfigPartial(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	os.WriteFile(cfg, []byte("bastion_url = ws://host\n"), 0600)

	url, path, err := loadConfig(cfg)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if url != "ws://host" {
		t.Errorf("bastion_url = %q", url)
	}
	if path != "" {
		t.Errorf("hosted_path should be empty, got %q", path)
	}
}

func TestLoadConfigIgnoresUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	os.WriteFile(cfg, []byte("unknown_key = value\nbastion_url = ws://ok\n"), 0600)

	url, _, err := loadConfig(cfg)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if url != "ws://ok" {
		t.Errorf("bastion_url = %q", url)
	}
}

func TestSaveConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")

	err := saveConfig(cfg, "ws://example.com/ws", "/srv/files")
	if err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	// Verify content
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)
	if content != "bastion_url = ws://example.com/ws\nhosted_path = /srv/files\n" {
		t.Errorf("content = %q", content)
	}

	// Verify permissions
	info, _ := os.Stat(cfg)
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("permissions = %o, want 0600", perm)
	}
}

func TestSaveAndLoadConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")

	saveConfig(cfg, "ws://roundtrip", "/round/trip")
	url, path, err := loadConfig(cfg)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if url != "ws://roundtrip" {
		t.Errorf("url = %q", url)
	}
	if path != "/round/trip" {
		t.Errorf("path = %q", path)
	}
}
