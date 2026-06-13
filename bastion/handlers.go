package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// isSafeMethod reports whether the HTTP method is non-state-changing.
func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// sameOrigin reports whether the request's Origin matches its Host. A missing
// Origin is allowed (non-browser clients omit it; SameSite=Lax covers browsers).
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

const sessionExpiry = 24 * time.Hour

// Generic auth error messages to avoid user/enumeration leakage. Use same message and status for all failure cases.
const errMsgAuthFailed = "invalid credentials"
const errMsgBadRequest = "invalid request"
const errMsgUnavailable = "unavailable"
const errMsgRateLimited = "too many attempts, try again later"

// rateLimitAuth gates an auth endpoint by client IP. Returns false (and writes
// 429) when the caller is over the limit.
func (s *Server) rateLimitAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.authLimiter.Allow(clientIP(r, s.cfg.TrustProxy)) {
		return true
	}
	writeJSONError(w, http.StatusTooManyRequests, errMsgRateLimited)
	return false
}

// writeJSONError sends a JSON error response {"error": "message"} with the given status code.
func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	// best-effort encode; body may already be written on WriteHeader
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (s *Server) Setup(w http.ResponseWriter, r *http.Request) {
	// Note: registration_open reveals whether any user exists. Acceptable for single-user self-hosted UX.
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
	if !s.rateLimitAuth(w, r) {
		return
	}
	ctx := r.Context()
	hasUser, err := HasAnyUser(ctx, s.pool)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if hasUser {
		writeJSONError(w, http.StatusForbidden, errMsgUnavailable)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TotpCode string `json:"totp_code"`
		SetupID  string `json:"setup_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	if req.Username == "" || req.Password == "" || req.TotpCode == "" || req.SetupID == "" {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	secret, ok := s.totpCache.Get(req.SetupID)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	if !ValidateTOTP(req.TotpCode, secret) {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	encSecret, err := encryptTOTPSecret(s.totpKey, secret)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	_, err = CreateUserWithTOTP(ctx, s.pool, req.Username, req.Password, encSecret)
	if err != nil {
		if isDuplicate(err) {
			writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.totpCache.Delete(req.SetupID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}

func (s *Server) RegisterTOTPSetup(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimitAuth(w, r) {
		return
	}
	hasUser, err := HasAnyUser(r.Context(), s.pool)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if hasUser {
		writeJSONError(w, http.StatusForbidden, errMsgUnavailable)
		return
	}
	setupID, secret, provisioningURI, err := GenerateTOTPSetup("blackhaul", "user")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.totpCache.Set(setupID, secret)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"setup_id":         setupID,
		"secret":           secret,
		"provisioning_uri": provisioningURI,
	})
}

func isDuplicate(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimitAuth(w, r) {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	ctx := r.Context()
	// Per-account throttle on password attempts (the per-IP limiter alone is
	// bypassable with many source IPs / spoofed XFF).
	if s.loginFailLimiter.Blocked(req.Username) {
		writeJSONError(w, http.StatusTooManyRequests, errMsgRateLimited)
		return
	}
	user, err := GetUserByUsername(ctx, s.pool, req.Username)
	if err != nil {
		// Spend comparable bcrypt time on a missing user so existence can't be
		// inferred from response timing.
		CheckPassword(string(dummyPasswordHash), req.Password)
		s.loginFailLimiter.Record(req.Username)
		writeJSONError(w, http.StatusUnauthorized, errMsgAuthFailed)
		return
	}
	if !CheckPassword(user.PasswordHash, req.Password) {
		s.loginFailLimiter.Record(req.Username)
		writeJSONError(w, http.StatusUnauthorized, errMsgAuthFailed)
		return
	}
	s.loginFailLimiter.Reset(req.Username)
	if user.TotpSecret != "" {
		loginToken, err := IssueLoginToken(user.ID, user.Username, s.cfg.JWTSecret)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"requires_totp": true,
			"login_token":   loginToken,
		})
		return
	}
	token, err := IssueToken(user.ID, user.Username, user.TokenVersion, s.cfg.JWTSecret, sessionExpiry)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.setSessionCookie(w, token)
	w.Header().Set("Content-Type", "application/json")
	// Session token travels only in the httpOnly cookie, never the body.
	_ = json.NewEncoder(w).Encode(map[string]string{
		"user_id":  user.ID,
		"username": user.Username,
	})
}

func (s *Server) LoginTOTP(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimitAuth(w, r) {
		return
	}
	var req struct {
		LoginToken string `json:"login_token"`
		Code       string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	if req.LoginToken == "" || req.Code == "" {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	claims, err := ValidateToken(req.LoginToken, s.cfg.JWTSecret)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, errMsgAuthFailed)
		return
	}
	if claims.Purpose != "totp_challenge" {
		writeJSONError(w, http.StatusUnauthorized, errMsgAuthFailed)
		return
	}
	ctx := r.Context()
	user, err := GetUserByID(ctx, s.pool, claims.UserID)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, errMsgAuthFailed)
		return
	}
	// Lock out brute-forced 6-digit codes: a few failures per account, then 429.
	if s.totpFailLimiter.Blocked(user.ID) {
		writeJSONError(w, http.StatusTooManyRequests, errMsgRateLimited)
		return
	}
	secret, err := decryptTOTPSecret(s.totpKey, user.TotpSecret)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if secret == "" || !ValidateTOTP(req.Code, secret) {
		s.totpFailLimiter.Record(user.ID)
		writeJSONError(w, http.StatusUnauthorized, errMsgAuthFailed)
		return
	}
	s.totpFailLimiter.Reset(user.ID)
	token, err := IssueToken(user.ID, user.Username, user.TokenVersion, s.cfg.JWTSecret, sessionExpiry)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.setSessionCookie(w, token)
	w.Header().Set("Content-Type", "application/json")
	// Session token travels only in the httpOnly cookie, never the body.
	_ = json.NewEncoder(w).Encode(map[string]string{
		"user_id":  user.ID,
		"username": user.Username,
	})
}

// cookieSecure reports whether the session cookie should carry the Secure flag:
// either TLS terminates in-process, or COOKIE_SECURE is set (TLS at a proxy).
func (s *Server) cookieSecure() bool {
	return s.cfg.CookieSecure || (s.cfg.TLSCertFile != "" && s.cfg.TLSKeyFile != "")
}

// setSessionCookie attaches the session JWT as an httpOnly cookie.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionExpiry.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure(),
	})
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	// Bump token_version so every outstanding JWT for this user is revoked,
	// not just the cookie we clear below.
	if claims := ClaimsFromContext(r.Context()); claims != nil {
		if err := BumpTokenVersion(r.Context(), s.pool, claims.UserID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure(),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		fromCookie := false
		if c, _ := r.Cookie("session"); c != nil {
			token = c.Value
			fromCookie = true
		}
		if token == "" {
			fromCookie = false
			if prefix, suffix, ok := strings.Cut(r.Header.Get("Authorization"), " "); ok && strings.EqualFold(prefix, "Bearer") {
				token = strings.TrimSpace(suffix)
			}
		}
		if token == "" {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		// CSRF defense-in-depth: a cookie-authenticated state-changing request
		// must come from the same origin. (SameSite=Lax is the primary defense;
		// this blocks the residual cross-site cases. Bearer-token API clients are
		// not cookie-bound and so are exempt.)
		if fromCookie && !isSafeMethod(r.Method) && !sameOrigin(r) {
			writeJSONError(w, http.StatusForbidden, "cross-origin request blocked")
			return
		}
		claims, err := ValidateToken(token, s.cfg.JWTSecret)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		// Reject tokens that are not full session tokens (e.g. login_token with purpose totp_challenge).
		if claims.Purpose != "" {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		// Reject revoked tokens: version must match the user's current token_version.
		ver, err := GetTokenVersion(r.Context(), s.pool, claims.UserID)
		if err != nil || claims.Ver != ver {
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
