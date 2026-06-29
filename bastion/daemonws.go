package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"blackhaul/pkg"

	"github.com/gorilla/websocket"
)

func (s *Server) HandleDaemonWS(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: wsCheckOrigin(s.cfg.CORSOrigin)}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	// Bound per-frame memory: a daemon (compromised, or a future hosted-relay
	// peer) must not be able to OOM the server with an oversized frame. Caps the
	// largest list/read response well above the 5 MB download-chunk size.
	conn.SetReadLimit(maxDaemonFrameBytes)
	// Limit time for first message (auth) to avoid hanging connections.
	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		return
	}
	conn.SetReadDeadline(time.Time{}) // no deadline for rest of session
	var auth pkg.Auth
	if err := json.Unmarshal(data, &auth); err != nil || auth.Type != pkg.TypeAuth {
		if err := conn.WriteJSON(pkg.AuthError{Type: pkg.TypeAuthError, Error: "invalid auth message"}); err != nil {
			slog.Warn("daemon ws: write auth error", "err", err)
		}
		return
	}
	var daemonID string
	err = s.pool.QueryRow(r.Context(), `SELECT id::text FROM daemons WHERE token_hash = $1`, HashDaemonToken(auth.Token)).Scan(&daemonID)
	if err != nil {
		if err := conn.WriteJSON(pkg.AuthError{Type: pkg.TypeAuthError, Error: "invalid token"}); err != nil {
			slog.Warn("daemon ws: write auth error", "err", err)
		}
		return
	}
	// Send auth_ok before registering: once the daemon is in the hub, proxied
	// requests may write to the conn concurrently, and this direct write would
	// race them (gorilla/websocket allows only one writer).
	if err := conn.WriteJSON(pkg.AuthOK{Type: pkg.TypeAuthOK, DaemonID: daemonID}); err != nil {
		slog.Warn("daemon ws: write auth ok", "err", err)
		return
	}
	// readLoop's teardown unregisters this exact connection (compare-and-delete),
	// so no separate Unregister defer here — that would race a reconnect.
	ac := s.hub.Register(daemonID, conn)
	ac.readLoop(s.hub)
}
