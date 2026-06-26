//go:build integration

package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

// enablePasskeys flips a freshly built test server into passkey mode with a
// configured relying party. Handlers read s.cfg/s.webAuthn through the pointer
// at request time, so mutating after newTestServer takes effect immediately.
func enablePasskeys(t *testing.T, s *Server) {
	t.Helper()
	wa, err := newWebAuthn(Config{RPID: "localhost", RPDisplayName: "Blackhaul", RPOrigins: []string{"http://localhost"}})
	if err != nil {
		t.Fatalf("newWebAuthn: %v", err)
	}
	s.webAuthn = wa
	s.cfg.AuthMode = authModePasskey
}

func userIDByName(t *testing.T, s *Server, username string) string {
	t.Helper()
	var id string
	if err := s.pool.QueryRow(context.Background(),
		`SELECT id::text FROM users WHERE username = $1`, username).Scan(&id); err != nil {
		t.Fatalf("lookup user %s: %v", username, err)
	}
	return id
}

func fakeCredential(seed string) *webauthn.Credential {
	aaguid := make([]byte, 16)
	return &webauthn.Credential{
		ID:              []byte("credential-" + seed),
		PublicKey:       []byte("public-key-" + seed),
		AttestationType: "none",
		Transport:       []protocol.AuthenticatorTransport{protocol.Internal},
		Flags:           webauthn.CredentialFlags{BackupEligible: true},
		Authenticator:   webauthn.Authenticator{AAGUID: aaguid, SignCount: 1},
	}
}

func TestIntegrationSetupReportsAuthMode(t *testing.T) {
	srv, ts := newTestServer(t)
	srv.cfg.AuthMode = authModePassword
	c := client(t)

	resp, err := c.Get(ts.URL + "/api/setup")
	if err != nil {
		t.Fatal(err)
	}
	got := decodeJSON[map[string]any](t, resp)
	if got["auth_mode"] != "password" || got["passkey_enabled"] != false {
		t.Fatalf("password mode setup = %v", got)
	}

	enablePasskeys(t, srv)
	resp, err = c.Get(ts.URL + "/api/setup")
	if err != nil {
		t.Fatal(err)
	}
	got = decodeJSON[map[string]any](t, resp)
	if got["auth_mode"] != "passkey" || got["passkey_enabled"] != true {
		t.Fatalf("passkey mode setup = %v", got)
	}
}

func TestIntegrationPasskeyEndpointsDisabledWithoutRPID(t *testing.T) {
	_, ts := newTestServer(t) // webAuthn is nil
	c := client(t)
	for _, path := range []string{"/api/passkey/login/begin", "/api/passkey/register/begin"} {
		resp := postJSON(t, c, ts.URL+path, map[string]string{"username": "x"})
		wantStatus(t, resp, http.StatusServiceUnavailable)
		resp.Body.Close()
	}
}

func TestIntegrationPasskeyModeGatesPasswordEndpoints(t *testing.T) {
	srv, ts := newTestServer(t)
	enablePasskeys(t, srv)
	c := client(t)

	for _, path := range []string{"/api/login", "/api/register", "/api/register/totp-setup"} {
		resp := postJSON(t, c, ts.URL+path, map[string]string{"username": "zach", "password": "pw12345678"})
		wantStatus(t, resp, http.StatusForbidden)
		resp.Body.Close()
	}
}

func TestIntegrationPasswordModeGatesPasskeyLoginRegister(t *testing.T) {
	srv, ts := newTestServer(t)
	// RP configured but AUTH_MODE stays password: public passkey login/register
	// are gated off; enrollment (authenticated) stays available.
	wa, err := newWebAuthn(Config{RPID: "localhost", RPDisplayName: "Blackhaul", RPOrigins: []string{"http://localhost"}})
	if err != nil {
		t.Fatal(err)
	}
	srv.webAuthn = wa
	srv.cfg.AuthMode = authModePassword
	c := client(t)

	for _, path := range []string{"/api/passkey/login/begin", "/api/passkey/register/begin"} {
		resp := postJSON(t, c, ts.URL+path, map[string]string{"username": "zach"})
		wantStatus(t, resp, http.StatusForbidden)
		resp.Body.Close()
	}
}

func TestIntegrationBothModeOffersBoth(t *testing.T) {
	srv, ts := newTestServer(t)
	wa, err := newWebAuthn(Config{RPID: "localhost", RPDisplayName: "Blackhaul", RPOrigins: []string{"http://localhost"}})
	if err != nil {
		t.Fatal(err)
	}
	srv.webAuthn = wa
	srv.cfg.AuthMode = authModeBoth
	c := client(t)

	// Setup advertises both methods.
	resp, err := c.Get(ts.URL + "/api/setup")
	if err != nil {
		t.Fatal(err)
	}
	got := decodeJSON[map[string]any](t, resp)
	if got["password_enabled"] != true || got["passkey_enabled"] != true {
		t.Fatalf("both mode setup = %v", got)
	}

	// Passkey login is live (200, not 403).
	resp = postJSON(t, c, ts.URL+"/api/passkey/login/begin", map[string]string{})
	wantStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Password login is live (unknown user → 401, i.e. not gated off with 403).
	resp = postJSON(t, c, ts.URL+"/api/login", map[string]string{"username": "ghost", "password": "nope"})
	wantStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()

	// Password registration is live (first-run TOTP setup returns 200).
	resp = postJSON(t, c, ts.URL+"/api/register/totp-setup", map[string]string{})
	wantStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestIntegrationEnrollAvailableInPasswordMode(t *testing.T) {
	srv, ts := newTestServer(t)
	// Relying party configured but AUTH_MODE=password: passkey *enrollment* must
	// still work so the existing user can add a passkey before switching modes.
	wa, err := newWebAuthn(Config{RPID: "localhost", RPDisplayName: "Blackhaul", RPOrigins: []string{"http://localhost"}})
	if err != nil {
		t.Fatal(err)
	}
	srv.webAuthn = wa
	srv.cfg.AuthMode = authModePassword
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")
	login(t, c, ts.URL, "zach", "pw12345678", secret)

	resp := postJSON(t, c, ts.URL+"/api/passkeys/enroll/begin", map[string]string{"name": "Laptop"})
	wantStatus(t, resp, http.StatusOK)
	opts := decodeJSON[map[string]any](t, resp)
	if pk, ok := opts["publicKey"].(map[string]any); !ok || pk["challenge"] == nil {
		t.Fatalf("enroll options missing publicKey.challenge: %v", opts)
	}
	// But public passkey *login* is still gated off in password mode.
	resp = postJSON(t, c, ts.URL+"/api/passkey/login/begin", map[string]string{})
	wantStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()
}

func TestIntegrationPasskeyLoginBeginReturnsOptions(t *testing.T) {
	srv, ts := newTestServer(t)
	enablePasskeys(t, srv)
	c := client(t)

	resp := postJSON(t, c, ts.URL+"/api/passkey/login/begin", map[string]string{})
	wantStatus(t, resp, http.StatusOK)
	opts := decodeJSON[map[string]any](t, resp)
	pk, ok := opts["publicKey"].(map[string]any)
	if !ok || pk["challenge"] == nil {
		t.Fatalf("login options missing publicKey.challenge: %v", opts)
	}
	// The ceremony correlation cookie must be set so finish can find the challenge.
	var found bool
	for _, ck := range resp.Cookies() {
		if ck.Name == ceremonyCookieName && ck.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("no webauthn_ceremony cookie on login begin")
	}
}

func TestIntegrationPasskeyRegisterBeginFirstRun(t *testing.T) {
	srv, ts := newTestServer(t)
	enablePasskeys(t, srv)
	c := client(t)

	// First run (no user): register begin returns creation options with the user
	// handle baked in, and creates no user row yet.
	resp := postJSON(t, c, ts.URL+"/api/passkey/register/begin", map[string]string{"username": "zach", "name": "Laptop"})
	wantStatus(t, resp, http.StatusOK)
	opts := decodeJSON[map[string]any](t, resp)
	pk, ok := opts["publicKey"].(map[string]any)
	if !ok || pk["challenge"] == nil {
		t.Fatalf("register options missing publicKey.challenge: %v", opts)
	}
	if has, _ := HasAnyUser(context.Background(), srv.pool); has {
		t.Fatal("register begin must not create a user row")
	}
}

// --- credential DB helpers --------------------------------------------------

func TestIntegrationCredentialDBHelpers(t *testing.T) {
	srv, ts := newTestServer(t)
	c := client(t)
	registerUser(t, c, ts.URL, "zach", "pw12345678")
	uid := userIDByName(t, srv, "zach")
	ctx := context.Background()

	cred := fakeCredential("a")
	if err := InsertCredential(ctx, srv.pool, uid, cred, "Laptop"); err != nil {
		t.Fatalf("InsertCredential: %v", err)
	}

	// Round trips intact.
	creds, err := ListCredentialsByUser(ctx, srv.pool, uid)
	if err != nil || len(creds) != 1 {
		t.Fatalf("ListCredentialsByUser = %v, %v", creds, err)
	}
	if string(creds[0].ID) != string(cred.ID) || string(creds[0].PublicKey) != string(cred.PublicKey) {
		t.Fatal("credential id/public key not preserved")
	}
	if len(creds[0].Transport) != 1 || creds[0].Transport[0] != protocol.Internal {
		t.Fatalf("transports not preserved: %v", creds[0].Transport)
	}

	if n, _ := CountCredentials(ctx, srv.pool, uid); n != 1 {
		t.Fatalf("CountCredentials = %d, want 1", n)
	}

	// Lookup by handle (user UUID bytes) and by raw credential id.
	handle, _ := uuid.MustParse(uid).MarshalBinary()
	u, cs, err := GetUserByHandle(ctx, srv.pool, handle)
	if err != nil || u.Username != "zach" || len(cs) != 1 {
		t.Fatalf("GetUserByHandle = %v, %v, %v", u, cs, err)
	}
	u2, _, err := GetCredentialUser(ctx, srv.pool, cred.ID)
	if err != nil || u2.ID != uid {
		t.Fatalf("GetCredentialUser = %v, %v", u2, err)
	}

	// Sign-count advances on login update.
	cred.Authenticator.SignCount = 42
	if err := UpdateCredentialOnLogin(ctx, srv.pool, cred); err != nil {
		t.Fatalf("UpdateCredentialOnLogin: %v", err)
	}
	creds, _ = ListCredentialsByUser(ctx, srv.pool, uid)
	if creds[0].Authenticator.SignCount != 42 {
		t.Fatalf("sign count = %d, want 42", creds[0].Authenticator.SignCount)
	}

	// Delete is owner-scoped: a foreign user id removes nothing.
	metas, _ := ListPasskeyMeta(ctx, srv.pool, uid)
	rowID := metas[0].ID
	if n, _ := DeleteCredential(ctx, srv.pool, uuid.NewString(), rowID); n != 0 {
		t.Fatal("DeleteCredential with wrong user must affect 0 rows")
	}
	if n, _ := DeleteCredential(ctx, srv.pool, uid, rowID); n != 1 {
		t.Fatal("DeleteCredential with right user must affect 1 row")
	}
}

// --- last-passkey guard + IDOR via the HTTP handlers ------------------------

func TestIntegrationDeletePasskeyLastFactorGuard(t *testing.T) {
	srv, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")
	login(t, c, ts.URL, "zach", "pw12345678", secret)
	uid := userIDByName(t, srv, "zach")
	ctx := context.Background()

	if err := InsertCredential(ctx, srv.pool, uid, fakeCredential("a"), "Laptop"); err != nil {
		t.Fatal(err)
	}
	metas, _ := ListPasskeyMeta(ctx, srv.pool, uid)
	rowID := metas[0].ID

	// Password mode + a real password: removing the last passkey is allowed.
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/passkeys/"+rowID, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// Re-add and switch to passkey mode: now the last passkey is protected.
	if err := InsertCredential(ctx, srv.pool, uid, fakeCredential("b"), "Laptop"); err != nil {
		t.Fatal(err)
	}
	metas, _ = ListPasskeyMeta(ctx, srv.pool, uid)
	rowID = metas[0].ID
	srv.cfg.AuthMode = authModePasskey
	req, _ = http.NewRequest("DELETE", ts.URL+"/api/passkeys/"+rowID, nil)
	resp, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusConflict)
	resp.Body.Close()
}

func TestIntegrationDeletePasskeyIDOR(t *testing.T) {
	srv, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")
	login(t, c, ts.URL, "zach", "pw12345678", secret)
	ctx := context.Background()

	// A second user with a passkey of their own.
	var otherID string
	if err := srv.pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash) VALUES ('intruder', 'x') RETURNING id::text`).Scan(&otherID); err != nil {
		t.Fatal(err)
	}
	if err := InsertCredential(ctx, srv.pool, otherID, fakeCredential("other"), "Theirs"); err != nil {
		t.Fatal(err)
	}
	metas, _ := ListPasskeyMeta(ctx, srv.pool, otherID)
	otherRow := metas[0].ID

	// zach must not be able to delete intruder's passkey (owner-scoped → 404).
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/passkeys/"+otherRow, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	// And it must still exist.
	if n, _ := CountCredentials(ctx, srv.pool, otherID); n != 1 {
		t.Fatal("intruder's passkey was wrongly removed")
	}
}

func TestIntegrationListPasskeys(t *testing.T) {
	srv, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")
	login(t, c, ts.URL, "zach", "pw12345678", secret)
	uid := userIDByName(t, srv, "zach")
	ctx := context.Background()

	_ = InsertCredential(ctx, srv.pool, uid, fakeCredential("a"), "Laptop")
	_ = InsertCredential(ctx, srv.pool, uid, fakeCredential("b"), "Phone")

	resp, err := c.Get(ts.URL + "/api/passkeys")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusOK)
	list := decodeJSON[[]map[string]any](t, resp)
	if len(list) != 2 {
		t.Fatalf("listed %d passkeys, want 2", len(list))
	}
	names := map[string]bool{}
	for _, p := range list {
		names[p["name"].(string)] = true
	}
	if !names["Laptop"] || !names["Phone"] {
		t.Fatalf("passkey names = %v", names)
	}
}
