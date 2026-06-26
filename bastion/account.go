package main

import (
	"encoding/json"
	"net/http"
	"net/mail"
	"strings"
)

// Account-management constraints.
const (
	minPasswordLen = 8
	maxEmailLen    = 254 // RFC 5321 maximum address length
)

// accountResponse is the authenticated user's profile plus the capability
// flags the account screen uses to decide which credential controls to render.
type accountResponse struct {
	Username        string `json:"username"`
	Email           string `json:"email"`
	HasPassword     bool   `json:"has_password"`     // a password is set (change vs. set)
	PasswordEnabled bool   `json:"password_enabled"` // password sign-in offered in this mode
	PasskeyEnabled  bool   `json:"passkey_enabled"`  // a relying party is configured (manageable)
}

// Account returns the current user's profile and credential capabilities.
func (s *Server) Account(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	user, err := GetUserByID(r.Context(), s.pool, claims.UserID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, accountResponse{
		Username:        user.Username,
		Email:           user.Email,
		HasPassword:     user.PasswordHash != "",
		PasswordEnabled: s.passwordAuthEnabled(),
		PasskeyEnabled:  s.webAuthn != nil,
	})
}

// validEmail reports whether addr is a single, bare RFC 5322 address (no
// display name) within the length cap. Callers pass a trimmed, lowercased
// value.
func validEmail(addr string) bool {
	if addr == "" || len(addr) > maxEmailLen {
		return false
	}
	parsed, err := mail.ParseAddress(addr)
	if err != nil {
		return false
	}
	// Reject "Name <a@b>" forms: we store and hash only the bare address.
	return parsed.Name == "" && strings.EqualFold(parsed.Address, addr)
}

// UpdateAccount sets (or clears) the profile email. An empty string clears it.
func (s *Server) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	var req struct {
		Email *string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == nil {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	email := strings.ToLower(strings.TrimSpace(*req.Email))
	if email != "" && !validEmail(email) {
		writeJSONError(w, http.StatusBadRequest, "invalid email address")
		return
	}
	// Persist an empty address as NULL so the column stays cleanly "unset".
	var val any
	if email != "" {
		val = email
	}
	if _, err := s.pool.Exec(r.Context(),
		`UPDATE users SET email = $1 WHERE id = $2`, val, claims.UserID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"email": email})
}

// ChangePassword sets a new password. When the account already has one, the
// current password must be supplied and match; a passkey-only account (no
// password yet) may set its first password without it. Every other outstanding
// session is revoked and the current request is re-issued a fresh cookie.
func (s *Server) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if !s.passwordAuthEnabled() {
		writeJSONError(w, http.StatusForbidden, errMsgUnavailable)
		return
	}
	claims := ClaimsFromContext(r.Context())
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	if len(req.NewPassword) < minPasswordLen {
		writeJSONError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	ctx := r.Context()
	user, err := GetUserByID(ctx, s.pool, claims.UserID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user.PasswordHash != "" && !CheckPassword(user.PasswordHash, req.CurrentPassword) {
		writeJSONError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Bump token_version in the same statement to revoke sibling sessions.
	if _, err := s.pool.Exec(ctx,
		`UPDATE users SET password_hash = $1, token_version = token_version + 1 WHERE id = $2`,
		hash, user.ID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	newVer, err := GetTokenVersion(ctx, s.pool, user.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Re-issue so this session survives the version bump (other devices don't).
	token, err := IssueToken(user.ID, user.Username, newVer, s.cfg.JWTSecret, sessionExpiry)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
