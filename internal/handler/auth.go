package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/service"
)

type principalContextKey struct{}

func withPrincipal(ctx context.Context, principal *service.Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func principalFromContext(ctx context.Context) *service.Principal {
	principal, _ := ctx.Value(principalContextKey{}).(*service.Principal)
	return principal
}

func authMiddleware(auth *service.AuthService, apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth == nil {
			apiKeyOnlyMiddleware(apiKey, next).ServeHTTP(w, r)
			return
		}

		apiKeyHeader := strings.TrimSpace(r.Header.Get("X-API-Key"))
		bearer := extractBearerToken(r)
		cookieToken := ""
		if c, err := r.Cookie(service.SessionCookieName()); err == nil {
			cookieToken = c.Value
		}

		principal, err := auth.AuthenticateRequest(r.Context(), apiKeyHeader, bearer, cookieToken)
		if err != nil {
			writeAuthError(w, err)
			return
		}

		next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), principal)))
	})
}

func apiKeyOnlyMiddleware(expectedKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if expectedKey == "" {
			writeError(w, http.StatusForbidden, "authentication is not configured")
			return
		}

		provided := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if provided == "" {
			provided = extractBearerToken(r)
		}
		if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(expectedKey)) != 1 {
			writeError(w, http.StatusUnauthorized, "missing or invalid API key")
			return
		}

		principal := &service.Principal{Kind: model.PrincipalKindAPIKey}
		next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), principal)))
	})
}

func adminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := service.RequireAdmin(principalFromContext(r.Context())); err != nil {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := service.RequireAdmin(principalFromContext(r.Context())); err != nil {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		next(w, r)
	}
}

func extractBearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return strings.TrimSpace(auth[len(prefix):])
	}
	return ""
}

func extractSessionToken(r *http.Request) string {
	if bearer := extractBearerToken(r); bearer != "" {
		return bearer
	}
	if c, err := r.Cookie(service.SessionCookieName()); err == nil {
		return c.Value
	}
	return ""
}

func setSessionCookie(w http.ResponseWriter, token *service.SessionToken) {
	http.SetCookie(w, &http.Cookie{
		Name:     service.SessionCookieName(),
		Value:    token.Token,
		Path:     "/",
		Expires:  token.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     service.SessionCookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrAuthNotConfigured):
		writeError(w, http.StatusForbidden, "authentication is not configured")
	case errors.Is(err, service.ErrUnauthenticated),
		errors.Is(err, service.ErrSessionNotFound),
		errors.Is(err, service.ErrSessionExpired):
		writeError(w, http.StatusUnauthorized, "missing or invalid credentials")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
