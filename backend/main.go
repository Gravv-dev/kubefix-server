package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"kubefix-labs/backend/internal/auth"
	"kubefix-labs/backend/internal/challenge"
	"kubefix-labs/backend/internal/db"
	"kubefix-labs/backend/internal/docker"
	"kubefix-labs/backend/internal/middleware"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Database pool.
	pool, err := db.NewPool(ctx)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	store := db.NewStore(pool.Pool)

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// Docker client (best-effort; challenges can still be browsed without it).
	dockerClient, dockerErr := docker.NewClient()
	if dockerErr != nil {
		log.Printf("warning: docker client unavailable (%v); challenge environments will fail to allocate", dockerErr)
	}

	// Services.
	authSvc := auth.NewService(store)
	chalSvc := challenge.NewService(store, dockerClient)

	// Router.
	mux := http.NewServeMux()

	// Public auth routes.
	mux.HandleFunc("POST /api/auth/google", authSvc.HandleGoogleLogin)
	mux.HandleFunc("POST /api/auth/logout", middleware.RequireAuth(authSvc, authSvc.HandleLogout))
	mux.HandleFunc("GET /api/auth/me", middleware.RequireAuth(authSvc, authSvc.HandleMe))

	// Challenge catalog (public read).
	mux.HandleFunc("GET /api/challenges", chalSvc.HandleListChallenges)
	mux.HandleFunc("GET /api/challenges/{id}", chalSvc.HandleGetChallenge)

	// Authenticated challenge routes.
	mux.HandleFunc("POST /api/challenges/{id}/start", middleware.RequireAuth(authSvc, chalSvc.HandleStartChallenge))
	mux.HandleFunc("POST /api/challenges/{id}/verify", middleware.RequireAuth(authSvc, chalSvc.HandleVerifyChallenge))
	mux.HandleFunc("GET /api/me/attempts", middleware.RequireAuth(authSvc, chalSvc.HandleMyAttempts))

	// Terminal proxy (best-effort pty to challenge environment).
	mux.HandleFunc("GET /api/terminal/{environment_id}", middleware.RequireAuth(authSvc, chalSvc.HandleTerminal))

	// Admin routes (challenge management). RequireAuth first; admin role enforced inside handlers.
	mux.HandleFunc("POST /api/admin/challenges", middleware.RequireAuth(authSvc, requireAdmin(authSvc, chalSvc.HandleCreateChallenge)))
	mux.HandleFunc("GET /api/admin/challenges", middleware.RequireAuth(authSvc, requireAdmin(authSvc, chalSvc.HandleListAdminChallenges)))
	mux.HandleFunc("GET /api/admin/challenges/{id}", middleware.RequireAuth(authSvc, requireAdmin(authSvc, chalSvc.HandleGetAdminChallenge)))
	mux.HandleFunc("PUT /api/admin/challenges/{id}", middleware.RequireAuth(authSvc, requireAdmin(authSvc, chalSvc.HandleUpdateChallenge)))
	mux.HandleFunc("PATCH /api/admin/challenges/{id}/publish", middleware.RequireAuth(authSvc, requireAdmin(authSvc, chalSvc.HandlePublishChallenge)))
	mux.HandleFunc("DELETE /api/admin/challenges/{id}", middleware.RequireAuth(authSvc, requireAdmin(authSvc, chalSvc.HandleDeleteChallenge)))

	// Health.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:         addr,
		Handler:      middleware.CORS(middleware.RequestID(mux)),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("kubefix-labs backend listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}

// requireAdmin wraps a handler and rejects non-admin users.
func requireAdmin(svc *auth.Service, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(auth.UserIDCtxKey).(uuid.UUID)
		if !ok {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		user, err := svc.Store().GetUserByID(r.Context(), userID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if !auth.IsAdmin(user.Role) {
			http.Error(w, "admin access required", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}
