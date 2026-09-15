package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/service"
)

func registerMachineIdentityRoutes(mux *http.ServeMux, machines *service.MachineIdentityService) {
	if machines == nil {
		return
	}
	mux.HandleFunc("GET /auth/machine-pools", requireAdmin(listMachinePoolsHandler(machines)))
	mux.HandleFunc("POST /auth/machine-pools", requireAdmin(createMachinePoolHandler(machines)))
	mux.HandleFunc("GET /auth/machine-pools/{uuid}", requireAdmin(getMachinePoolHandler(machines)))
	mux.HandleFunc("PUT /auth/machine-pools/{uuid}", requireAdmin(updateMachinePoolHandler(machines)))
	mux.HandleFunc("DELETE /auth/machine-pools/{uuid}", requireAdmin(deleteMachinePoolHandler(machines)))

	mux.HandleFunc("GET /auth/machine-pools/{uuid}/providers", requireAdmin(listMachineProvidersHandler(machines)))
	mux.HandleFunc("POST /auth/machine-pools/{uuid}/providers", requireAdmin(createMachineProviderHandler(machines)))
	mux.HandleFunc("GET /auth/machine-pools/{uuid}/providers/{provider_uuid}", requireAdmin(getMachineProviderHandler(machines)))
	mux.HandleFunc("PUT /auth/machine-pools/{uuid}/providers/{provider_uuid}", requireAdmin(updateMachineProviderHandler(machines)))
	mux.HandleFunc("DELETE /auth/machine-pools/{uuid}/providers/{provider_uuid}", requireAdmin(deleteMachineProviderHandler(machines)))
	mux.HandleFunc("POST /auth/machine-pools/{uuid}/providers/{provider_uuid}/refresh", requireAdmin(refreshMachineProviderHandler(machines)))

	mux.HandleFunc("GET /auth/machine-pools/{uuid}/machines", requireAdmin(listMachinesHandler(machines)))
	mux.HandleFunc("POST /auth/machine-pools/{uuid}/machines", requireAdmin(createMachineHandler(machines)))
	mux.HandleFunc("GET /auth/machine-pools/{uuid}/machines/{machine_uuid}", requireAdmin(getMachineHandler(machines)))
	mux.HandleFunc("PUT /auth/machine-pools/{uuid}/machines/{machine_uuid}", requireAdmin(updateMachineHandler(machines)))
	mux.HandleFunc("DELETE /auth/machine-pools/{uuid}/machines/{machine_uuid}", requireAdmin(deleteMachineHandler(machines)))

	mux.HandleFunc("GET /auth/machine-pools/{uuid}/machines/{machine_uuid}/api-keys", requireAdmin(listMachineAPIKeysHandler(machines)))
	mux.HandleFunc("POST /auth/machine-pools/{uuid}/machines/{machine_uuid}/api-keys", requireAdmin(createMachineAPIKeyHandler(machines)))
	mux.HandleFunc("DELETE /auth/machine-pools/{uuid}/machines/{machine_uuid}/api-keys/{key_uuid}", requireAdmin(revokeMachineAPIKeyHandler(machines)))
}

type createMachinePoolRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
}

type updateMachinePoolRequest struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Description *string `json:"description"`
	Enabled     *bool   `json:"enabled"`
}

type createMachineProviderRequest struct {
	Name      string          `json:"name"`
	Issuer    string          `json:"issuer"`
	Audiences []string        `json:"audiences"`
	JWKSMode  model.JWKSMode  `json:"jwks_mode"`
	JWKSURI   string          `json:"jwks_uri"`
	JWKS      json.RawMessage `json:"jwks"`
	Enabled   *bool           `json:"enabled"`
}

type updateMachineProviderRequest struct {
	Name      *string          `json:"name"`
	Issuer    *string          `json:"issuer"`
	Audiences *[]string        `json:"audiences"`
	JWKSMode  *model.JWKSMode  `json:"jwks_mode"`
	JWKSURI   *string          `json:"jwks_uri"`
	JWKS      *json.RawMessage `json:"jwks"`
	Enabled   *bool            `json:"enabled"`
}

type createMachineRequest struct {
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	Enabled       *bool                  `json:"enabled"`
	ProviderUUID  *uuid.UUID             `json:"provider_uuid"`
	Subject       string                 `json:"subject"`
	SubjectPrefix string                 `json:"subject_prefix"`
	Claims        []model.ClaimCondition `json:"claims"`
}

type updateMachineRequest struct {
	Name          *string                 `json:"name"`
	Description   *string                 `json:"description"`
	Enabled       *bool                   `json:"enabled"`
	ProviderUUID  *uuid.UUID              `json:"provider_uuid"`
	ClearProvider bool                    `json:"clear_provider"`
	Subject       *string                 `json:"subject"`
	SubjectPrefix *string                 `json:"subject_prefix"`
	Claims        *[]model.ClaimCondition `json:"claims"`
}

type createMachineAPIKeyRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func listMachinePoolsHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.ListPools(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list machine pools")
			return
		}
		writeSuccess(w, http.StatusOK, list)
	}
}

func createMachinePoolHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createMachinePoolRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		pool, err := svc.CreatePool(r.Context(), service.CreateMachinePoolInput{
			Name:        req.Name,
			Slug:        req.Slug,
			Description: req.Description,
			Enabled:     req.Enabled,
		})
		if err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusCreated, pool)
	}
}

func getMachinePoolHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid machine pool uuid")
			return
		}
		pool, err := svc.GetPool(r.Context(), id)
		if err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, pool)
	}
}

func updateMachinePoolHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid machine pool uuid")
			return
		}
		var req updateMachinePoolRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		pool, err := svc.UpdatePool(r.Context(), id, service.UpdateMachinePoolInput{
			Name:        req.Name,
			Slug:        req.Slug,
			Description: req.Description,
			Enabled:     req.Enabled,
		})
		if err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, pool)
	}
}

func deleteMachinePoolHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid machine pool uuid")
			return
		}
		if err := svc.DeletePool(r.Context(), id); err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, nil)
	}
}

func listMachineProvidersHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid machine pool uuid")
			return
		}
		list, err := svc.ListProviders(r.Context(), poolID)
		if err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, list)
	}
}

func createMachineProviderHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid machine pool uuid")
			return
		}
		var req createMachineProviderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		provider, err := svc.CreateProvider(r.Context(), poolID, service.CreateMachineProviderInput{
			Name:      req.Name,
			Issuer:    req.Issuer,
			Audiences: req.Audiences,
			JWKSMode:  req.JWKSMode,
			JWKSURI:   req.JWKSURI,
			JWKS:      req.JWKS,
			Enabled:   req.Enabled,
		})
		if err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusCreated, provider)
	}
}

func getMachineProviderHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, providerID, ok := parsePoolAndProvider(w, r)
		if !ok {
			return
		}
		provider, err := svc.GetProvider(r.Context(), poolID, providerID)
		if err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, provider)
	}
}

func updateMachineProviderHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, providerID, ok := parsePoolAndProvider(w, r)
		if !ok {
			return
		}
		var req updateMachineProviderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		provider, err := svc.UpdateProvider(r.Context(), poolID, providerID, service.UpdateMachineProviderInput{
			Name:      req.Name,
			Issuer:    req.Issuer,
			Audiences: req.Audiences,
			JWKSMode:  req.JWKSMode,
			JWKSURI:   req.JWKSURI,
			JWKS:      req.JWKS,
			Enabled:   req.Enabled,
		})
		if err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, provider)
	}
}

func deleteMachineProviderHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, providerID, ok := parsePoolAndProvider(w, r)
		if !ok {
			return
		}
		if err := svc.DeleteProvider(r.Context(), poolID, providerID); err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, nil)
	}
}

func refreshMachineProviderHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, providerID, ok := parsePoolAndProvider(w, r)
		if !ok {
			return
		}
		provider, err := svc.RefreshProviderJWKS(r.Context(), poolID, providerID)
		if err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, provider)
	}
}

func listMachinesHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid machine pool uuid")
			return
		}
		list, err := svc.ListMachines(r.Context(), poolID)
		if err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, list)
	}
}

func createMachineHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid machine pool uuid")
			return
		}
		var req createMachineRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		machine, err := svc.CreateMachine(r.Context(), poolID, service.CreateMachineInput{
			Name:          req.Name,
			Description:   req.Description,
			Enabled:       req.Enabled,
			ProviderUUID:  req.ProviderUUID,
			Subject:       req.Subject,
			SubjectPrefix: req.SubjectPrefix,
			Claims:        req.Claims,
		})
		if err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusCreated, machine)
	}
}

func getMachineHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, machineID, ok := parsePoolAndMachine(w, r)
		if !ok {
			return
		}
		machine, err := svc.GetMachine(r.Context(), poolID, machineID)
		if err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, machine)
	}
}

func updateMachineHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, machineID, ok := parsePoolAndMachine(w, r)
		if !ok {
			return
		}
		var req updateMachineRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		input := service.UpdateMachineInput{
			Name:          req.Name,
			Description:   req.Description,
			Enabled:       req.Enabled,
			ClearProvider: req.ClearProvider,
			Subject:       req.Subject,
			SubjectPrefix: req.SubjectPrefix,
			Claims:        req.Claims,
		}
		if req.ProviderUUID != nil {
			input.ProviderUUID = &req.ProviderUUID
		}
		machine, err := svc.UpdateMachine(r.Context(), poolID, machineID, input)
		if err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, machine)
	}
}

func deleteMachineHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, machineID, ok := parsePoolAndMachine(w, r)
		if !ok {
			return
		}
		if err := svc.DeleteMachine(r.Context(), poolID, machineID); err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, nil)
	}
}

func listMachineAPIKeysHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, machineID, ok := parsePoolAndMachine(w, r)
		if !ok {
			return
		}
		list, err := svc.ListAPIKeys(r.Context(), poolID, machineID)
		if err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, list)
	}
}

func createMachineAPIKeyHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, machineID, ok := parsePoolAndMachine(w, r)
		if !ok {
			return
		}
		var req createMachineAPIKeyRequest
		if r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid JSON body")
				return
			}
		}
		key, err := svc.CreateAPIKey(r.Context(), poolID, machineID, service.CreateMachineAPIKeyInput{
			Name:      req.Name,
			ExpiresAt: req.ExpiresAt,
		})
		if err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusCreated, key)
	}
}

func revokeMachineAPIKeyHandler(svc *service.MachineIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, machineID, ok := parsePoolAndMachine(w, r)
		if !ok {
			return
		}
		keyID, err := parseUUID(r.PathValue("key_uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid api key uuid")
			return
		}
		if err := svc.RevokeAPIKey(r.Context(), poolID, machineID, keyID); err != nil {
			writeMachineIdentityError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, nil)
	}
}

func parsePoolAndProvider(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	poolID, err := parseUUID(r.PathValue("uuid"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid machine pool uuid")
		return uuid.Nil, uuid.Nil, false
	}
	providerID, err := parseUUID(r.PathValue("provider_uuid"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid provider uuid")
		return uuid.Nil, uuid.Nil, false
	}
	return poolID, providerID, true
}

func parsePoolAndMachine(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	poolID, err := parseUUID(r.PathValue("uuid"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid machine pool uuid")
		return uuid.Nil, uuid.Nil, false
	}
	machineID, err := parseUUID(r.PathValue("machine_uuid"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid machine uuid")
		return uuid.Nil, uuid.Nil, false
	}
	return poolID, machineID, true
}

func writeMachineIdentityError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrMachinePoolNotFound),
		errors.Is(err, service.ErrMachineProviderNotFound),
		errors.Is(err, service.ErrMachineNotFound),
		errors.Is(err, service.ErrMachineAPIKeyNotFound):
		writeError(w, http.StatusNotFound, errorMessage(err, "not found"))
	case errors.Is(err, service.ErrMachinePoolNameTaken),
		errors.Is(err, service.ErrMachinePoolSlugTaken),
		errors.Is(err, service.ErrMachineProviderNameTaken),
		errors.Is(err, service.ErrMachineProviderIssuerTaken),
		errors.Is(err, service.ErrMachineNameTaken):
		writeError(w, http.StatusConflict, errorMessage(err, "conflict"))
	case errors.Is(err, service.ErrInvalidMachinePool),
		errors.Is(err, service.ErrInvalidMachineProvider),
		errors.Is(err, service.ErrInvalidMachine),
		errors.Is(err, service.ErrInvalidMachineAPIKey):
		writeError(w, http.StatusBadRequest, errorMessage(err, "invalid request"))
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
