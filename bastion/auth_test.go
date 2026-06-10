package main

import (
	"testing"
	"time"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("test-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" {
		t.Fatal("hash should not be empty")
	}
	if !CheckPassword(hash, "test-password") {
		t.Error("CheckPassword should return true for correct password")
	}
	if CheckPassword(hash, "wrong-password") {
		t.Error("CheckPassword should return false for wrong password")
	}
}

func TestHashPasswordUniqueness(t *testing.T) {
	h1, _ := HashPassword("same")
	h2, _ := HashPassword("same")
	if h1 == h2 {
		t.Error("two hashes of the same password should differ (bcrypt uses random salt)")
	}
}

func TestIssueAndValidateToken(t *testing.T) {
	secret := "test-secret-key"
	token, err := IssueToken("user-123", "alice", 3, secret, time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if token == "" {
		t.Fatal("token should not be empty")
	}
	claims, err := ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-123")
	}
	if claims.Username != "alice" {
		t.Errorf("Username = %q, want %q", claims.Username, "alice")
	}
	if claims.Purpose != "" {
		t.Errorf("Purpose = %q, want empty", claims.Purpose)
	}
	if claims.Ver != 3 {
		t.Errorf("Ver = %d, want 3", claims.Ver)
	}
}

func TestValidateTokenWrongSecret(t *testing.T) {
	token, _ := IssueToken("user-123", "alice", 1, "secret-a", time.Hour)
	_, err := ValidateToken(token, "secret-b")
	if err == nil {
		t.Error("ValidateToken should fail with wrong secret")
	}
}

func TestValidateTokenExpired(t *testing.T) {
	token, _ := IssueToken("user-123", "alice", 1, "secret", -time.Hour)
	_, err := ValidateToken(token, "secret")
	if err == nil {
		t.Error("ValidateToken should fail with expired token")
	}
}

func TestIssueLoginToken(t *testing.T) {
	secret := "test-secret"
	token, err := IssueLoginToken("user-456", "bob", secret)
	if err != nil {
		t.Fatalf("IssueLoginToken: %v", err)
	}
	claims, err := ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Purpose != "totp_challenge" {
		t.Errorf("Purpose = %q, want %q", claims.Purpose, "totp_challenge")
	}
	if claims.UserID != "user-456" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-456")
	}
}

func TestValidateTokenRejectsNonHMAC(t *testing.T) {
	// Empty/garbage token
	_, err := ValidateToken("not.a.token", "secret")
	if err == nil {
		t.Error("ValidateToken should reject garbage token")
	}
}

func TestValidateTokenRejectsEmptyString(t *testing.T) {
	_, err := ValidateToken("", "secret")
	if err == nil {
		t.Error("ValidateToken should reject empty string")
	}
}
