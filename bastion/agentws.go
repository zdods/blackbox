package main

import (
	"encoding/json"
	"net/http"

	"blackbox/pkg"
)

func (s *Server) HandleAgentWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	_, data, err := conn.ReadMessage()
	if err != nil {
		return
	}
	var auth pkg.Auth
	if err := json.Unmarshal(data, &auth); err != nil || auth.Type != pkg.TypeAuth {
		conn.WriteJSON(pkg.AuthError{Type: pkg.TypeAuthError, Error: "invalid auth message"})
		return
	}
	var agentID string
	err = s.pool.QueryRow(r.Context(), `SELECT id::text FROM agents WHERE token = $1`, auth.Token).Scan(&agentID)
	if err != nil {
		conn.WriteJSON(pkg.AuthError{Type: pkg.TypeAuthError, Error: "invalid token"})
		return
	}
	ac := s.hub.Register(agentID, conn)
	defer s.hub.Unregister(agentID)
	if err := conn.WriteJSON(pkg.AuthOK{Type: pkg.TypeAuthOK, AgentID: agentID}); err != nil {
		return
	}
	ac.readLoop(s.hub)
}
