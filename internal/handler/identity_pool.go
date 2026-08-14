package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kilhog-io/kilhog/internal/service"
)

func registerIdentityPoolRoutes(mux *http.ServeMux, pools *service.IdentityPoolService) {
	if pools == nil {
		return
	}
	mux.HandleFunc("GET /auth/identity-pools", requireAdmin(listIdentityPoolsHandler(pools)))
	mux.HandleFunc("POST /auth/identity-pools", requireAdmin(createIdentityPoolHandler(pools)))
	mux.HandleFunc("GET /auth/identity-pools/{uuid}", requireAdmin(getIdentityPoolHandler(pools)))
	mux.HandleFunc("PUT /auth/identity-pools/{uuid}", requireAdmin(updateIdentityPoolHandler(pools)))
	mux.HandleFunc("DELETE /auth/identity-pools/{uuid}", requireAdmin(deleteIdentityPoolHandler(pools)))
}

type createIdentityPoolRequest struct {
	Name         string   `json:"name"`
	Slug         string   `json:"slug"`
	Issuer       string   `json:"issuer"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	Scopes       []string `json:"scopes"`
	Enabled      *bool    `json:"enabled"`
}

type updateIdentityPoolRequest struct {
	Name         *string   `json:"name"`
	Slug         *string   `json:"slug"`
	Issuer       *string   `json:"issuer"`
	ClientID     *string   `json:"client_id"`
	ClientSecret *string   `json:"client_secret"`
	ClearSecret  bool      `json:"clear_secret"`
	Scopes       *[]string `json:"scopes"`
	Enabled      *bool     `json:"enabled"`
}

func listIdentityPoolsHandler(pools *service.IdentityPoolService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := pools.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list identity pools")
			return
		}
		writeSuccess(w, http.StatusOK, list)
	}
}

func createIdentityPoolHandler(pools *service.IdentityPoolService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createIdentityPoolRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		pool, err := pools.Create(r.Context(), service.CreateIdentityPoolInput{
			Name:         req.Name,
			Slug:         req.Slug,
			Issuer:       req.Issuer,
			ClientID:     req.ClientID,
			ClientSecret: req.ClientSecret,
			Scopes:       req.Scopes,
			Enabled:      req.Enabled,
		})
		if err != nil {
			writeIdentityPoolError(w, err)
			return
		}
		writeSuccess(w, http.StatusCreated, pool)
	}
}

func getIdentityPoolHandler(pools *service.IdentityPoolService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid identity pool uuid")
			return
		}
		pool, err := pools.GetByUUID(r.Context(), id)
		if err != nil {
			writeIdentityPoolError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, pool)
	}
}

func updateIdentityPoolHandler(pools *service.IdentityPoolService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid identity pool uuid")
			return
		}
		var req updateIdentityPoolRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		pool, err := pools.Update(r.Context(), id, service.UpdateIdentityPoolInput{
			Name:         req.Name,
			Slug:         req.Slug,
			Issuer:       req.Issuer,
			ClientID:     req.ClientID,
			ClientSecret: req.ClientSecret,
			ClearSecret:  req.ClearSecret,
			Scopes:       req.Scopes,
			Enabled:      req.Enabled,
		})
		if err != nil {
			writeIdentityPoolError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, pool)
	}
}

func deleteIdentityPoolHandler(pools *service.IdentityPoolService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid identity pool uuid")
			return
		}
		if err := pools.Delete(r.Context(), id); err != nil {
			writeIdentityPoolError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, nil)
	}
}

func writeIdentityPoolError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrIdentityPoolNotFound):
		writeError(w, http.StatusNotFound, errorMessage(err, "identity pool not found"))
	case errors.Is(err, service.ErrIdentityPoolNameTaken),
		errors.Is(err, service.ErrIdentityPoolSlugTaken),
		errors.Is(err, service.ErrIdentityPoolIssuerTaken):
		writeError(w, http.StatusConflict, errorMessage(err, "identity pool conflict"))
	case errors.Is(err, service.ErrInvalidIdentityPool):
		writeError(w, http.StatusBadRequest, errorMessage(err, "invalid identity pool"))
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
