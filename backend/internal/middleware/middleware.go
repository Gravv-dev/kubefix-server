package middleware

import (
	"net/http"
	"os"

	"kubefix-labs/backend/internal/auth"
)

// RequireAuth wraps a handler so only authenticated users (valid session cookie) can access it.
// The user id is injected into the request context.
func RequireAuth(svc *auth.Service, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := svc.ExtractSession(r)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		ctx := auth.WithUserID(r.Context(), userID)
		next(w, r.WithContext(ctx))
	}
}

// RequireAdmin is a pass-through middleware; admin enforcement is performed inside
// the admin handlers using the store (keeps middleware free of DB coupling).
func RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return next
}

// CORS adds cross-origin headers.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := getEnv("CORS_ALLOW_ORIGIN", "*")
		if allowed == "*" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequestID is a placeholder for future request-id tracing.
func RequestID(next http.Handler) http.Handler {
	return next
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
