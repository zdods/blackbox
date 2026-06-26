//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// doJSON issues a method+JSON request through the client's cookie jar.
func doJSON(t *testing.T, c *http.Client, method, url string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func TestIntegrationAccountProfile(t *testing.T) {
	_, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "pw12345678")
	login(t, c, ts.URL, "zach", "pw12345678", secret)

	// Fresh account: username set, no email, has a password, password mode.
	resp := c2get(t, c, ts.URL+"/api/account")
	got := decodeJSON[map[string]any](t, resp)
	if got["username"] != "zach" {
		t.Fatalf("username = %v, want zach", got["username"])
	}
	if got["email"] != "" {
		t.Fatalf("email = %v, want empty", got["email"])
	}
	if got["has_password"] != true || got["password_enabled"] != true {
		t.Fatalf("password flags = %v", got)
	}
	if got["passkey_enabled"] != false {
		t.Fatalf("passkey_enabled = %v, want false (no RP configured)", got["passkey_enabled"])
	}

	// Set a valid email.
	resp = doJSON(t, c, http.MethodPatch, ts.URL+"/api/account", map[string]string{"email": "Zach@Zdods.com"})
	wantStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = c2get(t, c, ts.URL+"/api/account")
	got = decodeJSON[map[string]any](t, resp)
	if got["email"] != "zach@zdods.com" { // normalized to lowercase
		t.Fatalf("email = %v, want zach@zdods.com", got["email"])
	}

	// Reject a malformed email.
	resp = doJSON(t, c, http.MethodPatch, ts.URL+"/api/account", map[string]string{"email": "nope"})
	wantStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()

	// Clearing with an empty string is allowed and removes the address.
	resp = doJSON(t, c, http.MethodPatch, ts.URL+"/api/account", map[string]string{"email": ""})
	wantStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = c2get(t, c, ts.URL+"/api/account")
	got = decodeJSON[map[string]any](t, resp)
	if got["email"] != "" {
		t.Fatalf("email after clear = %v, want empty", got["email"])
	}
}

func TestIntegrationChangePassword(t *testing.T) {
	_, ts := newTestServer(t)
	c := client(t)
	secret := registerUser(t, c, ts.URL, "zach", "old-password-123")
	oldCookie := login(t, c, ts.URL, "zach", "old-password-123", secret)

	// Wrong current password is rejected.
	resp := doJSON(t, c, http.MethodPost, ts.URL+"/api/account/password",
		map[string]string{"current_password": "wrong", "new_password": "new-password-456"})
	wantStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()

	// A too-short new password is rejected.
	resp = doJSON(t, c, http.MethodPost, ts.URL+"/api/account/password",
		map[string]string{"current_password": "old-password-123", "new_password": "short"})
	wantStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()

	// Correct current + valid new password succeeds.
	resp = doJSON(t, c, http.MethodPost, ts.URL+"/api/account/password",
		map[string]string{"current_password": "old-password-123", "new_password": "new-password-456"})
	wantStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// The current client got a fresh cookie and stays authenticated.
	resp = c2get(t, c, ts.URL+"/api/account")
	wantStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// The pre-change session cookie is revoked (token_version bumped).
	req, _ := http.NewRequest("GET", ts.URL+"/api/me", nil)
	req.AddCookie(oldCookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()

	// The old password no longer works; the new one does.
	fresh := client(t)
	resp = postJSON(t, fresh, ts.URL+"/api/login", map[string]string{"username": "zach", "password": "old-password-123"})
	wantStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
	login(t, fresh, ts.URL, "zach", "new-password-456", secret)
}

// c2get is a tiny GET helper that fails the test on transport error.
func c2get(t *testing.T, c *http.Client, url string) *http.Response {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}
