package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/service"
)

func registerGrantRoutes(mux *http.ServeMux, grants *service.GrantService, authz *service.AuthorizationService) {
	if grants == nil {
		return
	}

	mux.HandleFunc("GET /auth/me/grants", listMyGrantsHandler(grants))
	mux.HandleFunc("GET /auth/platform-grants", requireAdmin(listPlatformGrantsHandler(grants)))
	mux.HandleFunc("POST /auth/platform-grants", requireAdmin(createPlatformGrantHandler(grants)))
	mux.HandleFunc("GET /auth/platform-grants/{grant_uuid}", requireAdmin(getPlatformGrantHandler(grants)))
	mux.HandleFunc("PUT /auth/platform-grants/{grant_uuid}", requireAdmin(updatePlatformGrantHandler(grants)))
	mux.HandleFunc("DELETE /auth/platform-grants/{grant_uuid}", requireAdmin(deletePlatformGrantHandler(grants)))

	mux.HandleFunc("GET /networks/{uuid}/grants", listNetworkGrantsHandler(grants, authz))
	mux.HandleFunc("POST /networks/{uuid}/grants", createNetworkGrantHandler(grants, authz))
	mux.HandleFunc("GET /networks/{uuid}/grants/{grant_uuid}", getNetworkGrantHandler(grants, authz))
	mux.HandleFunc("PUT /networks/{uuid}/grants/{grant_uuid}", updateNetworkGrantHandler(grants, authz))
	mux.HandleFunc("DELETE /networks/{uuid}/grants/{grant_uuid}", deleteNetworkGrantHandler(grants, authz))
	mux.HandleFunc("POST /networks/{uuid}/ownership/transfer", transferNetworkOwnershipHandler(grants, authz))

	mux.HandleFunc("GET /networks/{uuid}/subnets/{subnet_uuid}/grants", listSubnetGrantsHandler(grants, authz))
	mux.HandleFunc("POST /networks/{uuid}/subnets/{subnet_uuid}/grants", createSubnetGrantHandler(grants, authz))
	mux.HandleFunc("GET /networks/{uuid}/subnets/{subnet_uuid}/grants/{grant_uuid}", getSubnetGrantHandler(grants, authz))
	mux.HandleFunc("PUT /networks/{uuid}/subnets/{subnet_uuid}/grants/{grant_uuid}", updateSubnetGrantHandler(grants, authz))
	mux.HandleFunc("DELETE /networks/{uuid}/subnets/{subnet_uuid}/grants/{grant_uuid}", deleteSubnetGrantHandler(grants, authz))
	mux.HandleFunc("POST /networks/{uuid}/subnets/{subnet_uuid}/ownership/transfer", transferSubnetOwnershipHandler(grants, authz))
}

type grantPrincipalRequest struct {
	Kind             model.GrantPrincipalKind `json:"kind"`
	LocalUserUUID    *uuid.UUID               `json:"local_user_uuid"`
	IdentityPoolUUID *uuid.UUID               `json:"identity_pool_uuid"`
	Subject          string                   `json:"subject"`
	Group            string                   `json:"group"`
	MachineUUID      *uuid.UUID               `json:"machine_uuid"`
	MachinePoolUUID  *uuid.UUID               `json:"machine_pool_uuid"`
}

type createResourceGrantRequest struct {
	Principal   grantPrincipalRequest `json:"principal"`
	Permissions model.Permissions     `json:"permissions"`
	Owner       bool                  `json:"owner"`
}

type updateGrantRequest struct {
	Permissions model.Permissions `json:"permissions"`
	Owner       bool              `json:"owner"`
}

type createPlatformGrantRequest struct {
	Principal  grantPrincipalRequest `json:"principal"`
	Capability string                `json:"capability"`
}

type transferOwnershipRequest struct {
	From *grantPrincipalRequest `json:"from"`
	To   grantPrincipalRequest  `json:"to"`
}

func (p grantPrincipalRequest) toModel() model.GrantPrincipal {
	return model.GrantPrincipal{
		Kind:             p.Kind,
		LocalUserUUID:    p.LocalUserUUID,
		IdentityPoolUUID: p.IdentityPoolUUID,
		Subject:          p.Subject,
		Group:            p.Group,
		MachineUUID:      p.MachineUUID,
		MachinePoolUUID:  p.MachinePoolUUID,
	}
}

func listMyGrantsHandler(grants *service.GrantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := grants.ListEffectiveForPrincipal(r.Context(), principalFromContext(r.Context()))
		if err != nil {
			writeGrantError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, list)
	}
}

func listPlatformGrantsHandler(grants *service.GrantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := grants.ListPlatform(r.Context())
		if err != nil {
			writeGrantError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, list)
	}
}

func createPlatformGrantHandler(grants *service.GrantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createPlatformGrantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		capability := req.Capability
		if capability == "" {
			capability = model.CapabilityCreateNetworks
		}
		grant, err := grants.Create(r.Context(), principalFromContext(r.Context()), service.CreateGrantInput{
			Principal:   req.Principal.toModel(),
			Resource:    model.GrantResource{Kind: model.GrantResourcePlatform, Capability: capability},
			Permissions: model.Permissions{Create: true},
		})
		if err != nil {
			writeGrantError(w, err)
			return
		}
		writeSuccess(w, http.StatusCreated, grant)
	}
}

func getPlatformGrantHandler(grants *service.GrantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("grant_uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid grant uuid")
			return
		}
		grant, err := grants.GetByUUID(r.Context(), id)
		if err != nil {
			writeGrantError(w, err)
			return
		}
		if grant.Resource.Kind != model.GrantResourcePlatform {
			writeError(w, http.StatusNotFound, "grant not found")
			return
		}
		writeSuccess(w, http.StatusOK, grant)
	}
}

func updatePlatformGrantHandler(grants *service.GrantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("grant_uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid grant uuid")
			return
		}
		grant, err := grants.GetByUUID(r.Context(), id)
		if err != nil {
			writeGrantError(w, err)
			return
		}
		if grant.Resource.Kind != model.GrantResourcePlatform {
			writeError(w, http.StatusNotFound, "grant not found")
			return
		}
		updated, err := grants.Update(r.Context(), principalFromContext(r.Context()), id, service.UpdateGrantInput{
			Permissions: model.Permissions{Create: true},
		})
		if err != nil {
			writeGrantError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, updated)
	}
}

func deletePlatformGrantHandler(grants *service.GrantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r.PathValue("grant_uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid grant uuid")
			return
		}
		grant, err := grants.GetByUUID(r.Context(), id)
		if err != nil {
			writeGrantError(w, err)
			return
		}
		if grant.Resource.Kind != model.GrantResourcePlatform {
			writeError(w, http.StatusNotFound, "grant not found")
			return
		}
		if err := grants.Delete(r.Context(), principalFromContext(r.Context()), id); err != nil {
			writeGrantError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, nil)
	}
}

func listNetworkGrantsHandler(grants *service.GrantService, authz *service.AuthorizationService) http.HandlerFunc {
	return resourceGrantListHandler(grants, authz, model.GrantResourceNetwork, false)
}

func createNetworkGrantHandler(grants *service.GrantService, authz *service.AuthorizationService) http.HandlerFunc {
	return resourceGrantCreateHandler(grants, authz, model.GrantResourceNetwork, false)
}

func getNetworkGrantHandler(grants *service.GrantService, authz *service.AuthorizationService) http.HandlerFunc {
	return resourceGrantGetHandler(grants, authz, model.GrantResourceNetwork, false)
}

func updateNetworkGrantHandler(grants *service.GrantService, authz *service.AuthorizationService) http.HandlerFunc {
	return resourceGrantUpdateHandler(grants, authz, model.GrantResourceNetwork, false)
}

func deleteNetworkGrantHandler(grants *service.GrantService, authz *service.AuthorizationService) http.HandlerFunc {
	return resourceGrantDeleteHandler(grants, authz, model.GrantResourceNetwork, false)
}

func transferNetworkOwnershipHandler(grants *service.GrantService, authz *service.AuthorizationService) http.HandlerFunc {
	return resourceTransferHandler(grants, authz, model.GrantResourceNetwork, false)
}

func listSubnetGrantsHandler(grants *service.GrantService, authz *service.AuthorizationService) http.HandlerFunc {
	return resourceGrantListHandler(grants, authz, model.GrantResourceSubnet, true)
}

func createSubnetGrantHandler(grants *service.GrantService, authz *service.AuthorizationService) http.HandlerFunc {
	return resourceGrantCreateHandler(grants, authz, model.GrantResourceSubnet, true)
}

func getSubnetGrantHandler(grants *service.GrantService, authz *service.AuthorizationService) http.HandlerFunc {
	return resourceGrantGetHandler(grants, authz, model.GrantResourceSubnet, true)
}

func updateSubnetGrantHandler(grants *service.GrantService, authz *service.AuthorizationService) http.HandlerFunc {
	return resourceGrantUpdateHandler(grants, authz, model.GrantResourceSubnet, true)
}

func deleteSubnetGrantHandler(grants *service.GrantService, authz *service.AuthorizationService) http.HandlerFunc {
	return resourceGrantDeleteHandler(grants, authz, model.GrantResourceSubnet, true)
}

func transferSubnetOwnershipHandler(grants *service.GrantService, authz *service.AuthorizationService) http.HandlerFunc {
	return resourceTransferHandler(grants, authz, model.GrantResourceSubnet, true)
}

func resourceGrantListHandler(grants *service.GrantService, authz *service.AuthorizationService, kind model.GrantResourceKind, subnet bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceUUID, ok := grantResourceUUID(w, r, kind, subnet)
		if !ok {
			return
		}
		if !requireResourceOwner(w, r, authz, kind, resourceUUID) {
			return
		}
		list, err := grants.ListByResource(r.Context(), kind, resourceUUID)
		if err != nil {
			writeGrantError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, list)
	}
}

func resourceGrantCreateHandler(grants *service.GrantService, authz *service.AuthorizationService, kind model.GrantResourceKind, subnet bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceUUID, ok := grantResourceUUID(w, r, kind, subnet)
		if !ok {
			return
		}
		if !requireResourceOwner(w, r, authz, kind, resourceUUID) {
			return
		}
		var req createResourceGrantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		grant, err := grants.Create(r.Context(), principalFromContext(r.Context()), service.CreateGrantInput{
			Principal:   req.Principal.toModel(),
			Resource:    model.GrantResource{Kind: kind, UUID: &resourceUUID},
			Permissions: req.Permissions,
			Owner:       req.Owner,
		})
		if err != nil {
			writeGrantError(w, err)
			return
		}
		writeSuccess(w, http.StatusCreated, grant)
	}
}

func resourceGrantGetHandler(grants *service.GrantService, authz *service.AuthorizationService, kind model.GrantResourceKind, subnet bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceUUID, ok := grantResourceUUID(w, r, kind, subnet)
		if !ok {
			return
		}
		if !requireResourceOwner(w, r, authz, kind, resourceUUID) {
			return
		}
		id, err := parseUUID(r.PathValue("grant_uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid grant uuid")
			return
		}
		grant, err := grants.GetByUUID(r.Context(), id)
		if err != nil {
			writeGrantError(w, err)
			return
		}
		if !grants.GrantMatchesResource(grant, kind, resourceUUID) {
			writeError(w, http.StatusNotFound, "grant not found")
			return
		}
		writeSuccess(w, http.StatusOK, grant)
	}
}

func resourceGrantUpdateHandler(grants *service.GrantService, authz *service.AuthorizationService, kind model.GrantResourceKind, subnet bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceUUID, ok := grantResourceUUID(w, r, kind, subnet)
		if !ok {
			return
		}
		if !requireResourceOwner(w, r, authz, kind, resourceUUID) {
			return
		}
		id, err := parseUUID(r.PathValue("grant_uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid grant uuid")
			return
		}
		grant, err := grants.GetByUUID(r.Context(), id)
		if err != nil {
			writeGrantError(w, err)
			return
		}
		if !grants.GrantMatchesResource(grant, kind, resourceUUID) {
			writeError(w, http.StatusNotFound, "grant not found")
			return
		}
		var req updateGrantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		updated, err := grants.Update(r.Context(), principalFromContext(r.Context()), id, service.UpdateGrantInput{
			Permissions: req.Permissions,
			Owner:       req.Owner,
		})
		if err != nil {
			writeGrantError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, updated)
	}
}

func resourceGrantDeleteHandler(grants *service.GrantService, authz *service.AuthorizationService, kind model.GrantResourceKind, subnet bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceUUID, ok := grantResourceUUID(w, r, kind, subnet)
		if !ok {
			return
		}
		if !requireResourceOwner(w, r, authz, kind, resourceUUID) {
			return
		}
		id, err := parseUUID(r.PathValue("grant_uuid"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid grant uuid")
			return
		}
		grant, err := grants.GetByUUID(r.Context(), id)
		if err != nil {
			writeGrantError(w, err)
			return
		}
		if !grants.GrantMatchesResource(grant, kind, resourceUUID) {
			writeError(w, http.StatusNotFound, "grant not found")
			return
		}
		if err := grants.Delete(r.Context(), principalFromContext(r.Context()), id); err != nil {
			writeGrantError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, nil)
	}
}

func resourceTransferHandler(grants *service.GrantService, authz *service.AuthorizationService, kind model.GrantResourceKind, subnet bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceUUID, ok := grantResourceUUID(w, r, kind, subnet)
		if !ok {
			return
		}
		if !requireResourceOwner(w, r, authz, kind, resourceUUID) {
			return
		}
		var req transferOwnershipRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.To.Kind == "" {
			writeError(w, http.StatusBadRequest, "to principal is required")
			return
		}
		var from *model.GrantPrincipal
		if req.From != nil {
			p := req.From.toModel()
			from = &p
		}
		grant, err := grants.Transfer(r.Context(), principalFromContext(r.Context()), model.GrantResource{
			Kind: kind,
			UUID: &resourceUUID,
		}, service.TransferInput{
			From: from,
			To:   req.To.toModel(),
		})
		if err != nil {
			writeGrantError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, grant)
	}
}

func grantResourceUUID(w http.ResponseWriter, r *http.Request, kind model.GrantResourceKind, subnet bool) (uuid.UUID, bool) {
	if subnet {
		_, subnetUUID, ok := parseNetworkSubnetPath(w, r)
		return subnetUUID, ok
	}
	id, err := parseUUID(r.PathValue("uuid"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid network uuid")
		return uuid.Nil, false
	}
	_ = kind
	return id, true
}

func requireResourceOwner(w http.ResponseWriter, r *http.Request, authz *service.AuthorizationService, kind model.GrantResourceKind, resourceUUID uuid.UUID) bool {
	if authz == nil {
		return true
	}
	if err := authz.RequireOwner(r.Context(), principalFromContext(r.Context()), kind, resourceUUID); err != nil {
		writeAuthzError(w, err, notFoundMessage(kind))
		return false
	}
	return true
}

func notFoundMessage(kind model.GrantResourceKind) string {
	if kind == model.GrantResourceSubnet {
		return "subnet not found"
	}
	return "network not found"
}

func writeGrantError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrGrantNotFound):
		writeError(w, http.StatusNotFound, errorMessage(err, "grant not found"))
	case errors.Is(err, service.ErrGrantConflict), errors.Is(err, service.ErrLastOwner):
		writeError(w, http.StatusConflict, errorMessage(err, "grant conflict"))
	case errors.Is(err, service.ErrInvalidGrant),
		errors.Is(err, service.ErrGrantTargetAdmin),
		errors.Is(err, service.ErrGrantDisabledSubject),
		errors.Is(err, service.ErrGrantSubjectNotFound):
		writeError(w, http.StatusBadRequest, errorMessage(err, "invalid grant"))
	case errors.Is(err, service.ErrNetworkNotFound):
		writeError(w, http.StatusNotFound, errorMessage(err, "network not found"))
	case errors.Is(err, service.ErrSubnetNotFound):
		writeError(w, http.StatusNotFound, errorMessage(err, "subnet not found"))
	case errors.Is(err, service.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, errorMessage(err, "forbidden"))
	case errors.Is(err, service.ErrResourceNotVisible):
		writeError(w, http.StatusNotFound, "not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeAuthzError(w http.ResponseWriter, err error, notFound string) {
	switch {
	case errors.Is(err, service.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, errorMessage(err, "forbidden"))
	case errors.Is(err, service.ErrResourceNotVisible),
		errors.Is(err, service.ErrNetworkNotFound),
		errors.Is(err, service.ErrSubnetNotFound):
		writeError(w, http.StatusNotFound, notFound)
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
