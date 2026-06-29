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

// Multi-chunk assembly: chunks may arrive out of order and must reassemble into
// the original byte stream at the destination path.
func TestHandleWriteChunkMultiChunkOutOfOrder(t *testing.T) {
	root := t.TempDir()
	id := "multi_1"
	mk := func(idx int, data string) pkg.WriteChunkResponse {
		return handleWriteChunk(root, &pkg.WriteChunkRequest{
			Type: pkg.TypeWriteChunk, RequestID: "r", UploadID: id,
			Path: "joined.bin", ChunkIndex: idx, TotalChunks: 3,
		}, []byte(data))
	}
	// Deliver out of order: 2, 0, 1.
	if r := mk(2, "CCC"); r.Error != "" {
		t.Fatalf("chunk 2: %s", r.Error)
	}
	if r := mk(0, "AAA"); r.Error != "" {
		t.Fatalf("chunk 0: %s", r.Error)
	}
	if r := mk(1, "BBB"); r.Error != "" {
		t.Fatalf("chunk 1: %s", r.Error)
	}
	got, err := os.ReadFile(filepath.Join(root, "joined.bin"))
	if err != nil {
		t.Fatalf("read assembled: %v", err)
	}
	if string(got) != "AAABBBCCC" {
		t.Errorf("assembled = %q, want %q", got, "AAABBBCCC")
	}
}

// Retargeting protection: destPath is bound when the upload is created. A later
// chunk reusing the same upload_id with a *different valid* path must not move
// the write — the file lands at the original path and the alternate is untouched.
func TestHandleWriteChunkCannotRetargetPath(t *testing.T) {
	root := t.TempDir()
	id := "retarget_1"
	first := handleWriteChunk(root, &pkg.WriteChunkRequest{
		Type: pkg.TypeWriteChunk, RequestID: "r", UploadID: id,
		Path: "original.txt", ChunkIndex: 0, TotalChunks: 2,
	}, []byte("AA"))
	if first.Error != "" {
		t.Fatalf("chunk 0: %s", first.Error)
	}
	second := handleWriteChunk(root, &pkg.WriteChunkRequest{
		Type: pkg.TypeWriteChunk, RequestID: "r", UploadID: id,
		Path: "attacker.txt", ChunkIndex: 1, TotalChunks: 2,
	}, []byte("BB"))
	if second.Error != "" {
		t.Fatalf("chunk 1: %s", second.Error)
	}
	if got, err := os.ReadFile(filepath.Join(root, "original.txt")); err != nil || string(got) != "AABB" {
		t.Errorf("original.txt = %q, err=%v; want %q", got, err, "AABB")
	}
	if _, err := os.Stat(filepath.Join(root, "attacker.txt")); !os.IsNotExist(err) {
		t.Error("a later chunk retargeted the upload to a different path")
	}
}

func TestHandleReadChunkHappyPath(t *testing.T) {
	root := t.TempDir()
	content := []byte("0123456789")
	if err := os.WriteFile(filepath.Join(root, "f"), content, 0600); err != nil {
		t.Fatal(err)
	}
	// Read 4 bytes at offset 2.
	resp, data := handleReadChunk(root, &pkg.ReadChunkRequest{Type: pkg.TypeReadChunk, RequestID: "r", Path: "f", Offset: 2, Size: 4})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.ChunkSize != 4 || string(data) != "2345" {
		t.Errorf("chunk = %q (size %d), want %q", data, resp.ChunkSize, "2345")
	}
	// Partial read at EOF: ask for more than remains, get only the tail.
	resp, data = handleReadChunk(root, &pkg.ReadChunkRequest{Type: pkg.TypeReadChunk, RequestID: "r", Path: "f", Offset: 8, Size: 100})
	if resp.Error != "" {
		t.Fatalf("unexpected EOF-read error: %s", resp.Error)
	}
	if string(data) != "89" {
		t.Errorf("EOF chunk = %q, want %q", data, "89")
	}
}

func TestHandleGetDisk(t *testing.T) {
	root := t.TempDir()
	resp := handleGetDisk(root, &pkg.GetDiskRequest{Type: pkg.TypeGetDisk, RequestID: "r"})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.TotalBytes <= 0 {
		t.Errorf("TotalBytes = %d, want > 0", resp.TotalBytes)
	}
	if resp.FreeBytes < 0 || resp.FreeBytes > resp.TotalBytes {
		t.Errorf("FreeBytes = %d out of range (total %d)", resp.FreeBytes, resp.TotalBytes)
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
