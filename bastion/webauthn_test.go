package main

import (
	"reflect"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/google/uuid"
)

func TestTransportsRoundTrip(t *testing.T) {
	in := []protocol.AuthenticatorTransport{protocol.Internal, protocol.Hybrid, protocol.USB}
	got := stringsToTransports(transportsToStrings(in))
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("round trip = %v, want %v", got, in)
	}
	// Empty/nil input yields an empty (non-panicking) slice.
	if s := transportsToStrings(nil); len(s) != 0 {
		t.Fatalf("transportsToStrings(nil) = %v, want empty", s)
	}
}

func TestWebauthnUserHandle(t *testing.T) {
	id := uuid.New()
	u := &webauthnUser{user: &User{ID: id.String(), Username: "zach"}}
	handle := u.WebAuthnID()
	if len(handle) != 16 {
		t.Fatalf("handle len = %d, want 16 raw bytes", len(handle))
	}
	got, err := uuid.FromBytes(handle)
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}
	if got != id {
		t.Fatalf("handle round trip = %s, want %s", got, id)
	}
	if u.WebAuthnName() != "zach" || u.WebAuthnDisplayName() != "zach" {
		t.Fatal("name/display name should be the username")
	}
}

func TestNewWebAuthn(t *testing.T) {
	// No RP_ID → passkeys disabled (nil, no error).
	wa, err := newWebAuthn(Config{})
	if err != nil || wa != nil {
		t.Fatalf("newWebAuthn(empty) = (%v, %v), want (nil, nil)", wa, err)
	}
	// Configured RP_ID → a relying party is built.
	wa, err = newWebAuthn(Config{RPID: "blackhaul.example.com", RPDisplayName: "Blackhaul", RPOrigins: []string{"https://blackhaul.example.com"}})
	if err != nil {
		t.Fatalf("newWebAuthn(configured): %v", err)
	}
	if wa == nil {
		t.Fatal("expected a configured *webauthn.WebAuthn")
	}
}

func TestLoadConfigPasskeyDefaults(t *testing.T) {
	// Defaults: password mode, Blackhaul display name, no origins without RP_ID.
	for _, k := range []string{"AUTH_MODE", "RP_ID", "RP_ORIGINS", "RP_DISPLAY_NAME"} {
		t.Setenv(k, "")
	}
	cfg := LoadConfig()
	if cfg.AuthMode != authModePassword {
		t.Fatalf("AuthMode default = %q, want password", cfg.AuthMode)
	}
	if cfg.RPDisplayName != "Blackhaul" {
		t.Fatalf("RPDisplayName default = %q, want Blackhaul", cfg.RPDisplayName)
	}
	if len(cfg.RPOrigins) != 0 {
		t.Fatalf("RPOrigins should be empty without RP_ID, got %v", cfg.RPOrigins)
	}

	// RP_ID set, RP_ORIGINS unset → origin defaults to https://<RP_ID>.
	t.Setenv("RP_ID", "blackhaul.example.com")
	cfg = LoadConfig()
	if want := []string{"https://blackhaul.example.com"}; !reflect.DeepEqual(cfg.RPOrigins, want) {
		t.Fatalf("default RPOrigins = %v, want %v", cfg.RPOrigins, want)
	}

	// Explicit comma-separated RP_ORIGINS are parsed and trimmed.
	t.Setenv("AUTH_MODE", "passkey")
	t.Setenv("RP_ORIGINS", "https://a.example.com, https://b.example.com ")
	cfg = LoadConfig()
	if cfg.AuthMode != authModePasskey {
		t.Fatalf("AuthMode = %q, want passkey", cfg.AuthMode)
	}
	if want := []string{"https://a.example.com", "https://b.example.com"}; !reflect.DeepEqual(cfg.RPOrigins, want) {
		t.Fatalf("parsed RPOrigins = %v, want %v", cfg.RPOrigins, want)
	}
}
