package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"blackhaul/pkg"

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

func (s *Server) ListDaemons(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	rows, err := s.pool.Query(r.Context(),
		`SELECT id::text, label, hosted_path, created_at FROM daemons WHERE owner_id = $1 ORDER BY label`, claims.UserID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()
	type daemonRow struct {
		ID         string `json:"id"`
		Label      string `json:"label"`
		HostedPath string `json:"hosted_path"`
		Connected  bool   `json:"connected"`
		DiskFree   *int64 `json:"disk_free,omitempty"`
		DiskTotal  *int64 `json:"disk_total,omitempty"`
	}
	var list []daemonRow
	for rows.Next() {
		var id, label, hostedPath string
		var createdAt interface{}
		if err := rows.Scan(&id, &label, &hostedPath, &createdAt); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		connected := s.hub.Connected(id)
		list = append(list, daemonRow{ID: id, Label: label, HostedPath: hostedPath, Connected: connected})
	}
	if err := rows.Err(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Fan out disk-stat requests concurrently: done serially, each connected
	// daemon adds a full round-trip (up to 2s on timeout) to the response.
	var wg sync.WaitGroup
	for i := range list {
		if !list[i].Connected {
			continue
		}
		wg.Add(1)
		go func(row *daemonRow) {
			defer wg.Done()
			if free, total := s.getDaemonDisk(r.Context(), row.ID); free >= 0 && total >= 0 {
				row.DiskFree = &free
				row.DiskTotal = &total
			}
		}(&list[i])
	}
	wg.Wait()
	if list == nil {
		list = []daemonRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(list)
}

// getDaemonDisk returns free and total bytes for the daemon's volume, or -1,-1 on failure.
func (s *Server) getDaemonDisk(ctx context.Context, daemonID string) (free, total int64) {
	ac := s.hub.Get(daemonID)
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

// CreateDaemon creates a new daemon; returns daemon id and token (show token only on create).
func (s *Server) CreateDaemon(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
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
		hostedPath = "." // path is set by the daemon when it runs
	}
	token, err := generateDaemonToken()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	var id string
	err = s.pool.QueryRow(r.Context(),
		`INSERT INTO daemons (label, token_hash, hosted_path, owner_id) VALUES ($1, $2, $3, $4) RETURNING id::text`,
		req.Label, HashDaemonToken(token), hostedPath, claims.UserID,
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

// UpdateDaemon updates a daemon (e.g. label). PATCH /api/daemons/:id
func (s *Server) UpdateDaemon(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	daemonID := r.PathValue("id")
	if daemonID == "" {
		writeJSONError(w, http.StatusBadRequest, "daemon id required")
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
	result, err := s.pool.Exec(r.Context(), `UPDATE daemons SET label = $1 WHERE id::text = $2 AND owner_id = $3`, *req.Label, daemonID, claims.UserID)
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

// DeleteDaemon removes a daemon. DELETE /api/daemons/:id
func (s *Server) DeleteDaemon(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	daemonID := r.PathValue("id")
	if daemonID == "" {
		writeJSONError(w, http.StatusBadRequest, "daemon id required")
		return
	}
	result, err := s.pool.Exec(r.Context(), `DELETE FROM daemons WHERE id::text = $1 AND owner_id = $2`, daemonID, claims.UserID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if result.RowsAffected() == 0 {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	s.hub.Disconnect(daemonID)
	w.WriteHeader(http.StatusNoContent)
}

// daemonOwnedBy reports whether the daemon belongs to the given user. Used to
// scope file/meta proxying to the caller's own daemons.
func (s *Server) daemonOwnedBy(ctx context.Context, daemonID, userID string) bool {
	var one int
	err := s.pool.QueryRow(ctx, `SELECT 1 FROM daemons WHERE id::text = $1 AND owner_id = $2`, daemonID, userID).Scan(&one)
	return err == nil
}

// generateDaemonToken returns a cryptographically secure token (32 bytes entropy, base64url).
func generateDaemonToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashDaemonToken hashes a daemon token for at-rest storage. SHA-256 is
// sufficient (no bcrypt needed): tokens carry 32 bytes of entropy, so
// brute-forcing the hash is infeasible.
func HashDaemonToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
