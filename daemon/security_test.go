package main

import (
	"os"
	"path/filepath"
	"testing"

	"blackhaul/pkg"
)

// A symlink inside the hosted root that points outside it must not be a usable
// escape hatch — this is the daemon's core security boundary.
func TestSafePathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("top secret"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if got := safePath(root, "escape/secret.txt"); got != "" {
		t.Errorf("safePath allowed read through escaping symlink: %q", got)
	}
	if got := safePath(root, "escape"); got != "" {
		t.Errorf("safePath allowed the escaping symlink itself: %q", got)
	}
}

// Legitimate files under the root, and not-yet-existent write targets, must
// still resolve (the symlink check must not break normal operation).
func TestSafePathAllowsRealPathsUnderRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "f.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := safePath(root, "docs/f.txt"); got == "" {
		t.Error("safePath rejected a legitimate file under root")
	}
	if got := safePath(root, "newdir/new.txt"); got == "" {
		t.Error("safePath rejected a new (non-existent) write target under root")
	}
}

// upload_id names a temp directory under the root; an attacker-controlled value
// must never be able to escape it.
func TestHandleWriteChunkRejectsMaliciousUploadID(t *testing.T) {
	root := t.TempDir()
	req := &pkg.WriteChunkRequest{
		Type: pkg.TypeWriteChunk, RequestID: "r1",
		UploadID: "../../../../tmp/evil", Path: "f.txt",
		ChunkIndex: 0, TotalChunks: 1,
	}
	resp := handleWriteChunk(root, req, []byte("data"))
	if resp.Error == "" {
		t.Fatal("expected rejection of traversal upload_id, got success")
	}
}

func TestHandleWriteChunkRejectsBadChunkParams(t *testing.T) {
	root := t.TempDir()
	cases := []*pkg.WriteChunkRequest{
		{UploadID: "ok", Path: "f", ChunkIndex: 5, TotalChunks: 1},
		{UploadID: "ok", Path: "f", ChunkIndex: 0, TotalChunks: 0},
		{UploadID: "ok", Path: "f", ChunkIndex: -1, TotalChunks: 3},
		{UploadID: "ok", Path: "f", ChunkIndex: 0, TotalChunks: maxTotalChunks + 1},
	}
	for _, req := range cases {
		req.Type = pkg.TypeWriteChunk
		req.RequestID = "r"
		if resp := handleWriteChunk(root, req, []byte("x")); resp.Error == "" {
			t.Errorf("expected error for chunk params %+v", req)
		}
	}
}

// A valid single-chunk upload must assemble correctly (the chunked path still works).
func TestHandleWriteChunkValidRoundTrip(t *testing.T) {
	root := t.TempDir()
	req := &pkg.WriteChunkRequest{
		Type: pkg.TypeWriteChunk, RequestID: "r1",
		UploadID: "abc-123_X", Path: "out.txt",
		ChunkIndex: 0, TotalChunks: 1,
	}
	if resp := handleWriteChunk(root, req, []byte("hello")); resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	got, err := os.ReadFile(filepath.Join(root, "out.txt"))
	if err != nil || string(got) != "hello" {
		t.Fatalf("assembled file = %q, err = %v; want %q", got, err, "hello")
	}
}

func TestHandleReadChunkRejectsOversizeAndNegativeOffset(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if resp, _ := handleReadChunk(root, &pkg.ReadChunkRequest{Type: pkg.TypeReadChunk, RequestID: "r", Path: "f", Size: 1 << 30}); resp.Error == "" {
		t.Error("expected error for oversize read size")
	}
	if resp, _ := handleReadChunk(root, &pkg.ReadChunkRequest{Type: pkg.TypeReadChunk, RequestID: "r", Path: "f", Offset: -1, Size: 4}); resp.Error == "" {
		t.Error("expected error for negative offset")
	}
}

func TestHandleReadFileRejectsOversizeFile(t *testing.T) {
	root := t.TempDir()
	big := make([]byte, maxReadBytes+1)
	if err := os.WriteFile(filepath.Join(root, "big"), big, 0600); err != nil {
		t.Fatal(err)
	}
	if resp := handleReadFile(root, &pkg.ReadFileRequest{Type: pkg.TypeReadFile, RequestID: "r", Path: "big"}); resp.Error == "" {
		t.Error("expected error reading an oversized file via the small-file path")
	}
}
