package main

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterAllowWithinLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.Allow("ip-1") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if rl.Allow("ip-1") {
		t.Error("attempt over the limit should be denied")
	}
}

func TestRateLimiterKeysIndependent(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	if !rl.Allow("ip-1") {
		t.Fatal("first attempt for ip-1 should be allowed")
	}
	if !rl.Allow("ip-2") {
		t.Error("ip-2 should not be affected by ip-1's attempts")
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	rl := NewRateLimiter(1, 30*time.Millisecond)
	if !rl.Allow("ip-1") {
		t.Fatal("first attempt should be allowed")
	}
	if rl.Allow("ip-1") {
		t.Fatal("second attempt inside window should be denied")
	}
	time.Sleep(40 * time.Millisecond)
	if !rl.Allow("ip-1") {
		t.Error("attempt after window expiry should be allowed")
	}
}

func TestRateLimiterRecordAndBlocked(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	if rl.Blocked("user-1") {
		t.Fatal("fresh key should not be blocked")
	}
	rl.Record("user-1")
	if rl.Blocked("user-1") {
		t.Fatal("one failure of two should not block")
	}
	rl.Record("user-1")
	if !rl.Blocked("user-1") {
		t.Error("reaching the limit should block")
	}
	// Blocked must not record attempts itself.
	if !rl.Blocked("user-1") {
		t.Error("Blocked should be repeatable without changing state")
	}
}

func TestRateLimiterReset(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	rl.Record("user-1")
	if !rl.Blocked("user-1") {
		t.Fatal("should be blocked before reset")
	}
	rl.Reset("user-1")
	if rl.Blocked("user-1") {
		t.Error("reset should clear the block")
	}
}

func TestClientIPRemoteAddr(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "203.0.113.7:51234"
	if ip := clientIP(r, false); ip != "203.0.113.7" {
		t.Errorf("clientIP = %q, want 203.0.113.7", ip)
	}
}

func TestClientIPIgnoresForwardedForByDefault(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "203.0.113.7:51234"
	r.Header.Set("X-Forwarded-For", "198.51.100.1")
	if ip := clientIP(r, false); ip != "203.0.113.7" {
		t.Errorf("clientIP = %q; X-Forwarded-For must be ignored unless TRUST_PROXY is set", ip)
	}
}

func TestClientIPTrustProxy(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "10.0.0.2:51234"
	r.Header.Set("X-Forwarded-For", "198.51.100.1, 10.0.0.2")
	if ip := clientIP(r, true); ip != "198.51.100.1" {
		t.Errorf("clientIP = %q, want first X-Forwarded-For hop", ip)
	}
}

func TestHashDaemonTokenDeterministic(t *testing.T) {
	h1 := HashDaemonToken("token-a")
	h2 := HashDaemonToken("token-a")
	if h1 != h2 {
		t.Error("same token should hash identically")
	}
	if h1 == HashDaemonToken("token-b") {
		t.Error("different tokens should hash differently")
	}
	if h1 == "token-a" {
		t.Error("hash should not equal the plaintext token")
	}
}
