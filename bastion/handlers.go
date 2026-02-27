package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const sessionExpiry = 24 * time.Hour

// writeJSONError sends a JSON error response {"error": "message"} with the given status code.
func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	// best-effort encode; body may already be written on WriteHeader
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (s *Server) Setup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hasUser, err := HasAnyUser(ctx, s.pool)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"registration_open": !hasUser})
}

func (s *Server) Register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hasUser, err := HasAnyUser(ctx, s.pool)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if hasUser {
		writeJSONError(w, http.StatusForbidden, "registration already completed")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSONError(w, http.StatusBadRequest, "username and password required")
		return
	}
	_, err = CreateUser(ctx, s.pool, req.Username, req.Password)
	if err != nil {
		if isDuplicate(err) {
			writeJSONError(w, http.StatusConflict, "username already exists")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}

func isDuplicate(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSONError(w, http.StatusBadRequest, "username and password required")
		return
	}
	ctx := r.Context()
	user, err := GetUserByUsername(ctx, s.pool, req.Username)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !CheckPassword(user.PasswordHash, req.Password) {
		writeJSONError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := IssueToken(user.ID, user.Username, s.cfg.JWTSecret, sessionExpiry)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionExpiry.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	_ = json.NewEncoder(w).Encode(map[string]string{
		"token":   token,
		"user_id": user.ID,
		"username": user.Username,
	})
}

func (s *Server) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if c, _ := r.Cookie("session"); c != nil {
			token = c.Value
		}
		if token == "" {
			if prefix, suffix, ok := strings.Cut(r.Header.Get("Authorization"), " "); ok && strings.EqualFold(prefix, "Bearer") {
				token = strings.TrimSpace(suffix)
			}
		}
		if token == "" {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		claims, err := ValidateToken(token, s.cfg.JWTSecret)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyClaims, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

type contextKey string

const ctxKeyClaims contextKey = "claims"

func ClaimsFromContext(ctx context.Context) *SessionClaims {
	c, _ := ctx.Value(ctxKeyClaims).(*SessionClaims)
	return c
}
