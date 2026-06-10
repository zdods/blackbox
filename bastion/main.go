package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	mux := http.NewServeMux()
	// Health (public; used by Docker HEALTHCHECK and uptime monitors)
	mux.HandleFunc("GET /healthz", srv.Healthz)
	// Auth (public)
	mux.HandleFunc("GET /api/setup", srv.Setup)
	mux.HandleFunc("POST /api/register", srv.Register)
	mux.HandleFunc("POST /api/register/totp-setup", srv.RegisterTOTPSetup)
	mux.HandleFunc("POST /api/login", srv.Login)
	mux.HandleFunc("POST /api/login/totp", srv.LoginTOTP)
	mux.HandleFunc("POST /api/logout", srv.AuthMiddleware(srv.Logout))
	// Protected (placeholder until step 5)
	mux.HandleFunc("GET /api/me", srv.AuthMiddleware(srv.Me))
	mux.HandleFunc("GET /api/daemons", srv.AuthMiddleware(srv.ListDaemons))
	mux.HandleFunc("POST /api/daemons", srv.AuthMiddleware(srv.CreateDaemon))
	mux.HandleFunc("PATCH /api/daemons/{id}", srv.AuthMiddleware(srv.UpdateDaemon))
	mux.HandleFunc("DELETE /api/daemons/{id}", srv.AuthMiddleware(srv.DeleteDaemon))
	mux.HandleFunc("GET /api/daemons/{id}/files", srv.AuthMiddleware(srv.DaemonFiles))
	mux.HandleFunc("PUT /api/daemons/{id}/files", srv.AuthMiddleware(srv.DaemonFiles))
	mux.HandleFunc("DELETE /api/daemons/{id}/files", srv.AuthMiddleware(srv.DaemonFiles))
	mux.HandleFunc("GET /api/daemons/{id}/meta", srv.AuthMiddleware(srv.DaemonMeta))
	// Daemon WebSocket (no session; daemon uses token)
	mux.HandleFunc("GET /ws/daemon", srv.HandleDaemonWS)
	// Static web app (SPA fallback to index.html); single pattern catches all GET requests not matched above
	mux.Handle("GET /{path...}", staticHandler(cfg.StaticDir))
	httpServer := &http.Server{Addr: cfg.ServerAddr, Handler: corsThenMux(cfg, mux)}
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
