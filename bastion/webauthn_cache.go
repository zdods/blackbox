package main

import (
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

const webauthnCeremonyTTL = 5 * time.Minute

// pendingCeremony holds the server-side WebAuthn challenge state between the
// begin and finish steps of a ceremony. username is set only for first-run
// registration (where no user row exists yet). name is the friendly label for a
// registration/enroll ceremony. Both are empty for login.
type pendingCeremony struct {
	session   *webauthn.SessionData
	username  string
	name      string
	expiresAt time.Time
}

// WebAuthnSessionCache stores in-progress ceremony state keyed by an opaque id
// carried in a short-lived cookie. Mirrors TotpSetupCache: the challenge never
// leaves the server, and entries are single-use + TTL-expired.
type WebAuthnSessionCache struct {
	mu    sync.Mutex
	store map[string]pendingCeremony
}

func NewWebAuthnSessionCache() *WebAuthnSessionCache {
	c := &WebAuthnSessionCache{store: make(map[string]pendingCeremony)}
	go c.cleanup()
	return c
}

func (c *WebAuthnSessionCache) cleanup() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for id, p := range c.store {
			if now.After(p.expiresAt) {
				delete(c.store, id)
			}
		}
		c.mu.Unlock()
	}
}

func (c *WebAuthnSessionCache) Set(key string, sd *webauthn.SessionData, username, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = pendingCeremony{session: sd, username: username, name: name, expiresAt: time.Now().Add(webauthnCeremonyTTL)}
}

func (c *WebAuthnSessionCache) Get(key string) (pendingCeremony, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.store[key]
	if !ok || time.Now().After(p.expiresAt) {
		return pendingCeremony{}, false
	}
	return p, true
}

func (c *WebAuthnSessionCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, key)
}
