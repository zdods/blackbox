package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		vals []string
		want string
	}{
		{[]string{"a", "b", "c"}, "a"},
		{[]string{"", "b", "c"}, "b"},
		{[]string{"", "", "c"}, "c"},
		{[]string{"", "", ""}, ""},
		{[]string{}, ""},
	}
	for _, tt := range tests {
		got := firstNonEmpty(tt.vals...)
		if got != tt.want {
			t.Errorf("firstNonEmpty(%v) = %q, want %q", tt.vals, got, tt.want)
		}
	}
}

func TestResolveDir(t *testing.T) {
	// Absolute path should return unchanged (after Clean)
	got, err := resolveDir("/tmp/test")
	if err != nil {
		t.Fatalf("resolveDir: %v", err)
	}
	if got != "/tmp/test" {
		t.Errorf("resolveDir(/tmp/test) = %q, want /tmp/test", got)
	}
}

func TestResolveDirTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	got, err := resolveDir("~/Documents")
	if err != nil {
		t.Fatalf("resolveDir: %v", err)
	}
	want := filepath.Join(home, "Documents")
	if got != want {
		t.Errorf("resolveDir(~/Documents) = %q, want %q", got, want)
	}
}

func TestResolveDirTildeAlone(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	got, err := resolveDir("~")
	if err != nil {
		t.Fatalf("resolveDir: %v", err)
	}
	if got != home {
		t.Errorf("resolveDir(~) = %q, want %q", got, home)
	}
}
