package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"blackbox/pkg"

	"github.com/gorilla/websocket"
)

func (s *Server) HandleDaemonWS(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: wsCheckOrigin(s.cfg.CORSOrigin)}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
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
	ac := s.hub.Register(daemonID, conn)
	defer s.hub.Unregister(daemonID)
	ac.readLoop(s.hub)
}
