package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/repository"
	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/service"
)

func TestNetworkRoutes_Lifecycle(t *testing.T) {
	repos := openHandlerRepositories(t)
	svc := service.NewNetworkService(repos.Networks, repos.Subnets)
	router := newAuthedTestRouter(Dependencies{NetworkService: svc})

	createBody := map[string]any{
		"name":        "lab",
		"description": "test network",
		"tags":        []map[string]string{{"key": "env", "value": "dev"}},
	}
	createPayload, err := json.Marshal(createBody)
	if err != nil {
		t.Fatalf("marshal create body: %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/networks", bytes.NewReader(createPayload))
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d, body = %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var createResp successResponse
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	networkData, ok := createResp.Data.(map[string]any)
	if !ok {
		t.Fatalf("create data = %#v, want object", createResp.Data)
	}
	networkUUID, ok := networkData["uuid"].(string)
	if !ok || networkUUID == "" {
		t.Fatalf("uuid = %#v, want non-empty string", networkData["uuid"])
	}

	listReq := httptest.NewRequest(http.MethodGet, "/networks", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /networks status = %d, want %d", listRec.Code, http.StatusOK)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/networks/"+networkUUID, nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /networks/{uuid} status = %d, want %d", getRec.Code, http.StatusOK)
	}

	updateBody := map[string]any{
		"name":        "lab-updated",
		"description": "updated",
	}
	updatePayload, err := json.Marshal(updateBody)
	if err != nil {
		t.Fatalf("marshal update body: %v", err)
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/networks/"+networkUUID, bytes.NewReader(updatePayload))
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d, body = %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/networks/"+networkUUID, nil)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, want %d, body = %s", deleteRec.Code, http.StatusOK, deleteRec.Body.String())
	}
}

func TestNetworkRoutes_DeleteWithChildren(t *testing.T) {
	repos := openHandlerRepositories(t)
	svc := service.NewNetworkService(repos.Networks, repos.Subnets)
	router := newAuthedTestRouter(Dependencies{NetworkService: svc})

	network, err := svc.Create(t.Context(), service.CreateNetworkInput{Name: "protected"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	subnetID := uuid.New()
	if err := repos.Subnets.Create(t.Context(), &model.Subnet{
		UUID:    subnetID,
		Name:    "child",
		Prefix:  24,
		Address: "192.168.0.0",
		Type:    model.AddressTypeIPv4,
		Parent: model.Parent{
			Kind: model.ParentKindNetwork,
			UUID: network.UUID,
		},
	}); err != nil {
		t.Fatalf("create subnet: %v", err)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/networks/"+network.UUID.String(), nil)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusConflict {
		t.Fatalf("DELETE status = %d, want %d, body = %s", deleteRec.Code, http.StatusConflict, deleteRec.Body.String())
	}
}

func TestNetworkRoutes_NotFound(t *testing.T) {
	repos := openHandlerRepositories(t)
	svc := service.NewNetworkService(repos.Networks, repos.Subnets)
	router := newAuthedTestRouter(Dependencies{NetworkService: svc})

	id := uuid.New()
	getReq := httptest.NewRequest(http.MethodGet, "/networks/"+id.String(), nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Fatalf("GET status = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func openHandlerRepositories(t *testing.T) *repository.Repositories {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "kilhog.db")
	cfg := db.Config{
		Driver:      db.DialectSQLite,
		DSN:         "file:" + dbPath + "?_pragma=foreign_keys(ON)",
		AutoMigrate: true,
	}

	repos, err := repository.Open(t.Context(), cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = repos.Close() })

	return repos
}
