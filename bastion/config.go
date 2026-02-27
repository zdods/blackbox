package main

import (
	"os"
)

type Config struct {
	DatabaseURL string
	ServerAddr  string
	JWTSecret   string
	StaticDir   string // optional: serve web app from this dir (e.g. web/build)
	// TLS: if both set, server listens with TLS (HTTPS/WSS). Agents use wss://.
	TLSCertFile string
	TLSKeyFile  string
	// CORSOrigin: if set, sent as Access-Control-Allow-Origin; empty means "*"
	CORSOrigin string
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
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "dev-secret-change-in-production"
	}
	staticDir := os.Getenv("STATIC_DIR")
	tlsCert := os.Getenv("TLS_CERT_FILE")
	tlsKey := os.Getenv("TLS_KEY_FILE")
	corsOrigin := os.Getenv("CORS_ORIGIN")
	return Config{
		DatabaseURL: dbURL,
		ServerAddr:  addr,
		JWTSecret:   jwtSecret,
		StaticDir:   staticDir,
		TLSCertFile: tlsCert,
		TLSKeyFile:  tlsKey,
		CORSOrigin:  corsOrigin,
	}
}
