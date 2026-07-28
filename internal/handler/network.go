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

func registerNetworkRoutes(mux *http.ServeMux, svc *service.NetworkService) {
	mux.HandleFunc("GET /networks", listNetworksHandler(svc))
	mux.HandleFunc("POST /networks", createNetworkHandler(svc))
	mux.HandleFunc("GET /networks/{uuid}", getNetworkHandler(svc))
	mux.HandleFunc("PUT /networks/{uuid}", updateNetworkHandler(svc))
	mux.HandleFunc("DELETE /networks/{uuid}", deleteNetworkHandler(svc))
}

func listNetworksHandler(svc *service.NetworkService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		networks, err := svc.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list networks")
			return
		}

		if networks == nil {
			networks = []*model.Network{}
		}

		writeSuccess(w, http.StatusOK, networks)
	}
}

func createNetworkHandler(svc *service.NetworkService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		writeSuccess(w, http.StatusCreated, network)
	}
}

func getNetworkHandler(svc *service.NetworkService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid network uuid")
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

func updateNetworkHandler(svc *service.NetworkService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid network uuid")
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

func deleteNetworkHandler(svc *service.NetworkService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid network uuid")
			return
		}

		if err := svc.Delete(r.Context(), id); err != nil {
			writeNetworkError(w, err)
			return
		}

		writeSuccess(w, http.StatusOK, nil)
	}
}

func parseUUID(raw string) (uuid.UUID, error) {
	return uuid.Parse(raw)
}

func writeNetworkError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrNetworkNotFound):
		writeError(w, http.StatusNotFound, "network not found")
	case errors.Is(err, service.ErrNetworkHasChildren):
		writeError(w, http.StatusConflict, "network has child subnets and cannot be deleted")
	case errors.Is(err, service.ErrNetworkNameTaken):
		writeError(w, http.StatusConflict, "network name already exists")
	case errors.Is(err, service.ErrInvalidNetworkName):
		writeError(w, http.StatusBadRequest, "network name is required")
	case errors.Is(err, service.ErrDuplicateTagKey):
		writeError(w, http.StatusBadRequest, "duplicate tag key")
	default:
		if err.Error() == "tag key is required" {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
