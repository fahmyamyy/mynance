package auth

import (
	"net/http"
	"strings"

	"mynance/internal/shared"
)

func Middleware(signer *Signer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if h == "" || !strings.HasPrefix(h, "Bearer ") {
				shared.HTTPError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
				return
			}
			token := strings.TrimPrefix(h, "Bearer ")
			claims, err := signer.Verify(token)
			if err != nil {
				shared.HTTPError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}
			ctx := WithUser(r.Context(), claims.UserID, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsAdmin(r.Context()) {
			shared.HTTPError(w, http.StatusForbidden, "admin role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
