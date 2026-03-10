package main

import (
	"os"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	// Clear env vars that would override defaults
	for _, key := range []string{"DATABASE_URL", "SERVER_ADDR", "JWT_SECRET", "STATIC_DIR", "TLS_CERT_FILE", "TLS_KEY_FILE", "CORS_ORIGIN"} {
		t.Setenv(key, "")
	}

	cfg := LoadConfig()

	if cfg.DatabaseURL != "postgres://postgres:postgres@localhost:5432/blackbox?sslmode=disable" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.ServerAddr != ":8080" {
		t.Errorf("ServerAddr = %q", cfg.ServerAddr)
	}
	if cfg.JWTSecret != "dev-secret-change-in-production" {
		t.Errorf("JWTSecret = %q", cfg.JWTSecret)
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
	for _, key := range []string{"BLACKBOX_BASTION_URL", "BLACKBOX_TOKEN", "BLACKBOX_HOSTED_PATH"} {
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
