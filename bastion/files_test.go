package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"blackhaul/pkg"

	"github.com/gorilla/websocket"
)

func TestContentDisposition(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"plain", "dir/report.pdf", `attachment; filename="report.pdf"; filename*=UTF-8''report.pdf`},
		{"no dir", "report.pdf", `attachment; filename="report.pdf"; filename*=UTF-8''report.pdf`},
		{"empty becomes download", "", `attachment; filename="download"; filename*=UTF-8''download`},
		{"trailing slash empty name", "dir/", `attachment; filename="download"; filename*=UTF-8''download`},
		// Non-ASCII, quote and backslash are sanitized in the ASCII fallback but
		// preserved (percent-encoded) in filename*.
		{"unicode", "résumé.txt", `attachment; filename="r_sum_.txt"; filename*=UTF-8''r%C3%A9sum%C3%A9.txt`},
		{"quote and backslash", `a"b\c.txt`, `attachment; filename="a_b_c.txt"; filename*=UTF-8''a%22b%5Cc.txt`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentDisposition(tt.path); got != tt.want {
				t.Errorf("contentDisposition(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// emulatedDaemon registers a hub connection whose peer (the dialed client) answers
// get_meta and read_chunk requests, optionally pausing perChunk before each chunk.
type emulatedDaemon struct {
	ac      *DaemonConn
	cleanup func()
}

func startEmulatedDaemon(t *testing.T, hub *Hub, file []byte, perChunk time.Duration) *emulatedDaemon {
	return startEmulatedDaemonOpts(t, hub, file, perChunk, false)
}

// startEmulatedDaemonOpts adds oversizeChunk: when true the peer returns a chunk
// larger than the bastion requested, to drive the size-mismatch guard.
func startEmulatedDaemonOpts(t *testing.T, hub *Hub, file []byte, perChunk time.Duration, oversizeChunk bool) *emulatedDaemon {
	t.Helper()
	acCh := make(chan *DaemonConn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ac := hub.Register("emu", conn)
		acCh <- ac
		go ac.readLoop(hub)
		<-ac.done
	}))
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	peer, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	// The peer plays the daemon role.
	go func() {
		for {
			mt, data, err := peer.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.TextMessage {
				continue
			}
			var req struct {
				Type      string `json:"type"`
				RequestID string `json:"request_id"`
				Offset    int64  `json:"offset"`
				Size      int    `json:"size"`
			}
			if json.Unmarshal(data, &req) != nil {
				continue
			}
			switch req.Type {
			case pkg.TypeGetMeta:
				_ = peer.WriteJSON(pkg.GetMetaResponse{Type: req.Type, RequestID: req.RequestID, Size: int64(len(file))})
			case pkg.TypeReadChunk:
				if perChunk > 0 {
					time.Sleep(perChunk)
				}
				end := req.Offset + int64(req.Size)
				if end > int64(len(file)) {
					end = int64(len(file))
				}
				chunk := file[req.Offset:end]
				if oversizeChunk {
					// Return more bytes than the bastion asked for.
					chunk = append(append([]byte{}, chunk...), bytes.Repeat([]byte("!"), req.Size+8)...)
				}
				_ = peer.WriteJSON(pkg.ReadChunkResponse{Type: req.Type, RequestID: req.RequestID, ChunkSize: len(chunk)})
				_ = peer.WriteMessage(websocket.BinaryMessage, chunk)
			}
		}
	}()
	ac := <-acCh
	return &emulatedDaemon{ac: ac, cleanup: func() { peer.Close(); srv.Close() }}
}

// TestProxyReadFileNotBoundedByProxyTimeout is the regression test for the bug
// where a streaming download inherited the 30s proxyTimeout: a large file over a
// slow link was silently truncated. Here the cumulative per-chunk delay exceeds
// proxyTimeout, yet the download must still complete in full.
func TestProxyReadFileNotBoundedByProxyTimeout(t *testing.T) {
	// Shrink knobs so the test is fast but still exercises the multi-chunk path.
	defer swap(&proxyTimeout, 60*time.Millisecond)()
	defer swap(&downloadChunkSize, 1024)()

	// 5 chunks of 1 KB. Each chunk pauses 40ms → ~200ms total, well past the
	// 60ms proxyTimeout. Each individual chunk stays under chunkTimeout (60s).
	file := bytes.Repeat([]byte("blackhaul-payload-"), 300) // ~5.4 KB
	hub := NewHub()
	emu := startEmulatedDaemon(t, hub, file, 40*time.Millisecond)
	defer emu.cleanup()

	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.proxyReadFile(context.Background(), rec, emu.ac, "big.bin")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), file) {
		t.Fatalf("downloaded %d bytes, want %d (truncated?)", rec.Body.Len(), len(file))
	}
}

// TestProxyReadFileRejectsOversizeChunk verifies the size-mismatch guard: if the
// daemon returns a chunk larger than requested, the bastion must stop streaming
// rather than overrun the announced Content-Length.
func TestProxyReadFileRejectsOversizeChunk(t *testing.T) {
	defer swap(&downloadChunkSize, 1024)()

	file := bytes.Repeat([]byte("x"), 4096) // forces the multi-chunk path
	hub := NewHub()
	emu := startEmulatedDaemonOpts(t, hub, file, 0, true)
	defer emu.cleanup()

	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.proxyReadFile(context.Background(), rec, emu.ac, "big.bin")

	// The guard bails on the first oversized chunk; the body must be shorter than
	// the declared file size (truncated, not overrun).
	if rec.Body.Len() >= len(file) {
		t.Fatalf("body = %d bytes; oversize chunk should have aborted before %d", rec.Body.Len(), len(file))
	}
}

// swap sets *p to v and returns a restore func, for temporarily overriding a
// package-level knob inside a test.
func swap[T any](p *T, v T) func() {
	old := *p
	*p = v
	return func() { *p = old }
}
