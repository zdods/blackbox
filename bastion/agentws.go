package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"blackbox/pkg"

	"github.com/gorilla/websocket"
)

func (s *Server) HandleAgentWS(w http.ResponseWriter, r *http.Request) {
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
			log.Printf("agent ws: write auth error: %v", err)
		}
		return
	}
	var agentID string
	err = s.pool.QueryRow(r.Context(), `SELECT id::text FROM agents WHERE token = $1`, auth.Token).Scan(&agentID)
	if err != nil {
		if err := conn.WriteJSON(pkg.AuthError{Type: pkg.TypeAuthError, Error: "invalid token"}); err != nil {
			log.Printf("agent ws: write auth error: %v", err)
		}
		return
	}
	ac := s.hub.Register(agentID, conn)
	defer s.hub.Unregister(agentID)
	if err := conn.WriteJSON(pkg.AuthOK{Type: pkg.TypeAuthOK, AgentID: agentID}); err != nil {
		log.Printf("agent ws: write auth ok: %v", err)
		return
	}
	ac.readLoop(s.hub)
}
