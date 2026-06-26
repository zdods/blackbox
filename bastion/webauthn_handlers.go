package main

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

const ceremonyCookieName = "webauthn_ceremony"

// passwordAuthEnabled reports whether password (+TOTP) sign-in/registration is
// offered: in "password" and "both" modes.
func (s *Server) passwordAuthEnabled() bool {
	return s.cfg.AuthMode == authModePassword || s.cfg.AuthMode == authModeBoth
}

// passkeyAuthEnabled reports whether passkey sign-in/registration is offered:
// in "passkey" and "both" modes, and only when a relying party is configured.
func (s *Server) passkeyAuthEnabled() bool {
	return (s.cfg.AuthMode == authModePasskey || s.cfg.AuthMode == authModeBoth) && s.webAuthn != nil
}

// passkeyConfigured gates handlers that only need a relying party (any mode):
// 503 when RP_ID is unset. Used by the authenticated enroll/manage endpoints so
// a password-mode user can add a passkey before switching modes.
func (s *Server) passkeyConfigured(w http.ResponseWriter) bool {
	if s.webAuthn == nil {
		writeJSONError(w, http.StatusServiceUnavailable, errMsgUnavailable)
		return false
	}
	return true
}

// passkeyReady gates the public passkey login/registration handlers: 503 when
// no relying party is configured, 403 when passkey sign-in isn't offered in the
// current mode (i.e. pure "password" mode).
func (s *Server) passkeyReady(w http.ResponseWriter) bool {
	if !s.passkeyConfigured(w) {
		return false
	}
	if !s.passkeyAuthEnabled() {
		writeJSONError(w, http.StatusForbidden, errMsgUnavailable)
		return false
	}
	return true
}

// setCeremonyCookie stores the opaque cache key correlating a begin/finish pair.
func (s *Server) setCeremonyCookie(w http.ResponseWriter, key string) {
	http.SetCookie(w, &http.Cookie{
		Name:     ceremonyCookieName,
		Value:    key,
		Path:     "/api",
		MaxAge:   int(webauthnCeremonyTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure(),
	})
}

func (s *Server) clearCeremonyCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     ceremonyCookieName,
		Value:    "",
		Path:     "/api",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure(),
	})
}

// takeCeremony reads and consumes the ceremony correlated by the request cookie:
// it returns the cached state and clears both the cache entry and the cookie so
// the ceremony is strictly single-use.
func (s *Server) takeCeremony(w http.ResponseWriter, r *http.Request) (pendingCeremony, bool) {
	c, err := r.Cookie(ceremonyCookieName)
	if err != nil || c.Value == "" {
		return pendingCeremony{}, false
	}
	p, ok := s.passkeyCache.Get(c.Value)
	s.passkeyCache.Delete(c.Value)
	s.clearCeremonyCookie(w)
	return p, ok
}

// startCeremony stashes session state under a fresh key and sets the cookie.
func (s *Server) startCeremony(w http.ResponseWriter, sd *webauthn.SessionData, username, name string) {
	key := uuid.NewString()
	s.passkeyCache.Set(key, sd, username, name)
	s.setCeremonyCookie(w, key)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// --- first-run passkey registration (passkey mode, no user yet) --------------

// PasskeyRegisterBegin starts first-run registration: it mints a user handle but
// creates no row (that happens atomically at finish).
func (s *Server) PasskeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimitAuth(w, r) || !s.passkeyReady(w) {
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
	var req struct {
		Username string `json:"username"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	wu := &webauthnUser{user: &User{ID: uuid.NewString(), Username: req.Username}}
	creation, sd, err := s.webAuthn.BeginRegistration(wu)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	s.startCeremony(w, sd, req.Username, req.Name)
	writeJSON(w, http.StatusOK, creation)
}

// PasskeyRegisterFinish completes first-run registration, creating the user and
// its credential in one transaction, then logs the user in.
func (s *Server) PasskeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimitAuth(w, r) || !s.passkeyReady(w) {
		return
	}
	ceremony, ok := s.takeCeremony(w, r)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	ctx := r.Context()
	// Guard the single-user invariant against a racing registration.
	hasUser, err := HasAnyUser(ctx, s.pool)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if hasUser {
		writeJSONError(w, http.StatusForbidden, errMsgUnavailable)
		return
	}
	userID, err := uuid.FromBytes(ceremony.session.UserID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	wu := &webauthnUser{user: &User{ID: userID.String(), Username: ceremony.username}}
	cred, err := s.webAuthn.FinishRegistration(wu, *ceremony.session, r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	if err := CreateUserWithCredential(ctx, s.pool, userID.String(), ceremony.username, cred, ceremony.name); err != nil {
		if isDuplicate(err) {
			writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	user, err := GetUserByID(ctx, s.pool, userID.String())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.issuePasskeySessionForUser(w, user)
}

// --- discoverable passkey login (passkey mode) -------------------------------

// PasskeyLoginBegin starts a usernameless (discoverable) login ceremony.
func (s *Server) PasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimitAuth(w, r) || !s.passkeyReady(w) {
		return
	}
	assertion, sd, err := s.webAuthn.BeginDiscoverableLogin(
		webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.startCeremony(w, sd, "", "")
	writeJSON(w, http.StatusOK, assertion)
}

// PasskeyLoginFinish verifies the assertion and issues a session.
func (s *Server) PasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimitAuth(w, r) || !s.passkeyReady(w) {
		return
	}
	ceremony, ok := s.takeCeremony(w, r)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	ctx := r.Context()
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		u, creds, err := GetUserByHandle(ctx, s.pool, userHandle)
		if err != nil {
			u, creds, err = GetCredentialUser(ctx, s.pool, rawID)
		}
		if err != nil {
			return nil, err
		}
		return &webauthnUser{user: u, creds: creds}, nil
	}
	user, cred, err := s.webAuthn.FinishPasskeyLogin(handler, *ceremony.session, r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, errMsgAuthFailed)
		return
	}
	wu, _ := user.(*webauthnUser)
	if wu == nil {
		writeJSONError(w, http.StatusUnauthorized, errMsgAuthFailed)
		return
	}
	if cred.Authenticator.CloneWarning {
		// Surface a possible authenticator clone, but do not hard-block: this is
		// the only login factor for a single-user instance.
		slog.Warn("webauthn clone warning on login", "user", wu.user.ID)
	}
	if err := UpdateCredentialOnLogin(ctx, s.pool, cred); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.issuePasskeySessionForUser(w, wu.user)
}

// --- authenticated enrollment + management (both modes) ----------------------

// PasskeyEnrollBegin starts adding a passkey to the already-authenticated user.
func (s *Server) PasskeyEnrollBegin(w http.ResponseWriter, r *http.Request) {
	if !s.passkeyConfigured(w) {
		return
	}
	claims := ClaimsFromContext(r.Context())
	user, creds, err := getUserWithCreds(r.Context(), s.pool, claims.UserID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // name is optional
	wu := &webauthnUser{user: user, creds: creds}
	creation, sd, err := s.webAuthn.BeginRegistration(wu, webauthn.WithExclusions(wu.credentialDescriptors()))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	s.startCeremony(w, sd, "", req.Name)
	writeJSON(w, http.StatusOK, creation)
}

// PasskeyEnrollFinish stores the freshly enrolled credential.
func (s *Server) PasskeyEnrollFinish(w http.ResponseWriter, r *http.Request) {
	if !s.passkeyConfigured(w) {
		return
	}
	claims := ClaimsFromContext(r.Context())
	ceremony, ok := s.takeCeremony(w, r)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	user, creds, err := getUserWithCreds(r.Context(), s.pool, claims.UserID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	wu := &webauthnUser{user: user, creds: creds}
	cred, err := s.webAuthn.FinishRegistration(wu, *ceremony.session, r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	if err := InsertCredential(r.Context(), s.pool, user.ID, cred, ceremony.name); err != nil {
		if isDuplicate(err) {
			writeJSONError(w, http.StatusConflict, "passkey already registered")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

// ListPasskeys returns the authenticated user's enrolled passkeys.
func (s *Server) ListPasskeys(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	metas, err := ListPasskeyMeta(r.Context(), s.pool, claims.UserID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, metas)
}

// DeletePasskey removes one of the user's passkeys, refusing to remove the last
// usable factor.
func (s *Server) DeletePasskey(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	id := r.PathValue("id")
	ctx := r.Context()
	user, err := GetUserByID(ctx, s.pool, claims.UserID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	count, err := CountCredentials(ctx, s.pool, user.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Refuse to delete the last passkey when doing so would leave no way to log
	// in (passkey mode, or a passkey-only account with no password).
	if count <= 1 && (s.cfg.AuthMode == authModePasskey || user.PasswordHash == "") {
		writeJSONError(w, http.StatusConflict, "cannot remove the last passkey")
		return
	}
	n, err := DeleteCredential(ctx, s.pool, user.ID, id)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, errMsgBadRequest)
		return
	}
	if n == 0 {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- session issuance --------------------------------------------------------

// issuePasskeySessionForUser issues the standard JWT session cookie for an
// authenticated user — the same session the password+TOTP path produces. The
// token lives only in the httpOnly cookie, never the response body.
func (s *Server) issuePasskeySessionForUser(w http.ResponseWriter, user *User) {
	token, err := IssueToken(user.ID, user.Username, user.TokenVersion, s.cfg.JWTSecret, sessionExpiry)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]string{"user_id": user.ID, "username": user.Username})
}
