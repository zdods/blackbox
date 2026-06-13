package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter is an in-memory sliding-window limiter keyed by an arbitrary
// string (client IP, user ID, ...). It is safe for concurrent use.
type RateLimiter struct {
	mu        sync.Mutex
	limit     int
	window    time.Duration
	attempts  map[string][]time.Time
	lastSweep time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limit:    limit,
		window:   window,
		attempts: make(map[string][]time.Time),
	}
}

// Allow records an attempt for key and reports whether it stays within the
// limit. Attempts over the limit are still recorded, so hammering extends
// the lockout.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	rl.sweep(now)
	recent := rl.prune(key, now)
	rl.attempts[key] = append(recent, now)
	return len(recent) < rl.limit
}

// Record adds an attempt for key without gating (e.g. count a failed TOTP code).
func (rl *RateLimiter) Record(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	rl.sweep(now)
	rl.attempts[key] = append(rl.prune(key, now), now)
}

// Blocked reports whether key has reached the limit, without recording an attempt.
func (rl *RateLimiter) Blocked(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	recent := rl.prune(key, now)
	rl.attempts[key] = recent
	return len(recent) >= rl.limit
}

// Reset clears attempts for key (e.g. after a successful login).
func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, key)
}

// prune returns the attempts for key still inside the window. Caller must hold mu.
func (rl *RateLimiter) prune(key string, now time.Time) []time.Time {
	cutoff := now.Add(-rl.window)
	all := rl.attempts[key]
	i := 0
	for ; i < len(all); i++ {
		if all[i].After(cutoff) {
			break
		}
	}
	return all[i:]
}

// sweep drops keys with no recent attempts so the map cannot grow unbounded.
// Runs at most once per window. Caller must hold mu.
func (rl *RateLimiter) sweep(now time.Time) {
	if now.Sub(rl.lastSweep) < rl.window {
		return
	}
	rl.lastSweep = now
	cutoff := now.Add(-rl.window)
	for key, all := range rl.attempts {
		if len(all) == 0 || !all[len(all)-1].After(cutoff) {
			delete(rl.attempts, key)
		}
	}
}

// clientIP extracts the caller's IP for rate-limit keying. With trustProxy set
// (deployments behind a reverse proxy), the *right-most* X-Forwarded-For hop is
// used — the address the trusted proxy observed and appended. The left-most
// entries are client-supplied and spoofable, so using them would let an
// attacker rotate the header to dodge the limiter. Otherwise the connection's
// remote address is authoritative.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
