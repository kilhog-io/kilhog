package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/service"
)

type subnetRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Prefix      int               `json:"prefix"`
	Address     string            `json:"address"`
	Type        model.AddressType `json:"type"`
}

type updateSubnetRequest struct {
	Description string `json:"description"`
}

func registerSubnetRoutes(mux *http.ServeMux, svc *service.SubnetService) {
	mux.HandleFunc("GET /networks/{uuid}/subnets", listNetworkSubnetsHandler(svc))
	mux.HandleFunc("POST /networks/{uuid}/subnets", createNetworkSubnetHandler(svc))
	mux.HandleFunc("GET /networks/{uuid}/subnets/{subnet_uuid}", getNetworkSubnetHandler(svc))
	mux.HandleFunc("PUT /networks/{uuid}/subnets/{subnet_uuid}", updateNetworkSubnetHandler(svc))
	mux.HandleFunc("DELETE /networks/{uuid}/subnets/{subnet_uuid}", deleteNetworkSubnetHandler(svc))
	mux.HandleFunc("GET /networks/{uuid}/subnets/{subnet_uuid}/subnets", listChildSubnetsHandler(svc))
	mux.HandleFunc("POST /networks/{uuid}/subnets/{subnet_uuid}/subnets", createChildSubnetHandler(svc))
}

func listNetworkSubnetsHandler(svc *service.SubnetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		networkUUID, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid network uuid")
			return
		}

		subnets, err := svc.ListByNetwork(r.Context(), networkUUID)
		if err != nil {
			writeSubnetError(w, err)
			return
		}
		if subnets == nil {
			subnets = []*model.Subnet{}
		}

		writeSuccess(w, http.StatusOK, subnets)
	}
}

func createNetworkSubnetHandler(svc *service.SubnetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		networkUUID, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid network uuid")
			return
		}

		var req subnetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		subnet, err := svc.CreateInNetwork(r.Context(), networkUUID, model.Parent{
			Kind: model.ParentKindNetwork,
			UUID: networkUUID,
		}, service.CreateSubnetInput{
			Name:        req.Name,
			Description: req.Description,
			Prefix:      req.Prefix,
			Address:     req.Address,
			Type:        req.Type,
		})
		if err != nil {
			writeSubnetError(w, err)
			return
		}

		writeSuccess(w, http.StatusCreated, subnet)
	}
}

func createChildSubnetHandler(svc *service.SubnetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		networkUUID, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid network uuid")
			return
		}

		parentSubnetUUID, err := parseUUID(r.PathValue("subnet_uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid subnet uuid")
			return
		}

		var req subnetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		subnet, err := svc.CreateInNetwork(r.Context(), networkUUID, model.Parent{
			Kind: model.ParentKindSubnet,
			UUID: parentSubnetUUID,
		}, service.CreateSubnetInput{
			Name:        req.Name,
			Description: req.Description,
			Prefix:      req.Prefix,
			Address:     req.Address,
			Type:        req.Type,
		})
		if err != nil {
			writeSubnetError(w, err)
			return
		}

		writeSuccess(w, http.StatusCreated, subnet)
	}
}

func getNetworkSubnetHandler(svc *service.SubnetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		networkUUID, subnetUUID, ok := parseNetworkSubnetPath(w, r)
		if !ok {
			return
		}

		subnet, err := svc.GetInNetwork(r.Context(), networkUUID, subnetUUID)
		if err != nil {
			writeSubnetError(w, err)
			return
		}

		writeSuccess(w, http.StatusOK, subnet)
	}
}

func updateNetworkSubnetHandler(svc *service.SubnetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		networkUUID, subnetUUID, ok := parseNetworkSubnetPath(w, r)
		if !ok {
			return
		}

		var req updateSubnetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		subnet, err := svc.UpdateInNetwork(r.Context(), networkUUID, subnetUUID, service.UpdateSubnetInput{
			Description: req.Description,
		})
		if err != nil {
			writeSubnetError(w, err)
			return
		}

		writeSuccess(w, http.StatusOK, subnet)
	}
}

func deleteNetworkSubnetHandler(svc *service.SubnetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		networkUUID, subnetUUID, ok := parseNetworkSubnetPath(w, r)
		if !ok {
			return
		}

		if err := svc.DeleteInNetwork(r.Context(), networkUUID, subnetUUID); err != nil {
			writeSubnetError(w, err)
			return
		}

		writeSuccess(w, http.StatusOK, nil)
	}
}

func listChildSubnetsHandler(svc *service.SubnetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		networkUUID, err := parseUUID(r.PathValue("uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid network uuid")
			return
		}

		parentSubnetUUID, err := parseUUID(r.PathValue("subnet_uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid subnet uuid")
			return
		}

		subnets, err := svc.ListChildren(r.Context(), networkUUID, parentSubnetUUID)
		if err != nil {
			writeSubnetError(w, err)
			return
		}
		if subnets == nil {
			subnets = []*model.Subnet{}
		}

		writeSuccess(w, http.StatusOK, subnets)
	}
}

func parseNetworkSubnetPath(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	networkUUID, err := parseUUID(r.PathValue("uuid"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid network uuid")
		return uuid.Nil, uuid.Nil, false
	}

	subnetUUID, err := parseUUID(r.PathValue("subnet_uuid"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid subnet uuid")
		return uuid.Nil, uuid.Nil, false
	}

	return networkUUID, subnetUUID, true
}

func writeSubnetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrNetworkNotFound):
		writeError(w, http.StatusNotFound, "network not found")
	case errors.Is(err, service.ErrSubnetNotFound):
		writeError(w, http.StatusNotFound, "subnet not found")
	case errors.Is(err, service.ErrSubnetNotInNetwork):
		writeError(w, http.StatusNotFound, "subnet not found in network")
	case errors.Is(err, service.ErrSubnetNameTaken):
		writeError(w, http.StatusConflict, "subnet name already exists in network")
	case errors.Is(err, service.ErrSubnetHasChildren):
		writeError(w, http.StatusConflict, "subnet has child subnets and cannot be deleted")
	case errors.Is(err, service.ErrInvalidSubnetName):
		writeError(w, http.StatusBadRequest, "subnet name is required")
	case errors.Is(err, service.ErrInvalidSubnetPrefix):
		writeError(w, http.StatusBadRequest, "invalid subnet prefix")
	case errors.Is(err, service.ErrInvalidSubnetAddress):
		writeError(w, http.StatusBadRequest, "invalid subnet address")
	case errors.Is(err, service.ErrAddressRequired):
		writeError(w, http.StatusBadRequest, "address is required when parent is a network")
	case errors.Is(err, service.ErrInvalidSubnetParent):
		writeError(w, http.StatusBadRequest, "subnet parent is required")
	case errors.Is(err, service.ErrParentNotFound):
		writeError(w, http.StatusBadRequest, "subnet parent not found")
	case errors.Is(err, service.ErrSubnetOverlap):
		writeError(w, http.StatusConflict, "subnet overlaps with sibling")
	case errors.Is(err, service.ErrAddressOutsideParent):
		writeError(w, http.StatusBadRequest, "subnet is outside parent CIDR")
	case errors.Is(err, service.ErrPrefixTooBroad):
		writeError(w, http.StatusBadRequest, "subnet prefix is less specific than parent")
	case errors.Is(err, service.ErrNoFreeAddress):
		writeError(w, http.StatusConflict, "no free address block found in parent")
	case errors.Is(err, service.ErrIPv6NotSupported):
		writeError(w, http.StatusBadRequest, "only ipv4 is supported")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
