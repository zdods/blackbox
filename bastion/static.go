package main

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// staticHandler serves files from dir and falls back to index.html for SPA routes.
func staticHandler(dir string) http.Handler {
	if dir == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "." {
			p = "index.html"
		}
		// Block path traversal
		if strings.Contains(p, "..") {
			http.NotFound(w, r)
			return
		}
		fullPath := filepath.Join(dir, p)
		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() {
			// SPA fallback
			indexPath := filepath.Join(dir, "index.html")
			if _, err := os.Stat(indexPath); err != nil {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, indexPath)
			return
		}
		http.ServeFile(w, r, fullPath)
	})
}
