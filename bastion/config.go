package main

import (
	"crypto/rand"
	"encoding/base64"
	"os"
)

// devJWTSecret is the well-known development secret. It is never used to sign
// tokens unless DEV_MODE is explicitly enabled.
const devJWTSecret = "dev-secret-change-in-production"

type Config struct {
	DatabaseURL string
	ServerAddr  string
	JWTSecret   string
	StaticDir   string // optional: serve web app from this dir (e.g. web/build)
	// TLS: if both set, server listens with TLS (HTTPS/WSS). Daemons use wss://.
	TLSCertFile string
	TLSKeyFile  string
	// CORSOrigin: if set, sent as Access-Control-Allow-Origin; empty means "*"
	CORSOrigin string
	// DevMode: allow the insecure development JWT secret (DEV_MODE=1).
	DevMode bool
	// TrustProxy: trust X-Forwarded-For for client IPs (TRUST_PROXY=1; set only
	// behind a reverse proxy that overwrites the header).
	TrustProxy bool
}

func LoadConfig() Config {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/blackbox?sslmode=disable"
	}
	addr := os.Getenv("SERVER_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	return Config{
		DatabaseURL: dbURL,
		ServerAddr:  addr,
		JWTSecret:   os.Getenv("JWT_SECRET"),
		StaticDir:   os.Getenv("STATIC_DIR"),
		TLSCertFile: os.Getenv("TLS_CERT_FILE"),
		TLSKeyFile:  os.Getenv("TLS_KEY_FILE"),
		CORSOrigin:  os.Getenv("CORS_ORIGIN"),
		DevMode:     boolEnv("DEV_MODE"),
		TrustProxy:  boolEnv("TRUST_PROXY"),
	}
}

func boolEnv(key string) bool {
	v := os.Getenv(key)
	return v == "1" || v == "true"
}

// resolveJWTSecret picks the signing secret. An explicit non-default secret is
// used as-is. Otherwise: DEV_MODE keeps the stable dev secret (so dev sessions
// survive rebuilds), and production falls back to a random ephemeral secret —
// secure by default, but sessions reset on restart. Returns the secret and a
// warning to log (empty when an explicit secret was provided).
func resolveJWTSecret(secret string, devMode bool) (string, string, error) {
	if secret != "" && secret != devJWTSecret {
		return secret, "", nil
	}
	if devMode {
		return devJWTSecret, "DEV_MODE enabled; using the insecure development JWT secret", nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(b),
		"JWT_SECRET not set; generated an ephemeral secret — sessions will not survive a restart. Set JWT_SECRET in production.",
		nil
}
