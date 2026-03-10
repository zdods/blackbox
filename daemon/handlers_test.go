package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"blackbox/pkg"
)

func TestHandleListDir(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "file1.txt"), []byte("hello"), 0644)
	os.Mkdir(filepath.Join(root, "subdir"), 0755)

	req := &pkg.ListDirRequest{Type: pkg.TypeListDir, RequestID: "req-1", Path: "."}
	resp := handleListDir(root, req)

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.RequestID != "req-1" {
		t.Errorf("RequestID = %q, want %q", resp.RequestID, "req-1")
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(resp.Entries))
	}

	found := map[string]bool{}
	for _, e := range resp.Entries {
		found[e.Name] = true
		if e.Name == "file1.txt" {
			if e.IsDir {
				t.Error("file1.txt should not be a directory")
			}
			if e.Size != 5 {
				t.Errorf("size = %d, want 5", e.Size)
			}
		}
		if e.Name == "subdir" && !e.IsDir {
			t.Error("subdir should be a directory")
		}
	}
	if !found["file1.txt"] || !found["subdir"] {
		t.Error("expected both file1.txt and subdir in entries")
	}
}

func TestHandleListDirInvalidPath(t *testing.T) {
	root := t.TempDir()
	req := &pkg.ListDirRequest{Type: pkg.TypeListDir, RequestID: "req-2", Path: "../../etc"}
	resp := handleListDir(root, req)
	if resp.Error == "" {
		t.Error("expected error for path traversal")
	}
}

func TestHandleListDirNonexistent(t *testing.T) {
	root := t.TempDir()
	req := &pkg.ListDirRequest{Type: pkg.TypeListDir, RequestID: "req-3", Path: "nope"}
	resp := handleListDir(root, req)
	if resp.Error == "" {
		t.Error("expected error for nonexistent directory")
	}
}

func TestHandleReadFile(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "test.txt"), []byte("content"), 0644)

	req := &pkg.ReadFileRequest{Type: pkg.TypeReadFile, RequestID: "req-4", Path: "test.txt"}
	resp := handleReadFile(root, req)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	data, err := base64.StdEncoding.DecodeString(resp.Data)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if string(data) != "content" {
		t.Errorf("data = %q, want %q", string(data), "content")
	}
}

func TestHandleReadFileWithOffset(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "offset.txt"), []byte("abcdefghij"), 0644)

	req := &pkg.ReadFileRequest{Type: pkg.TypeReadFile, RequestID: "req-5", Path: "offset.txt", Offset: 3, Size: 4}
	resp := handleReadFile(root, req)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	data, _ := base64.StdEncoding.DecodeString(resp.Data)
	if string(data) != "defg" {
		t.Errorf("data = %q, want %q", string(data), "defg")
	}
}

func TestHandleReadFileTraversal(t *testing.T) {
	root := t.TempDir()
	req := &pkg.ReadFileRequest{Type: pkg.TypeReadFile, RequestID: "req-6", Path: "../../etc/passwd"}
	resp := handleReadFile(root, req)
	if resp.Error == "" {
		t.Error("expected error for path traversal")
	}
}

func TestHandleWriteFile(t *testing.T) {
	root := t.TempDir()
	data := base64.StdEncoding.EncodeToString([]byte("written"))
	req := &pkg.WriteFileRequest{Type: pkg.TypeWriteFile, RequestID: "req-7", Path: "new.txt", Data: data}
	resp := handleWriteFile(root, req)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	content, _ := os.ReadFile(filepath.Join(root, "new.txt"))
	if string(content) != "written" {
		t.Errorf("file content = %q, want %q", string(content), "written")
	}
}

func TestHandleWriteFileCreatesSubdirs(t *testing.T) {
	root := t.TempDir()
	data := base64.StdEncoding.EncodeToString([]byte("deep"))
	req := &pkg.WriteFileRequest{Type: pkg.TypeWriteFile, RequestID: "req-8", Path: "a/b/c.txt", Data: data}
	resp := handleWriteFile(root, req)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	content, _ := os.ReadFile(filepath.Join(root, "a", "b", "c.txt"))
	if string(content) != "deep" {
		t.Errorf("file content = %q, want %q", string(content), "deep")
	}
}

func TestHandleWriteFileTraversal(t *testing.T) {
	root := t.TempDir()
	data := base64.StdEncoding.EncodeToString([]byte("bad"))
	req := &pkg.WriteFileRequest{Type: pkg.TypeWriteFile, RequestID: "req-9", Path: "../../evil.txt", Data: data}
	resp := handleWriteFile(root, req)
	if resp.Error == "" {
		t.Error("expected error for path traversal")
	}
}

func TestHandleGetMeta(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "meta.txt"), []byte("12345"), 0644)

	req := &pkg.GetMetaRequest{Type: pkg.TypeGetMeta, RequestID: "req-10", Path: "meta.txt"}
	resp := handleGetMeta(root, req)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Size != 5 {
		t.Errorf("size = %d, want 5", resp.Size)
	}
	if resp.IsDir {
		t.Error("should not be a directory")
	}
	if resp.Mtime == "" {
		t.Error("mtime should not be empty")
	}
}

func TestHandleGetMetaDirectory(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "adir"), 0755)

	req := &pkg.GetMetaRequest{Type: pkg.TypeGetMeta, RequestID: "req-11", Path: "adir"}
	resp := handleGetMeta(root, req)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if !resp.IsDir {
		t.Error("should be a directory")
	}
}

func TestHandleDeleteFile(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "bye.txt"), []byte("gone"), 0644)

	req := &pkg.DeleteFileRequest{Type: pkg.TypeDeleteFile, RequestID: "req-12", Path: "bye.txt"}
	resp := handleDeleteFile(root, req)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if _, err := os.Stat(filepath.Join(root, "bye.txt")); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestHandleDeleteFileDirectory(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "dir", "sub"), 0755)
	os.WriteFile(filepath.Join(root, "dir", "sub", "file.txt"), []byte("x"), 0644)

	req := &pkg.DeleteFileRequest{Type: pkg.TypeDeleteFile, RequestID: "req-13", Path: "dir"}
	resp := handleDeleteFile(root, req)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if _, err := os.Stat(filepath.Join(root, "dir")); !os.IsNotExist(err) {
		t.Error("directory should be deleted recursively")
	}
}

func TestHandleDeleteFileTraversal(t *testing.T) {
	root := t.TempDir()
	req := &pkg.DeleteFileRequest{Type: pkg.TypeDeleteFile, RequestID: "req-14", Path: "../../etc"}
	resp := handleDeleteFile(root, req)
	if resp.Error == "" {
		t.Error("expected error for path traversal")
	}
}
