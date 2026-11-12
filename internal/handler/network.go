package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/service"
)

type networkRequest struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Tags        []model.Tag `json:"tags"`
}

func registerNetworkRoutes(mux *http.ServeMux, svc *service.NetworkService, authz *service.AuthorizationService, grants *service.GrantService) {
	mux.HandleFunc("GET /networks", listNetworksHandler(svc, authz))
	mux.HandleFunc("POST /networks", createNetworkHandler(svc, authz, grants))
	mux.HandleFunc("GET /networks/{uuid}", getNetworkHandler(svc, authz))
	mux.HandleFunc("PUT /networks/{uuid}", updateNetworkHandler(svc, authz))
	mux.HandleFunc("DELETE /networks/{uuid}", deleteNetworkHandler(svc, authz))
}

func listNetworksHandler(svc *service.NetworkService, authz *service.AuthorizationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		networks, err := svc.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list networks")
			return
		}

		if networks == nil {
			networks = []*model.Network{}
		}
		if authz != nil {
			ids, err := authz.VisibleNetworkUUIDs(r.Context(), principalFromContext(r.Context()), networks)
			if err != nil {
				writeAuthzError(w, err, "network not found")
				return
			}
			allowed := make(map[uuid.UUID]struct{}, len(ids))
			for _, id := range ids {
				allowed[id] = struct{}{}
			}
			filtered := make([]*model.Network, 0, len(ids))
			for _, network := range networks {
				if _, ok := allowed[network.UUID]; ok {
					filtered = append(filtered, network)
				}
			}
			networks = filtered
		}

		writeSuccess(w, http.StatusOK, networks)
	}
}

func createNetworkHandler(svc *service.NetworkService, authz *service.AuthorizationService, grants *service.GrantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if authz != nil {
			if err := authz.RequireCreateNetworks(r.Context(), principalFromContext(r.Context())); err != nil {
				writeAuthzError(w, err, "forbidden")
				return
			}
		}

		var req networkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		network, err := svc.Create(r.Context(), service.CreateNetworkInput{
			Name:        req.Name,
			Description: req.Description,
			Tags:        req.Tags,
		})
		if err != nil {
			writeNetworkError(w, err)
			return
		}
		if grants != nil {
			if err := grants.EnsureOwner(r.Context(), principalFromContext(r.Context()), network.UUID); err != nil {
				writeGrantError(w, err)
				return
			}
		}

		writeSuccess(w, http.StatusCreated, network)
	}
}

func getNetworkHandler(svc *service.NetworkService, authz *service.AuthorizationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid network uuid")
			return
		}
		if !requireNetworkAction(w, r, authz, svc, id, service.ActionRead) {
			return
		}

		network, err := svc.GetByUUID(r.Context(), id)
		if err != nil {
			writeNetworkError(w, err)
			return
		}

		writeSuccess(w, http.StatusOK, network)
	}
}

func updateNetworkHandler(svc *service.NetworkService, authz *service.AuthorizationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid network uuid")
			return
		}
		if !requireNetworkAction(w, r, authz, svc, id, service.ActionUpdate) {
			return
		}

		var req networkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		network, err := svc.Update(r.Context(), id, service.UpdateNetworkInput{
			Name:        req.Name,
			Description: req.Description,
			Tags:        req.Tags,
		})
		if err != nil {
			writeNetworkError(w, err)
			return
		}

		writeSuccess(w, http.StatusOK, network)
	}
}

func deleteNetworkHandler(svc *service.NetworkService, authz *service.AuthorizationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid network uuid")
			return
		}
		if !requireNetworkAction(w, r, authz, svc, id, service.ActionDelete) {
			return
		}

		if err := svc.Delete(r.Context(), id); err != nil {
			writeNetworkError(w, err)
			return
		}

		writeSuccess(w, http.StatusOK, nil)
	}
}

func requireNetworkAction(w http.ResponseWriter, r *http.Request, authz *service.AuthorizationService, svc *service.NetworkService, id uuid.UUID, action service.Action) bool {
	if authz == nil {
		return true
	}
	if _, err := svc.GetByUUID(r.Context(), id); err != nil {
		writeNetworkError(w, err)
		return false
	}
	if err := authz.Require(r.Context(), principalFromContext(r.Context()), action, model.GrantResourceNetwork, id); err != nil {
		writeAuthzError(w, err, "network not found")
		return false
	}
	return true
}

func parseUUID(raw string) (uuid.UUID, error) {
	return uuid.Parse(raw)
}

func writeNetworkError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrNetworkNotFound):
		writeError(w, http.StatusNotFound, errorMessage(err, "network not found"))
	case errors.Is(err, service.ErrNetworkHasChildren):
		writeError(w, http.StatusConflict, errorMessage(err, "network has child subnets and cannot be deleted"))
	case errors.Is(err, service.ErrNetworkNameTaken):
		writeError(w, http.StatusConflict, errorMessage(err, "network name already exists"))
	case errors.Is(err, service.ErrInvalidNetworkName):
		writeError(w, http.StatusBadRequest, errorMessage(err, "network name is required"))
	case errors.Is(err, service.ErrDuplicateTagKey):
		writeError(w, http.StatusBadRequest, errorMessage(err, "duplicate tag key"))
	default:
		if err.Error() == "tag key is required" {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
