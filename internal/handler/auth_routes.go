package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kilhog-io/kilhog/internal/service"
)

func registerAuthRoutes(mux *http.ServeMux, auth *service.AuthService, pools *service.IdentityPoolService) {
	if auth == nil {
		return
	}

	mux.HandleFunc("GET /auth/status", authStatusHandler(auth))
	mux.HandleFunc("POST /auth/bootstrap", authBootstrapHandler(auth))
	mux.HandleFunc("POST /auth/login", authLoginHandler(auth))
	mux.HandleFunc("POST /auth/logout", authLogoutHandler(auth))
	mux.HandleFunc("GET /auth/me", authMeHandler(auth))
	mux.HandleFunc("GET /auth/oidc/pools", authListPublicOIDCPoolsHandler(pools))
	mux.HandleFunc("GET /auth/oidc/{slug}/login", authOIDCLoginHandler(auth))
	mux.HandleFunc("GET /auth/oidc/callback", authOIDCCallbackHandler(auth))
}

func authStatusHandler(auth *service.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := auth.Status(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read auth status")
			return
		}
		writeSuccess(w, http.StatusOK, status)
	}
}

type bootstrapRequest struct {
	Username       string `json:"username"`
	Password       string `json:"password"`
	DisplayName    string `json:"display_name"`
	Email          string `json:"email"`
	BootstrapToken string `json:"bootstrap_token"`
}

func authBootstrapHandler(auth *service.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req bootstrapRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		user, token, err := auth.Bootstrap(r.Context(), service.BootstrapInput{
			Username:       req.Username,
			Password:       req.Password,
			DisplayName:    req.DisplayName,
			Email:          req.Email,
			BootstrapToken: firstNonEmpty(req.BootstrapToken, r.Header.Get("X-Bootstrap-Token")),
		})
		if err != nil {
			writeAuthServiceError(w, err)
			return
		}

		setSessionCookie(w, token)
		writeSuccess(w, http.StatusCreated, map[string]any{
			"user":    user,
			"session": token,
		})
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func authLoginHandler(auth *service.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		user, token, err := auth.LoginLocal(r.Context(), service.LocalLoginInput{
			Username: req.Username,
			Password: req.Password,
		})
		if err != nil {
			writeAuthServiceError(w, err)
			return
		}

		setSessionCookie(w, token)
		writeSuccess(w, http.StatusOK, map[string]any{
			"user":    user,
			"session": token,
		})
	}
}

func authLogoutHandler(auth *service.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = auth.Logout(r.Context(), extractSessionToken(r))
		clearSessionCookie(w)
		writeSuccess(w, http.StatusOK, nil)
	}
}

func authMeHandler(auth *service.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKeyHeader := r.Header.Get("X-API-Key")
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
		writeSuccess(w, http.StatusOK, principal)
	}
}

func authListPublicOIDCPoolsHandler(pools *service.IdentityPoolService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if pools == nil {
			writeSuccess(w, http.StatusOK, []any{})
			return
		}
		list, err := pools.ListEnabledPublic(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list identity pools")
			return
		}
		writeSuccess(w, http.StatusOK, list)
	}
}

func authOIDCLoginHandler(auth *service.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start, err := auth.StartOIDCLogin(r.Context(), r.PathValue("slug"))
		if err != nil {
			writeAuthServiceError(w, err)
			return
		}
		if r.Header.Get("Accept") == "application/json" {
			writeSuccess(w, http.StatusOK, start)
			return
		}
		http.Redirect(w, r, start.AuthURL, http.StatusFound)
	}
}

func authOIDCCallbackHandler(auth *service.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			writeError(w, http.StatusBadRequest, "OIDC provider error: "+errMsg)
			return
		}

		token, principal, err := auth.CompleteOIDCLogin(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"))
		if err != nil {
			writeAuthServiceError(w, err)
			return
		}

		setSessionCookie(w, token)
		writeSuccess(w, http.StatusOK, map[string]any{
			"principal": principal,
			"session":   token,
		})
	}
}

func writeAuthServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrBootstrapUnavailable):
		writeError(w, http.StatusConflict, errorMessage(err, "bootstrap unavailable"))
	case errors.Is(err, service.ErrBootstrapForbidden):
		writeError(w, http.StatusForbidden, errorMessage(err, "invalid bootstrap token"))
	case errors.Is(err, service.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, errorMessage(err, "invalid username or password"))
	case errors.Is(err, service.ErrInvalidUsername),
		errors.Is(err, service.ErrInvalidPassword),
		errors.Is(err, service.ErrInvalidUserRole),
		errors.Is(err, service.ErrInvalidEmail),
		errors.Is(err, service.ErrInvalidIdentityPool):
		writeError(w, http.StatusBadRequest, errorMessage(err, "invalid request"))
	case errors.Is(err, service.ErrUsernameTaken):
		writeError(w, http.StatusConflict, errorMessage(err, "username already exists"))
	case errors.Is(err, service.ErrIdentityPoolNotFound):
		writeError(w, http.StatusNotFound, errorMessage(err, "identity pool not found"))
	case errors.Is(err, service.ErrOIDCNotConfigured),
		errors.Is(err, service.ErrOIDCLoginStateExpired),
		errors.Is(err, service.ErrOIDCLoginStateNotFound),
		errors.Is(err, service.ErrUnauthenticated):
		writeError(w, http.StatusUnauthorized, errorMessage(err, "authentication failed"))
	case errors.Is(err, service.ErrForbidden):
		writeError(w, http.StatusForbidden, errorMessage(err, "forbidden"))
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
