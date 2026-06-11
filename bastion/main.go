package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"blackbox/pkg/version"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	pool            *pgxpool.Pool
	cfg             Config
	hub             *Hub
	totpCache       *TotpSetupCache
	authLimiter     *RateLimiter // per-IP gate on auth endpoints
	totpFailLimiter *RateLimiter // per-user lockout on failed TOTP codes
}

func main() {
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("blackbox-bastion " + version.Version)
		return
	}
	log.Printf("blackbox-bastion %s", version.Version)
	cfg := LoadConfig()
	secret, warning, err := resolveJWTSecret(cfg.JWTSecret, cfg.DevMode)
	if err != nil {
		log.Fatalf("jwt secret: %v", err)
	}
	if warning != "" {
		log.Printf("warning: %s", warning)
	}
	cfg.JWTSecret = secret
	ctx := context.Background()
	pool, err := OpenDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	log.Print("database connected")
	if err := RunMigrations(ctx, pool); err != nil {
		log.Fatalf("migrations: %v", err)
	}
	log.Print("migrations applied")
	hub := NewHub()
	totpCache := NewTotpSetupCache()
	srv := &Server{
		pool:            pool,
		cfg:             cfg,
		hub:             hub,
		totpCache:       totpCache,
		authLimiter:     NewRateLimiter(10, time.Minute),
		totpFailLimiter: NewRateLimiter(5, 15*time.Minute),
	}
	httpServer := &http.Server{Addr: cfg.ServerAddr, Handler: srv.routes()}
	go func() {
		var err error
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			log.Printf("server listening on %s (TLS)", cfg.ServerAddr)
			err = httpServer.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			log.Printf("server listening on %s", cfg.ServerAddr)
			err = httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Print("shutdown signal received, stopping server")
	if err := httpServer.Shutdown(context.Background()); err != nil {
		log.Printf("shutdown: %v", err)
	} else {
		log.Print("server stopped")
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
	return corsThenMux(s.cfg, mux)
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
	origin := cfg.CORSOrigin
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
