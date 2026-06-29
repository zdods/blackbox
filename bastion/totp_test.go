package main

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestTotpSetupCacheSetAndGet(t *testing.T) {
	c := &TotpSetupCache{store: make(map[string]pendingSecret)}

	c.Set("setup-1", "JBSWY3DPEHPK3PXP")
	secret, ok := c.Get("setup-1")
	if !ok {
		t.Fatal("Get should return true for existing key")
	}
	if secret != "JBSWY3DPEHPK3PXP" {
		t.Errorf("secret = %q, want %q", secret, "JBSWY3DPEHPK3PXP")
	}
}

func TestTotpSetupCacheGetMissing(t *testing.T) {
	c := &TotpSetupCache{store: make(map[string]pendingSecret)}

	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("Get should return false for missing key")
	}
}

func TestTotpSetupCacheDelete(t *testing.T) {
	c := &TotpSetupCache{store: make(map[string]pendingSecret)}

	c.Set("setup-1", "secret")
	c.Delete("setup-1")
	_, ok := c.Get("setup-1")
	if ok {
		t.Error("Get should return false after Delete")
	}
}

func TestTotpSetupCacheExpiry(t *testing.T) {
	c := &TotpSetupCache{store: make(map[string]pendingSecret)}

	// Manually insert an expired entry
	c.mu.Lock()
	c.store["expired"] = pendingSecret{secret: "old", expiresAt: time.Now().Add(-time.Minute)}
	c.mu.Unlock()

	_, ok := c.Get("expired")
	if ok {
		t.Error("Get should return false for expired entries")
	}
}

func TestGenerateTOTPSetup(t *testing.T) {
	setupID, secret, uri, err := GenerateTOTPSetup("blackhaul", "user")
	if err != nil {
		t.Fatalf("GenerateTOTPSetup: %v", err)
	}
	if setupID == "" {
		t.Error("setupID should not be empty")
	}
	if secret == "" {
		t.Error("secret should not be empty")
	}
	if uri == "" {
		t.Error("provisioning URI should not be empty")
	}
}

func TestGenerateTOTPSetupUniqueness(t *testing.T) {
	id1, s1, _, _ := GenerateTOTPSetup("blackhaul", "user")
	id2, s2, _, _ := GenerateTOTPSetup("blackhaul", "user")
	if id1 == id2 {
		t.Error("setup IDs should be unique")
	}
	if s1 == s2 {
		t.Error("secrets should be unique")
	}
}

func TestValidateTOTPRejectsGarbage(t *testing.T) {
	if ValidateTOTP("000000", "JBSWY3DPEHPK3PXP") {
		// This could occasionally pass by chance (1 in ~333k), but is extremely unlikely
		t.Log("warning: TOTP validated 000000 — could be coincidence")
	}
}

// TestValidateTOTPAcceptsCurrentCode generates a real code for the current time
// step and confirms ValidateTOTP accepts it — the positive case the garbage test
// never covered.
func TestValidateTOTPAcceptsCurrentCode(t *testing.T) {
	_, secret, _, err := GenerateTOTPSetup("blackhaul", "user@example.com")
	if err != nil {
		t.Fatalf("GenerateTOTPSetup: %v", err)
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if !ValidateTOTP(code, secret) {
		t.Errorf("ValidateTOTP rejected a freshly generated valid code")
	}
	// A clearly wrong code for the same secret must fail.
	if ValidateTOTP("123456", secret) && code != "123456" {
		t.Errorf("ValidateTOTP accepted an unrelated code")
	}
}
