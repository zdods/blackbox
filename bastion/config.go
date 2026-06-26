package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"strings"
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
	// LogFormat: "json" for JSON lines; anything else logs text.
	LogFormat string
	// LogLevel: "debug" to include static-asset access logs.
	LogLevel string
	// TOTPEncKey: base64 of 32 bytes used to encrypt TOTP secrets at rest
	// (TOTP_ENC_KEY). When empty, a key is derived from a stable JWT secret.
	TOTPEncKey string
	// CookieSecure: force the Secure flag on the session cookie (COOKIE_SECURE=1),
	// for deployments that terminate TLS at a reverse proxy.
	CookieSecure bool
	// AuthMode selects the unauthenticated login/registration methods offered:
	// "password" (username+password+TOTP, the default), "passkey" (WebAuthn), or
	// "both" (offer each side by side). Passkey enrollment/management is
	// authenticated and available regardless of mode.
	AuthMode string
	// RPID is the WebAuthn Relying Party ID — the registrable domain only, with
	// no scheme or port (e.g. "blackhaul.example.com"). Empty disables passkeys.
	RPID string
	// RPOrigins are the full allowed origins for WebAuthn ceremonies, with scheme
	// and (non-default) port (RP_ORIGINS, comma-separated). Defaults to
	// ["https://"+RPID] when RP_ID is set and RP_ORIGINS is empty.
	RPOrigins []string
	// RPDisplayName is the human-facing relying-party name shown by authenticators
	// (RP_DISPLAY_NAME). Defaults to "Blackhaul".
	RPDisplayName string
}

const (
	authModePassword = "password"
	authModePasskey  = "passkey"
	authModeBoth     = "both"
)

func LoadConfig() Config {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/blackhaul?sslmode=disable"
	}
	addr := os.Getenv("SERVER_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	authMode := os.Getenv("AUTH_MODE")
	if authMode == "" {
		authMode = authModePassword
	}
	rpID := os.Getenv("RP_ID")
	rpDisplay := os.Getenv("RP_DISPLAY_NAME")
	if rpDisplay == "" {
		rpDisplay = "Blackhaul"
	}
	var rpOrigins []string
	if v := os.Getenv("RP_ORIGINS"); v != "" {
		for _, o := range strings.Split(v, ",") {
			if o = strings.TrimSpace(o); o != "" {
				rpOrigins = append(rpOrigins, o)
			}
		}
	} else if rpID != "" {
		rpOrigins = []string{"https://" + rpID}
	}
	return Config{
		DatabaseURL:   dbURL,
		ServerAddr:    addr,
		JWTSecret:     os.Getenv("JWT_SECRET"),
		StaticDir:     os.Getenv("STATIC_DIR"),
		TLSCertFile:   os.Getenv("TLS_CERT_FILE"),
		TLSKeyFile:    os.Getenv("TLS_KEY_FILE"),
		CORSOrigin:    os.Getenv("CORS_ORIGIN"),
		DevMode:       boolEnv("DEV_MODE"),
		TrustProxy:    boolEnv("TRUST_PROXY"),
		LogFormat:     os.Getenv("LOG_FORMAT"),
		LogLevel:      os.Getenv("LOG_LEVEL"),
		TOTPEncKey:    os.Getenv("TOTP_ENC_KEY"),
		CookieSecure:  boolEnv("COOKIE_SECURE"),
		AuthMode:      authMode,
		RPID:          rpID,
		RPOrigins:     rpOrigins,
		RPDisplayName: rpDisplay,
	}
}

func boolEnv(key string) bool {
	v := os.Getenv(key)
	return v == "1" || v == "true"
}

// resolveJWTSecret picks the signing secret and reports whether it is stable
// (survives restarts). An explicit secret must be at least 32 bytes — a short
// HS256 key is brute-forceable offline from one captured token, after which an
// attacker can mint sessions. Otherwise: DEV_MODE keeps the stable dev secret,
// and production falls back to a random ephemeral secret (secure by default,
// but sessions reset on restart). Returns (secret, stable, warning, err).
func resolveJWTSecret(secret string, devMode bool) (string, bool, string, error) {
	if secret != "" && secret != devJWTSecret {
		if len(secret) < 32 {
			return "", false, "", errors.New("JWT_SECRET must be at least 32 bytes (generate one with: openssl rand -base64 32)")
		}
		return secret, true, "", nil
	}
	if devMode {
		return devJWTSecret, true, "DEV_MODE enabled; using the insecure development JWT secret", nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", false, "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), false,
		"JWT_SECRET not set; generated an ephemeral secret — sessions will not survive a restart. Set JWT_SECRET in production.",
		nil
}
