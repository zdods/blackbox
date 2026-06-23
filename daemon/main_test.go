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

// logSafe must strip the control characters that log injection relies on, so a
// remote- or user-controlled value can't forge log lines or inject terminal
// escapes. (It also keeps the explicit \n/\r replacement CodeQL recognizes as a
// sanitizer — see logSafe's doc comment.)
func TestLogSafe(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"newline", "a\nb", "a b"},
		{"carriage return", "a\rb", "a b"},
		{"crlf forged entry", "ok\r\nADMIN logged in", "ok  ADMIN logged in"},
		{"tab", "a\tb", "a b"},
		{"escape sequence", "a\x1b[31mred", "a [31mred"},
		{"del", "a\x7fb", "a b"},
		{"plain text untouched", "hello world-123_/path", "hello world-123_/path"},
		{"unicode untouched", "café ☃", "café ☃"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := logSafe(tt.in); got != tt.want {
				t.Errorf("logSafe(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
