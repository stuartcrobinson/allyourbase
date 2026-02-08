package auth

import (
	"context"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
)

type ctxKey struct{}

// RequireAuth returns middleware that rejects requests without a valid JWT.
func RequireAuth(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := extractBearerToken(r)
			if !ok {
				httputil.WriteError(w, http.StatusUnauthorized, "missing or invalid authorization header")
				return
			}

			claims, err := svc.ValidateToken(token)
			if err != nil {
				httputil.WriteError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), ctxKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth returns middleware that extracts JWT claims if present
// but does not reject unauthenticated requests.
func OptionalAuth(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token, ok := extractBearerToken(r); ok {
				if claims, err := svc.ValidateToken(token); err == nil {
					ctx := context.WithValue(r.Context(), ctxKey{}, claims)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClaimsFromContext retrieves auth claims from the request context.
// Returns nil if no claims are present.
func ClaimsFromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(ctxKey{}).(*Claims)
	return claims
}

func extractBearerToken(r *http.Request) (string, bool) {
	return httputil.ExtractBearerToken(r)
}
