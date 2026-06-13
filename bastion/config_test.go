package main

import (
	"os"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	// Clear env vars that would override defaults
	for _, key := range []string{"DATABASE_URL", "SERVER_ADDR", "JWT_SECRET", "STATIC_DIR", "TLS_CERT_FILE", "TLS_KEY_FILE", "CORS_ORIGIN", "DEV_MODE", "TRUST_PROXY"} {
		t.Setenv(key, "")
	}

	cfg := LoadConfig()

	if cfg.DatabaseURL != "postgres://postgres:postgres@localhost:5432/blackhaul?sslmode=disable" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.ServerAddr != ":8080" {
		t.Errorf("ServerAddr = %q", cfg.ServerAddr)
	}
	if cfg.JWTSecret != "" {
		t.Errorf("JWTSecret = %q, want empty (resolved at startup)", cfg.JWTSecret)
	}
	if cfg.DevMode || cfg.TrustProxy {
		t.Errorf("DevMode = %v, TrustProxy = %v, want false", cfg.DevMode, cfg.TrustProxy)
	}
	if cfg.StaticDir != "" {
		t.Errorf("StaticDir = %q, want empty", cfg.StaticDir)
	}
	if cfg.TLSCertFile != "" {
		t.Errorf("TLSCertFile = %q, want empty", cfg.TLSCertFile)
	}
	if cfg.CORSOrigin != "" {
		t.Errorf("CORSOrigin = %q, want empty", cfg.CORSOrigin)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://custom:5433/mydb")
	t.Setenv("SERVER_ADDR", ":9090")
	t.Setenv("JWT_SECRET", "super-secret")
	t.Setenv("STATIC_DIR", "/var/www")
	t.Setenv("TLS_CERT_FILE", "/etc/cert.pem")
	t.Setenv("TLS_KEY_FILE", "/etc/key.pem")
	t.Setenv("CORS_ORIGIN", "https://myapp.com")

	// Clear any other env vars that could interfere
	for _, key := range []string{"BLACKHAUL_BASTION_URL", "BLACKHAUL_TOKEN", "BLACKHAUL_HOSTED_PATH"} {
		os.Unsetenv(key)
	}

	cfg := LoadConfig()

	if cfg.DatabaseURL != "postgres://custom:5433/mydb" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.ServerAddr != ":9090" {
		t.Errorf("ServerAddr = %q", cfg.ServerAddr)
	}
	if cfg.JWTSecret != "super-secret" {
		t.Errorf("JWTSecret = %q", cfg.JWTSecret)
	}
	if cfg.StaticDir != "/var/www" {
		t.Errorf("StaticDir = %q", cfg.StaticDir)
	}
	if cfg.TLSCertFile != "/etc/cert.pem" {
		t.Errorf("TLSCertFile = %q", cfg.TLSCertFile)
	}
	if cfg.TLSKeyFile != "/etc/key.pem" {
		t.Errorf("TLSKeyFile = %q", cfg.TLSKeyFile)
	}
	if cfg.CORSOrigin != "https://myapp.com" {
		t.Errorf("CORSOrigin = %q", cfg.CORSOrigin)
	}
}

func TestResolveJWTSecretExplicit(t *testing.T) {
	const explicit = "a-real-production-secret-at-least-32-bytes-long"
	secret, stable, warning, err := resolveJWTSecret(explicit, false)
	if err != nil {
		t.Fatalf("resolveJWTSecret: %v", err)
	}
	if secret != explicit {
		t.Errorf("secret = %q, want explicit secret unchanged", secret)
	}
	if !stable {
		t.Error("explicit secret should be reported stable")
	}
	if warning != "" {
		t.Errorf("warning = %q, want empty", warning)
	}
}

func TestResolveJWTSecretRejectsShort(t *testing.T) {
	if _, _, _, err := resolveJWTSecret("too-short", false); err == nil {
		t.Error("expected error for a sub-32-byte JWT secret")
	}
}

func TestResolveJWTSecretDevMode(t *testing.T) {
	for _, in := range []string{"", devJWTSecret} {
		secret, _, warning, err := resolveJWTSecret(in, true)
		if err != nil {
			t.Fatalf("resolveJWTSecret(%q): %v", in, err)
		}
		if secret != devJWTSecret {
			t.Errorf("secret = %q, want dev secret in DEV_MODE", secret)
		}
		if warning == "" {
			t.Error("DEV_MODE should produce a warning")
		}
	}
}

func TestResolveJWTSecretEphemeral(t *testing.T) {
	// Unset or default secret outside DEV_MODE must never be used as-is.
	for _, in := range []string{"", devJWTSecret} {
		secret, _, warning, err := resolveJWTSecret(in, false)
		if err != nil {
			t.Fatalf("resolveJWTSecret(%q): %v", in, err)
		}
		if secret == "" || secret == devJWTSecret {
			t.Errorf("secret = %q, want random ephemeral secret", secret)
		}
		if warning == "" {
			t.Error("ephemeral secret should produce a warning")
		}
	}
	a, _, _, _ := resolveJWTSecret("", false)
	b, _, _, _ := resolveJWTSecret("", false)
	if a == b {
		t.Error("ephemeral secrets should be random, got identical values")
	}
}
