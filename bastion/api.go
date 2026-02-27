package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"blackbox/pkg"

	"github.com/google/uuid"
)

func (s *Server) Me(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"user_id":  claims.UserID,
		"username": claims.Username,
	})
}

func (s *Server) ListAgents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(),
		`SELECT id::text, label, hosted_path, created_at FROM agents ORDER BY label`)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()
	type agentRow struct {
		ID           string  `json:"id"`
		Label        string  `json:"label"`
		HostedPath   string  `json:"hosted_path"`
		Connected    bool    `json:"connected"`
		DiskFree     *int64  `json:"disk_free,omitempty"`
		DiskTotal    *int64  `json:"disk_total,omitempty"`
	}
	var list []agentRow
	for rows.Next() {
		var id, label, hostedPath string
		var createdAt interface{}
		if err := rows.Scan(&id, &label, &hostedPath, &createdAt); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		connected := s.hub.Connected(id)
		row := agentRow{ID: id, Label: label, HostedPath: hostedPath, Connected: connected}
		if connected {
			if free, total := s.getAgentDisk(r.Context(), id); free >= 0 && total >= 0 {
				row.DiskFree = &free
				row.DiskTotal = &total
			}
		}
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if list == nil {
		list = []agentRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(list)
}

// getAgentDisk returns free and total bytes for the agent's volume, or -1,-1 on failure.
func (s *Server) getAgentDisk(ctx context.Context, agentID string) (free, total int64) {
	ac := s.hub.Get(agentID)
	if ac == nil {
		return -1, -1
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	reqID := uuid.New().String()
	req := pkg.GetDiskRequest{Type: pkg.TypeGetDisk, RequestID: reqID}
	respData, err := ac.Request(ctx, reqID, req)
	if err != nil {
		return -1, -1
	}
	var resp pkg.GetDiskResponse
	if json.Unmarshal(respData, &resp) != nil || resp.Error != "" {
		return -1, -1
	}
	return resp.FreeBytes, resp.TotalBytes
}

// CreateAgent creates a new agent; returns agent id and token (show token only on create).
func (s *Server) CreateAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label      string `json:"label"`
		HostedPath string `json:"hosted_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	if req.Label == "" {
		writeJSONError(w, http.StatusBadRequest, "label required")
		return
	}
	hostedPath := req.HostedPath
	if hostedPath == "" {
		hostedPath = "." // path is set by the agent when it runs
	}
	token, err := generateAgentToken()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	var id string
	err = s.pool.QueryRow(r.Context(),
		`INSERT INTO agents (label, token, hosted_path) VALUES ($1, $2, $3) RETURNING id::text`,
		req.Label, token, hostedPath,
	).Scan(&id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":          id,
		"label":       req.Label,
		"hosted_path": hostedPath,
		"token":       token,
	})
}

// UpdateAgent updates an agent (e.g. label). PATCH /api/agents/:id
func (s *Server) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		writeJSONError(w, http.StatusBadRequest, "agent id required")
		return
	}
	var req struct {
		Label *string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	if req.Label == nil || *req.Label == "" {
		writeJSONError(w, http.StatusBadRequest, "label required")
		return
	}
	result, err := s.pool.Exec(r.Context(), `UPDATE agents SET label = $1 WHERE id::text = $2`, *req.Label, agentID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if result.RowsAffected() == 0 {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteAgent removes an agent. DELETE /api/agents/:id
func (s *Server) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		writeJSONError(w, http.StatusBadRequest, "agent id required")
		return
	}
	s.hub.Unregister(agentID)
	result, err := s.pool.Exec(r.Context(), `DELETE FROM agents WHERE id::text = $1`, agentID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if result.RowsAffected() == 0 {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// generateAgentToken returns a cryptographically secure token (32 bytes entropy, base64url).
func generateAgentToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
