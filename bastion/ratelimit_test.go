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
	r.RemoteAddr = "10.0.0.2:51234" // the reverse proxy's connection
	// The trusted proxy appends the real client IP as the right-most hop.
	r.Header.Set("X-Forwarded-For", "198.51.100.1, 203.0.113.9")
	if ip := clientIP(r, true); ip != "203.0.113.9" {
		t.Errorf("clientIP = %q, want right-most hop 203.0.113.9", ip)
	}
}

func TestClientIPTrustProxyIgnoresSpoofedLeftEntries(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "10.0.0.2:51234"
	// An attacker rotating the left-most (client-supplied) value must not get a
	// fresh rate-limit bucket; the right-most hop added by the proxy is stable.
	for _, spoof := range []string{"1.1.1.1", "2.2.2.2"} {
		r.Header.Set("X-Forwarded-For", spoof+", 203.0.113.9")
		if ip := clientIP(r, true); ip != "203.0.113.9" {
			t.Errorf("clientIP = %q, want 203.0.113.9 regardless of spoofed left entry", ip)
		}
	}
}

func TestRateLimiterSweepEvictsStaleKeys(t *testing.T) {
	rl := NewRateLimiter(5, 50*time.Millisecond)
	now := time.Now()

	rl.mu.Lock()
	// "stale" last attempt is older than the window; "fresh" is current.
	rl.attempts["stale"] = []time.Time{now.Add(-time.Hour)}
	rl.attempts["fresh"] = []time.Time{now}
	// Force the once-per-window guard to allow this sweep.
	rl.lastSweep = now.Add(-time.Hour)
	rl.sweep(now)
	_, staleExists := rl.attempts["stale"]
	_, freshExists := rl.attempts["fresh"]
	rl.mu.Unlock()

	if staleExists {
		t.Error("sweep should evict a key whose last attempt is outside the window")
	}
	if !freshExists {
		t.Error("sweep must keep a key with a recent attempt")
	}
}

func TestRateLimiterSweepRunsAtMostOncePerWindow(t *testing.T) {
	rl := NewRateLimiter(5, time.Hour)
	now := time.Now()
	rl.mu.Lock()
	rl.attempts["stale"] = []time.Time{now.Add(-2 * time.Hour)}
	rl.lastSweep = now // just swept; another sweep must be a no-op
	rl.sweep(now)
	_, exists := rl.attempts["stale"]
	rl.mu.Unlock()
	if !exists {
		t.Error("sweep ran again within the same window; stale key removed too eagerly")
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
