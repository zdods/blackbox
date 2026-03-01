package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	pool      *pgxpool.Pool
	cfg       Config
	hub       *Hub
	totpCache *TotpSetupCache
}

func main() {
	cfg := LoadConfig()
	if cfg.JWTSecret == "poc-secret-change-in-production" {
		log.Printf("warning: JWT_SECRET is default; set JWT_SECRET in production")
	}
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
	srv := &Server{pool: pool, cfg: cfg, hub: hub, totpCache: totpCache}
	mux := http.NewServeMux()
	// Auth (public)
	mux.HandleFunc("GET /api/setup", srv.Setup)
	mux.HandleFunc("POST /api/register", srv.Register)
	mux.HandleFunc("POST /api/register/totp-setup", srv.RegisterTOTPSetup)
	mux.HandleFunc("POST /api/login", srv.Login)
	mux.HandleFunc("POST /api/login/totp", srv.LoginTOTP)
	// Protected (placeholder until step 5)
	mux.HandleFunc("GET /api/me", srv.AuthMiddleware(srv.Me))
	mux.HandleFunc("GET /api/agents", srv.AuthMiddleware(srv.ListAgents))
	mux.HandleFunc("POST /api/agents", srv.AuthMiddleware(srv.CreateAgent))
	mux.HandleFunc("PATCH /api/agents/{id}", srv.AuthMiddleware(srv.UpdateAgent))
	mux.HandleFunc("DELETE /api/agents/{id}", srv.AuthMiddleware(srv.DeleteAgent))
	mux.HandleFunc("GET /api/agents/{id}/files", srv.AuthMiddleware(srv.AgentFiles))
	mux.HandleFunc("PUT /api/agents/{id}/files", srv.AuthMiddleware(srv.AgentFiles))
	mux.HandleFunc("DELETE /api/agents/{id}/files", srv.AuthMiddleware(srv.AgentFiles))
	mux.HandleFunc("GET /api/agents/{id}/meta", srv.AuthMiddleware(srv.AgentMeta))
	// Agent WebSocket (no session; agent uses token)
	mux.HandleFunc("GET /ws/agent", srv.HandleAgentWS)
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

func corsThenMux(cfg Config, mux http.Handler) http.Handler {
	origin := cfg.CORSOrigin
	if origin == "" {
		origin = "*"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
