package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	for _, tls := range []bool{false, true} {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
		h := securityHeaders(tls)(next)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
		}
		if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
			t.Errorf("X-Frame-Options = %q, want DENY", got)
		}
		if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
			t.Errorf("Referrer-Policy = %q, want no-referrer", got)
		}
		hsts := rec.Header().Get("Strict-Transport-Security")
		if tls && hsts == "" {
			t.Error("HSTS header missing when TLS enabled")
		}
		if !tls && hsts != "" {
			t.Errorf("HSTS header set without TLS: %q", hsts)
		}
	}
}

func TestIsSafeMethod(t *testing.T) {
	safe := []string{http.MethodGet, http.MethodHead, http.MethodOptions}
	unsafe := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, m := range safe {
		if !isSafeMethod(m) {
			t.Errorf("isSafeMethod(%q) = false, want true", m)
		}
	}
	for _, m := range unsafe {
		if isSafeMethod(m) {
			t.Errorf("isSafeMethod(%q) = true, want false", m)
		}
	}
}

func TestSameOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{"missing origin allowed", "", "app.example.com", true},
		{"match", "https://app.example.com", "app.example.com", true},
		{"case-insensitive host", "https://APP.example.com", "app.example.com", true},
		{"cross-origin", "https://evil.com", "app.example.com", false},
		{"unparseable origin", "://bad", "app.example.com", false},
		{"origin without host", "https://", "app.example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r.Host = tt.host
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if got := sameOrigin(r); got != tt.want {
				t.Errorf("sameOrigin(origin=%q host=%q) = %v, want %v", tt.origin, tt.host, got, tt.want)
			}
		})
	}
}

// TestAuthMiddlewareRejections covers every rejection branch that fires before
// the token-version DB lookup, so it runs without a database.
func TestAuthMiddlewareRejections(t *testing.T) {
	const secret = "test-secret-test-secret-test-secret-32"
	srv := &Server{cfg: Config{JWTSecret: secret}}
	called := false
	guard := srv.AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	loginToken, err := IssueLoginToken("u1", "alice", secret)
	if err != nil {
		t.Fatalf("IssueLoginToken: %v", err)
	}

	tests := []struct {
		name     string
		method   string
		setup    func(r *http.Request)
		wantCode int
		wantNext bool
	}{
		{
			name:     "no token",
			method:   http.MethodGet,
			setup:    func(r *http.Request) {},
			wantCode: http.StatusUnauthorized,
		},
		{
			name:   "csrf cross-origin with cookie",
			method: http.MethodPost,
			setup: func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: "session", Value: "anything"})
				r.Host = "app.example.com"
				r.Header.Set("Origin", "https://evil.com")
			},
			wantCode: http.StatusForbidden,
		},
		{
			name:   "invalid token",
			method: http.MethodGet,
			setup: func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: "session", Value: "not-a-jwt"})
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name:   "wrong purpose (login token) via bearer",
			method: http.MethodGet,
			setup: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+loginToken)
			},
			wantCode: http.StatusUnauthorized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = false
			r := httptest.NewRequest(tt.method, "/api/account", nil)
			tt.setup(r)
			rec := httptest.NewRecorder()
			guard(rec, r)
			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if called != tt.wantNext {
				t.Errorf("next called = %v, want %v", called, tt.wantNext)
			}
		})
	}
}
