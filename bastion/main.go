package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"blackhaul/pkg/version"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	pool             *pgxpool.Pool
	cfg              Config
	hub              *Hub
	totpCache        *TotpSetupCache
	authLimiter      *RateLimiter          // per-IP gate on auth endpoints
	loginFailLimiter *RateLimiter          // per-account gate on failed passwords
	totpFailLimiter  *RateLimiter          // per-user lockout on failed TOTP codes
	totpKey          []byte                // TOTP-secret encryption key (nil = disabled)
	transferSem      chan struct{}         // bounds concurrent file uploads/downloads
	webAuthn         *webauthn.WebAuthn    // nil when RP_ID unset (passkeys disabled)
	passkeyCache     *WebAuthnSessionCache // in-progress WebAuthn ceremony state
}

func main() {
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("blackhaul-bastion " + version.Version)
		return
	}
	cfg := LoadConfig()
	setupLogger(cfg.LogFormat, cfg.LogLevel)
	slog.Info("starting blackhaul-bastion", "version", version.Version)
	secret, jwtStable, warning, err := resolveJWTSecret(cfg.JWTSecret, cfg.DevMode)
	if err != nil {
		slog.Error("jwt secret", "err", err)
		os.Exit(1)
	}
	if warning != "" {
		slog.Warn(warning)
	}
	cfg.JWTSecret = secret
	totpKey, err := resolveTOTPKey(cfg.TOTPEncKey, secret, jwtStable)
	if err != nil {
		slog.Error("totp encryption key", "err", err)
		os.Exit(1)
	}
	if totpKey == nil {
		slog.Warn("TOTP secrets stored in plaintext — set JWT_SECRET (or TOTP_ENC_KEY) to encrypt them at rest")
	}
	switch cfg.AuthMode {
	case authModePassword, authModePasskey, authModeBoth:
	default:
		slog.Error("invalid AUTH_MODE (want \"password\", \"passkey\", or \"both\")", "value", cfg.AuthMode)
		os.Exit(1)
	}
	if cfg.AuthMode == authModePasskey && cfg.RPID == "" {
		slog.Error("AUTH_MODE=passkey requires RP_ID (the WebAuthn relying-party domain)")
		os.Exit(1)
	}
	if cfg.AuthMode == authModeBoth && cfg.RPID == "" {
		slog.Warn("AUTH_MODE=both but RP_ID is unset — passkeys disabled, password sign-in only")
	}
	webAuthn, err := newWebAuthn(cfg)
	if err != nil {
		slog.Error("webauthn init", "err", err)
		os.Exit(1)
	}
	if webAuthn != nil {
		slog.Info("passkeys enabled", "rp_id", cfg.RPID, "rp_origins", cfg.RPOrigins, "auth_mode", cfg.AuthMode)
	}
	ctx := context.Background()
	pool, err := OpenDB(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("database connected")
	if err := RunMigrations(ctx, pool); err != nil {
		slog.Error("migrations failed", "err", err)
		os.Exit(1)
	}
	if err := backfillTOTPEncryption(ctx, pool, totpKey); err != nil {
		slog.Error("totp backfill failed", "err", err)
		os.Exit(1)
	}
	hub := NewHub()
	totpCache := NewTotpSetupCache()
	srv := &Server{
		pool:             pool,
		cfg:              cfg,
		hub:              hub,
		totpCache:        totpCache,
		authLimiter:      NewRateLimiter(10, time.Minute),
		loginFailLimiter: NewRateLimiter(5, 15*time.Minute),
		totpFailLimiter:  NewRateLimiter(5, 15*time.Minute),
		totpKey:          totpKey,
		transferSem:      make(chan struct{}, maxConcurrentTransfers),
		webAuthn:         webAuthn,
		passkeyCache:     NewWebAuthnSessionCache(),
	}
	httpServer := &http.Server{Addr: cfg.ServerAddr, Handler: srv.routes()}
	go func() {
		var err error
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			slog.Info("server listening", "addr", cfg.ServerAddr, "tls", true)
			err = httpServer.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			slog.Info("server listening", "addr", cfg.ServerAddr, "tls", false)
			err = httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutdown signal received, stopping server")
	if err := httpServer.Shutdown(context.Background()); err != nil {
		slog.Error("shutdown", "err", err)
	} else {
		slog.Info("server stopped")
	}
}

// routes builds the full HTTP surface (API, daemon WebSocket, static web
// app) wrapped in the CORS middleware. Factored out of main so tests can
// drive the real handler stack via httptest.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	// Health (public; used by Docker HEALTHCHECK and uptime monitors)
	mux.HandleFunc("GET /healthz", s.Healthz)
	// Auth (public)
	mux.HandleFunc("GET /api/setup", s.Setup)
	mux.HandleFunc("POST /api/register", s.Register)
	mux.HandleFunc("POST /api/register/totp-setup", s.RegisterTOTPSetup)
	mux.HandleFunc("POST /api/login", s.Login)
	mux.HandleFunc("POST /api/login/totp", s.LoginTOTP)
	mux.HandleFunc("POST /api/logout", s.AuthMiddleware(s.Logout))
	// Passkey auth (public; live only when AUTH_MODE=passkey and RP_ID is set)
	mux.HandleFunc("POST /api/passkey/login/begin", s.PasskeyLoginBegin)
	mux.HandleFunc("POST /api/passkey/login/finish", s.PasskeyLoginFinish)
	mux.HandleFunc("POST /api/passkey/register/begin", s.PasskeyRegisterBegin)
	mux.HandleFunc("POST /api/passkey/register/finish", s.PasskeyRegisterFinish)
	// Passkey enrollment + management (authenticated; available in both modes)
	mux.HandleFunc("GET /api/passkeys", s.AuthMiddleware(s.ListPasskeys))
	mux.HandleFunc("POST /api/passkeys/enroll/begin", s.AuthMiddleware(s.PasskeyEnrollBegin))
	mux.HandleFunc("POST /api/passkeys/enroll/finish", s.AuthMiddleware(s.PasskeyEnrollFinish))
	mux.HandleFunc("DELETE /api/passkeys/{id}", s.AuthMiddleware(s.DeletePasskey))
	// Protected
	mux.HandleFunc("GET /api/me", s.AuthMiddleware(s.Me))
	mux.HandleFunc("GET /api/daemons", s.AuthMiddleware(s.ListDaemons))
	mux.HandleFunc("POST /api/daemons", s.AuthMiddleware(s.CreateDaemon))
	mux.HandleFunc("PATCH /api/daemons/{id}", s.AuthMiddleware(s.UpdateDaemon))
	mux.HandleFunc("DELETE /api/daemons/{id}", s.AuthMiddleware(s.DeleteDaemon))
	mux.HandleFunc("GET /api/daemons/{id}/files", s.AuthMiddleware(s.DaemonFiles))
	mux.HandleFunc("PUT /api/daemons/{id}/files", s.AuthMiddleware(s.DaemonFiles))
	mux.HandleFunc("DELETE /api/daemons/{id}/files", s.AuthMiddleware(s.DaemonFiles))
	mux.HandleFunc("GET /api/daemons/{id}/meta", s.AuthMiddleware(s.DaemonMeta))
	// Daemon WebSocket (no session; daemon uses token)
	mux.HandleFunc("GET /ws/daemon", s.HandleDaemonWS)
	// Static web app (SPA fallback to index.html); single pattern catches all GET requests not matched above
	mux.Handle("GET /{path...}", staticHandler(s.cfg.StaticDir))
	tlsEnabled := s.cfg.TLSCertFile != "" && s.cfg.TLSKeyFile != ""
	return withRequestID(accessLog(securityHeaders(tlsEnabled)(corsThenMux(s.cfg, mux))))
}

// Healthz reports liveness: 200 when the server is up and the DB responds.
func (s *Server) Healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database unreachable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func corsThenMux(cfg Config, mux http.Handler) http.Handler {
	allowed := cfg.CORSOrigin
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only advertise CORS when an origin is configured and the request's
		// Origin matches it. With no CORS_ORIGIN the API is same-origin only;
		// emitting a permissive method/header set unconditionally is misleading.
		origin := r.Header.Get("Origin")
		if allowed != "" && origin != "" && (allowed == "*" || origin == allowed) {
			if allowed == "*" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", allowed)
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
