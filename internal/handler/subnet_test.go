package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/service"
)

func TestSubnetRoutes_Lifecycle(t *testing.T) {
	repos := openHandlerRepositories(t)
	networkSvc := service.NewNetworkService(repos.Networks, repos.Subnets)
	subnetSvc := service.NewSubnetService(repos.Subnets, repos.Networks)
	router := NewRouter(Dependencies{NetworkService: networkSvc, SubnetService: subnetSvc})

	network, err := networkSvc.Create(t.Context(), service.CreateNetworkInput{Name: "lab"})
	if err != nil {
		t.Fatalf("Create network: %v", err)
	}

	createBody := map[string]any{
		"name":        "dmz",
		"description": "DMZ subnet",
		"prefix":      24,
		"address":     "10.0.0.0",
		"type":        "ipv4",
	}
	createPayload, err := json.Marshal(createBody)
	if err != nil {
		t.Fatalf("marshal create body: %v", err)
	}

	base := fmt.Sprintf("/networks/%s/subnets", network.UUID)
	createReq := httptest.NewRequest(http.MethodPost, base, bytes.NewReader(createPayload))
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d, body = %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var createResp successResponse
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	subnetData, ok := createResp.Data.(map[string]any)
	if !ok {
		t.Fatalf("create data = %#v, want object", createResp.Data)
	}
	subnetUUID, ok := subnetData["uuid"].(string)
	if !ok || subnetUUID == "" {
		t.Fatalf("uuid = %#v, want non-empty string", subnetData["uuid"])
	}

	listReq := httptest.NewRequest(http.MethodGet, base, nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	subnetPath := fmt.Sprintf("%s/%s", base, subnetUUID)
	getReq := httptest.NewRequest(http.MethodGet, subnetPath, nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d, body = %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}

	updateBody := map[string]any{"description": "updated DMZ"}
	updatePayload, err := json.Marshal(updateBody)
	if err != nil {
		t.Fatalf("marshal update body: %v", err)
	}
	updateReq := httptest.NewRequest(http.MethodPut, subnetPath, bytes.NewReader(updatePayload))
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d, body = %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, subnetPath, nil)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, want %d, body = %s", deleteRec.Code, http.StatusOK, deleteRec.Body.String())
	}
}

func TestSubnetRoutes_CreateAutoAddress(t *testing.T) {
	repos := openHandlerRepositories(t)
	networkSvc := service.NewNetworkService(repos.Networks, repos.Subnets)
	subnetSvc := service.NewSubnetService(repos.Subnets, repos.Networks)
	router := NewRouter(Dependencies{NetworkService: networkSvc, SubnetService: subnetSvc})

	network, err := networkSvc.Create(t.Context(), service.CreateNetworkInput{Name: "lab"})
	if err != nil {
		t.Fatalf("Create network: %v", err)
	}

	parent, err := subnetSvc.CreateInNetwork(t.Context(), network.UUID, model.Parent{
		Kind: model.ParentKindNetwork,
		UUID: network.UUID,
	}, service.CreateSubnetInput{
		Name: "parent", Prefix: 24, Address: "10.0.0.0",
	})
	if err != nil {
		t.Fatalf("Create parent subnet: %v", err)
	}

	createBody := map[string]any{
		"name":   "auto",
		"prefix": 25,
		"type":   "ipv4",
	}
	createPayload, err := json.Marshal(createBody)
	if err != nil {
		t.Fatalf("marshal create body: %v", err)
	}

	path := fmt.Sprintf("/networks/%s/subnets/%s/subnets", network.UUID, parent.UUID)
	createReq := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(createPayload))
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d, body = %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var createResp successResponse
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	subnetData := createResp.Data.(map[string]any)
	if subnetData["address"] != "10.0.0.0" {
		t.Fatalf("address = %#v, want 10.0.0.0", subnetData["address"])
	}
}

func TestSubnetRoutes_AddressRequiredForNetworkParent(t *testing.T) {
	repos := openHandlerRepositories(t)
	networkSvc := service.NewNetworkService(repos.Networks, repos.Subnets)
	subnetSvc := service.NewSubnetService(repos.Subnets, repos.Networks)
	router := NewRouter(Dependencies{NetworkService: networkSvc, SubnetService: subnetSvc})

	network, err := networkSvc.Create(t.Context(), service.CreateNetworkInput{Name: "lab"})
	if err != nil {
		t.Fatalf("Create network: %v", err)
	}

	createBody := map[string]any{
		"name":   "no-address",
		"prefix": 24,
	}
	createPayload, _ := json.Marshal(createBody)
	path := fmt.Sprintf("/networks/%s/subnets", network.UUID)
	createReq := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(createPayload))
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("POST status = %d, want %d, body = %s", createRec.Code, http.StatusBadRequest, createRec.Body.String())
	}
}

func TestSubnetRoutes_OverlapConflict(t *testing.T) {
	repos := openHandlerRepositories(t)
	networkSvc := service.NewNetworkService(repos.Networks, repos.Subnets)
	subnetSvc := service.NewSubnetService(repos.Subnets, repos.Networks)
	router := NewRouter(Dependencies{NetworkService: networkSvc, SubnetService: subnetSvc})

	network, err := networkSvc.Create(t.Context(), service.CreateNetworkInput{Name: "lab"})
	if err != nil {
		t.Fatalf("Create network: %v", err)
	}

	parent := model.Parent{Kind: model.ParentKindNetwork, UUID: network.UUID}
	if _, err := subnetSvc.CreateInNetwork(t.Context(), network.UUID, parent, service.CreateSubnetInput{
		Name: "first", Prefix: 24, Address: "10.0.0.0",
	}); err != nil {
		t.Fatalf("Create first subnet: %v", err)
	}

	createBody := map[string]any{
		"name":    "overlap",
		"prefix":  24,
		"address": "10.0.0.0",
	}
	createPayload, _ := json.Marshal(createBody)
	path := fmt.Sprintf("/networks/%s/subnets", network.UUID)
	createReq := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(createPayload))
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusConflict {
		t.Fatalf("POST status = %d, want %d, body = %s", createRec.Code, http.StatusConflict, createRec.Body.String())
	}
}

func TestSubnetRoutes_NotFound(t *testing.T) {
	repos := openHandlerRepositories(t)
	networkSvc := service.NewNetworkService(repos.Networks, repos.Subnets)
	subnetSvc := service.NewSubnetService(repos.Subnets, repos.Networks)
	router := NewRouter(Dependencies{NetworkService: networkSvc, SubnetService: subnetSvc})

	network, err := networkSvc.Create(t.Context(), service.CreateNetworkInput{Name: "lab"})
	if err != nil {
		t.Fatalf("Create network: %v", err)
	}

	id := uuid.New()
	path := fmt.Sprintf("/networks/%s/subnets/%s", network.UUID, id)
	getReq := httptest.NewRequest(http.MethodGet, path, nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Fatalf("GET status = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestSubnetRoutes_NotInNetwork(t *testing.T) {
	repos := openHandlerRepositories(t)
	networkSvc := service.NewNetworkService(repos.Networks, repos.Subnets)
	subnetSvc := service.NewSubnetService(repos.Subnets, repos.Networks)
	router := NewRouter(Dependencies{NetworkService: networkSvc, SubnetService: subnetSvc})

	netA, err := networkSvc.Create(t.Context(), service.CreateNetworkInput{Name: "a"})
	if err != nil {
		t.Fatalf("Create network a: %v", err)
	}
	netB, err := networkSvc.Create(t.Context(), service.CreateNetworkInput{Name: "b"})
	if err != nil {
		t.Fatalf("Create network b: %v", err)
	}

	subnet, err := subnetSvc.CreateInNetwork(t.Context(), netA.UUID, model.Parent{
		Kind: model.ParentKindNetwork,
		UUID: netA.UUID,
	}, service.CreateSubnetInput{
		Name: "dmz", Prefix: 24, Address: "10.0.0.0",
	})
	if err != nil {
		t.Fatalf("Create subnet: %v", err)
	}

	path := fmt.Sprintf("/networks/%s/subnets/%s", netB.UUID, subnet.UUID)
	getReq := httptest.NewRequest(http.MethodGet, path, nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Fatalf("GET status = %d, want %d, body = %s", getRec.Code, http.StatusNotFound, getRec.Body.String())
	}
}

func TestSubnetRoutes_DeleteWithChildren(t *testing.T) {
	repos := openHandlerRepositories(t)
	networkSvc := service.NewNetworkService(repos.Networks, repos.Subnets)
	subnetSvc := service.NewSubnetService(repos.Subnets, repos.Networks)
	router := NewRouter(Dependencies{NetworkService: networkSvc, SubnetService: subnetSvc})

	network, err := networkSvc.Create(t.Context(), service.CreateNetworkInput{Name: "lab"})
	if err != nil {
		t.Fatalf("Create network: %v", err)
	}

	parent, err := subnetSvc.CreateInNetwork(t.Context(), network.UUID, model.Parent{
		Kind: model.ParentKindNetwork,
		UUID: network.UUID,
	}, service.CreateSubnetInput{
		Name: "parent", Prefix: 24, Address: "10.0.0.0",
	})
	if err != nil {
		t.Fatalf("Create parent: %v", err)
	}

	_, err = subnetSvc.CreateInNetwork(t.Context(), network.UUID, model.Parent{
		Kind: model.ParentKindSubnet,
		UUID: parent.UUID,
	}, service.CreateSubnetInput{
		Name: "child", Prefix: 25, Address: "10.0.0.0",
	})
	if err != nil {
		t.Fatalf("Create child: %v", err)
	}

	path := fmt.Sprintf("/networks/%s/subnets/%s", network.UUID, parent.UUID)
	deleteReq := httptest.NewRequest(http.MethodDelete, path, nil)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusConflict {
		t.Fatalf("DELETE status = %d, want %d, body = %s", deleteRec.Code, http.StatusConflict, deleteRec.Body.String())
	}
}

func TestSubnetRoutes_ListChildren(t *testing.T) {
	repos := openHandlerRepositories(t)
	networkSvc := service.NewNetworkService(repos.Networks, repos.Subnets)
	subnetSvc := service.NewSubnetService(repos.Subnets, repos.Networks)
	router := NewRouter(Dependencies{NetworkService: networkSvc, SubnetService: subnetSvc})

	network, err := networkSvc.Create(t.Context(), service.CreateNetworkInput{Name: "lab"})
	if err != nil {
		t.Fatalf("Create network: %v", err)
	}

	parent, err := subnetSvc.CreateInNetwork(t.Context(), network.UUID, model.Parent{
		Kind: model.ParentKindNetwork,
		UUID: network.UUID,
	}, service.CreateSubnetInput{
		Name: "parent", Prefix: 24, Address: "10.0.0.0",
	})
	if err != nil {
		t.Fatalf("Create parent: %v", err)
	}

	_, err = subnetSvc.CreateInNetwork(t.Context(), network.UUID, model.Parent{
		Kind: model.ParentKindSubnet,
		UUID: parent.UUID,
	}, service.CreateSubnetInput{
		Name: "child", Prefix: 25, Address: "10.0.0.0",
	})
	if err != nil {
		t.Fatalf("Create child: %v", err)
	}

	path := fmt.Sprintf("/networks/%s/subnets/%s/subnets", network.UUID, parent.UUID)
	listReq := httptest.NewRequest(http.MethodGet, path, nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d, body = %s", listRec.Code, http.StatusOK, listRec.Body.String())
	}

	var listResp successResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	items, ok := listResp.Data.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("list = %#v, want one child", listResp.Data)
	}
}
