package main

import (
	"path/filepath"
	"testing"
)

func TestSafePathValid(t *testing.T) {
	root := "/srv/files"
	tests := []struct {
		rel  string
		want string
	}{
		{".", "/srv/files"},
		{"docs", "/srv/files/docs"},
		{"docs/readme.txt", "/srv/files/docs/readme.txt"},
		{"a/b/c", "/srv/files/a/b/c"},
	}
	for _, tt := range tests {
		got := safePath(root, tt.rel)
		if got != tt.want {
			t.Errorf("safePath(%q, %q) = %q, want %q", root, tt.rel, got, tt.want)
		}
	}
}

func TestSafePathRejectsTraversal(t *testing.T) {
	root := "/srv/files"
	bad := []string{
		"..",
		"../etc/passwd",
		"../../..",
		"docs/../../..",
		"docs/../../../etc/shadow",
	}
	for _, rel := range bad {
		got := safePath(root, rel)
		if got != "" {
			t.Errorf("safePath(%q, %q) = %q, want empty (rejected)", root, rel, got)
		}
	}
}

func TestSafePathNormalization(t *testing.T) {
	root := "/srv/files"
	// "docs/../docs/file.txt" should normalize to /srv/files/docs/file.txt
	got := safePath(root, "docs/../docs/file.txt")
	want := filepath.Join(root, "docs", "file.txt")
	if got != want {
		t.Errorf("safePath normalized = %q, want %q", got, want)
	}
}
