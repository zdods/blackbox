package main

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
)

const totpSetupTTL = 15 * time.Minute

type pendingSecret struct {
	secret    string
	expiresAt time.Time
}

// TotpSetupCache holds TOTP secrets for in-progress registration. Key = setup_id.
type TotpSetupCache struct {
	mu    sync.Mutex
	store map[string]pendingSecret
}

func NewTotpSetupCache() *TotpSetupCache {
	c := &TotpSetupCache{store: make(map[string]pendingSecret)}
	go c.cleanup()
	return c
}

func (c *TotpSetupCache) cleanup() {
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

func (c *TotpSetupCache) Set(setupID, secret string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[setupID] = pendingSecret{secret: secret, expiresAt: time.Now().Add(totpSetupTTL)}
}

func (c *TotpSetupCache) Get(setupID string) (secret string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.store[setupID]
	if !ok || time.Now().After(p.expiresAt) {
		return "", false
	}
	return p.secret, true
}

func (c *TotpSetupCache) Delete(setupID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, setupID)
}

// GenerateTOTPSetup creates a new TOTP secret and provisioning URI. Returns setupID, secret, provisioningURI.
func GenerateTOTPSetup(issuer, accountName string) (setupID, secret, provisioningURI string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
	})
	if err != nil {
		return "", "", "", err
	}
	setupID = uuid.New().String()
	return setupID, key.Secret(), key.URL(), nil
}

// ValidateTOTP returns true if the code is valid for the given secret.
func ValidateTOTP(code, secret string) bool {
	return totp.Validate(code, secret)
}
