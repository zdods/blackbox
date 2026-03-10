package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestStaticHandlerEmptyDir(t *testing.T) {
	h := staticHandler("")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestStaticHandlerServesFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0644)

	h := staticHandler(dir)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/hello.txt", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != "hi" {
		t.Errorf("body = %q, want %q", rr.Body.String(), "hi")
	}
}

func TestStaticHandlerSPAFallback(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0644)

	h := staticHandler(dir)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/dashboard", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != "<html>app</html>" {
		t.Errorf("body = %q, want SPA index", rr.Body.String())
	}
}

func TestStaticHandlerBlocksPathTraversal(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("ok"), 0644)

	h := staticHandler(dir)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/../../../etc/passwd", nil)
	h.ServeHTTP(rr, req)
	// path.Clean will normalize this; the handler should not serve /etc/passwd
	// It will either 404 or serve the SPA fallback
	if rr.Code == http.StatusOK {
		body := rr.Body.String()
		if body != "ok" {
			t.Error("path traversal should not serve files outside dir")
		}
	}
}

func TestStaticHandlerRejectsPost(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("ok"), 0644)

	h := staticHandler(dir)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/index.html", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("POST should return 404, got %d", rr.Code)
	}
}

func TestStaticHandlerRootServesIndex(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("root"), 0644)

	h := staticHandler(dir)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}
