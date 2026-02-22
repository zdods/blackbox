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
	pool *pgxpool.Pool
	cfg  Config
	hub  *Hub
}

func main() {
	cfg := LoadConfig()
	ctx := context.Background()
	pool, err := OpenDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	if err := RunMigrations(ctx, pool); err != nil {
		log.Fatalf("migrations: %v", err)
	}
	hub := NewHub()
	srv := &Server{pool: pool, cfg: cfg, hub: hub}
	mux := http.NewServeMux()
	// Auth (public)
	mux.HandleFunc("GET /api/setup", srv.Setup)
	mux.HandleFunc("POST /api/register", srv.Register)
	mux.HandleFunc("POST /api/login", srv.Login)
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
	httpServer := &http.Server{Addr: cfg.ServerAddr, Handler: corsThenMux(mux)}
	go func() {
		var err error
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			err = httpServer.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			err = httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		log.Printf("blackbox-server listening on %s (TLS)", cfg.ServerAddr)
	} else {
		log.Printf("blackbox-server listening on %s", cfg.ServerAddr)
	}
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	httpServer.Shutdown(context.Background())
}

func corsThenMux(mux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
