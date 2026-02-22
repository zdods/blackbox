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
		jwtSecret = "poc-secret-change-in-production"
	}
	staticDir := os.Getenv("STATIC_DIR")
	tlsCert := os.Getenv("TLS_CERT_FILE")
	tlsKey := os.Getenv("TLS_KEY_FILE")
	return Config{
		DatabaseURL: dbURL,
		ServerAddr:  addr,
		JWTSecret:   jwtSecret,
		StaticDir:   staticDir,
		TLSCertFile: tlsCert,
		TLSKeyFile:  tlsKey,
	}
}
