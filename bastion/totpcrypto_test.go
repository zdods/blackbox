package main

import "testing"

func TestTOTPEncryptRoundTrip(t *testing.T) {
	key, err := resolveTOTPKey("", "a-stable-jwt-secret-at-least-32-bytes-long", true)
	if err != nil || key == nil {
		t.Fatalf("resolveTOTPKey: key=%v err=%v", key != nil, err)
	}
	const secret = "JBSWY3DPEHPK3PXP"
	enc, err := encryptTOTPSecret(key, secret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == secret {
		t.Fatal("ciphertext equals plaintext (not encrypted)")
	}
	got, err := decryptTOTPSecret(key, enc)
	if err != nil || got != secret {
		t.Fatalf("decrypt = %q err=%v, want %q", got, err, secret)
	}
}

func TestTOTPDecryptLegacyPlaintextPassthrough(t *testing.T) {
	key, _ := resolveTOTPKey("", "a-stable-jwt-secret-at-least-32-bytes-long", true)
	// A value without the enc prefix is treated as legacy plaintext.
	got, err := decryptTOTPSecret(key, "PLAINTEXTSECRET")
	if err != nil || got != "PLAINTEXTSECRET" {
		t.Fatalf("legacy passthrough = %q err=%v", got, err)
	}
}

func TestTOTPKeyDisabledWhenUnstable(t *testing.T) {
	// Ephemeral JWT secret (not stable) and no TOTP_ENC_KEY → encryption off.
	key, err := resolveTOTPKey("", "whatever", false)
	if err != nil || key != nil {
		t.Fatalf("expected nil key when no stable secret, got key=%v err=%v", key != nil, err)
	}
	// With encryption disabled, secrets pass through unchanged both ways.
	enc, _ := encryptTOTPSecret(nil, "SECRET")
	if enc != "SECRET" {
		t.Errorf("disabled encrypt = %q, want passthrough", enc)
	}
}

func TestTOTPKeyRejectsBadEncKey(t *testing.T) {
	if _, err := resolveTOTPKey("not-base64-or-wrong-length", "", false); err == nil {
		t.Error("expected error for malformed TOTP_ENC_KEY")
	}
}

func TestTOTPDecryptWrongKeyFails(t *testing.T) {
	k1, _ := resolveTOTPKey("", "stable-jwt-secret-at-least-32-bytes-long!!", true)
	k2, _ := resolveTOTPKey("", "a-different-stable-secret-32-bytes-minimum", true)
	enc, _ := encryptTOTPSecret(k1, "JBSWY3DPEHPK3PXP")
	if _, err := decryptTOTPSecret(k2, enc); err == nil {
		t.Error("expected decryption failure with the wrong key")
	}
}
