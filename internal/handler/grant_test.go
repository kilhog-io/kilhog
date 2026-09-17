package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/service"
)

func TestRBAC_OwnerGrantsAndTransfer(t *testing.T) {
	repos := openHandlerRepositories(t)
	deps := authDepsFromRepos(repos, "")
	deps.NetworkService = service.NewNetworkService(repos.Networks, repos.Subnets)
	deps.SubnetService = service.NewSubnetService(repos.Subnets, repos.Networks)
	router := NewRouter(deps)

	adminToken := bootstrapSession(t, router, "admin", "password123")
	operator := createLocalUser(t, router, adminToken, "operator", model.UserRoleUser)
	other := createLocalUser(t, router, adminToken, "other", model.UserRoleUser)

	platformBody, _ := json.Marshal(map[string]any{
		"principal": map[string]any{
			"kind":            "local_user",
			"local_user_uuid": operator,
		},
		"capability": "create_networks",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/platform-grants", bytes.NewReader(platformBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("platform grant status = %d, body = %s", rec.Code, rec.Body.String())
	}

	opToken := loginSession(t, router, "operator", "password123")
	otherToken := loginSession(t, router, "other", "password123")

	createBody, _ := json.Marshal(map[string]any{"name": "lab"})
	req = httptest.NewRequest(http.MethodPost, "/networks", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create network status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created successResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	network := created.Data.(map[string]any)
	networkUUID := network["uuid"].(string)

	req = httptest.NewRequest(http.MethodGet, "/networks", nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("other list status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var listResp successResponse
	if err := json.NewDecoder(rec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if items, _ := listResp.Data.([]any); len(items) != 0 {
		t.Fatalf("other should see no networks, got %#v", listResp.Data)
	}

	req = httptest.NewRequest(http.MethodGet, "/networks/"+networkUUID, nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("other get status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}

	shareBody, _ := json.Marshal(map[string]any{
		"principal": map[string]any{
			"kind":            "local_user",
			"local_user_uuid": other,
		},
		"permissions": map[string]bool{"read": true},
		"owner":       false,
	})
	req = httptest.NewRequest(http.MethodPost, "/networks/"+networkUUID+"/grants", bytes.NewReader(shareBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("owner create grant status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/networks/"+networkUUID, nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reader get status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/networks/"+networkUUID, nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reader delete status = %d, want 403, body = %s", rec.Code, rec.Body.String())
	}

	transferBody, _ := json.Marshal(map[string]any{
		"to": map[string]any{
			"kind":            "local_user",
			"local_user_uuid": other,
		},
	})
	req = httptest.NewRequest(http.MethodPost, "/networks/"+networkUUID+"/ownership/transfer", bytes.NewReader(transferBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("transfer status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/networks/"+networkUUID+"/grants", bytes.NewReader(shareBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("former owner grant admin status = %d, want 403 or 404, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/auth/platform-grants", nil)
	req.Header.Set("Authorization", "Bearer "+opToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin platform grants status = %d, want 403", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/auth/me/grants", nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me grants status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestRBAC_APIKeyCannotManageGrants(t *testing.T) {
	repos := openHandlerRepositories(t)
	const apiKey = "rbac-api-key"
	deps := authDepsFromRepos(repos, apiKey)
	deps.NetworkService = service.NewNetworkService(repos.Networks, repos.Subnets)
	router := NewRouter(deps)

	createBody, _ := json.Marshal(map[string]any{"name": "from-key"})
	req := httptest.NewRequest(http.MethodPost, "/networks", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("api key create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created successResponse
	_ = json.NewDecoder(rec.Body).Decode(&created)
	networkUUID := created.Data.(map[string]any)["uuid"].(string)

	req = httptest.NewRequest(http.MethodGet, "/networks/"+networkUUID+"/grants", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("api key list grants status = %d, want 403, body = %s", rec.Code, rec.Body.String())
	}
}

func TestRBAC_CreateNetworksForbiddenWithoutGrant(t *testing.T) {
	repos := openHandlerRepositories(t)
	deps := authDepsFromRepos(repos, "")
	deps.NetworkService = service.NewNetworkService(repos.Networks, repos.Subnets)
	router := NewRouter(deps)

	adminToken := bootstrapSession(t, router, "admin", "password123")
	_ = createLocalUser(t, router, adminToken, "operator", model.UserRoleUser)
	opToken := loginSession(t, router, "operator", "password123")

	createBody, _ := json.Marshal(map[string]any{"name": "denied"})
	req := httptest.NewRequest(http.MethodPost, "/networks", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("create without grant status = %d, want 403, body = %s", rec.Code, rec.Body.String())
	}
}

func bootstrapSession(t *testing.T, router http.Handler, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("bootstrap status = %d, body = %s", rec.Code, rec.Body.String())
	}
	return sessionTokenFromBody(t, rec.Body.Bytes())
}

func loginSession(t *testing.T, router http.Handler, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body.String())
	}
	return sessionTokenFromBody(t, rec.Body.Bytes())
}

func createLocalUser(t *testing.T, router http.Handler, adminToken, username string, role model.UserRole) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"username": username,
		"password": "password123",
		"role":     role,
	})
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp successResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode user: %v", err)
	}
	data := resp.Data.(map[string]any)
	id, _ := data["uuid"].(string)
	if id == "" {
		t.Fatal("expected user uuid")
	}
	return id
}

func sessionTokenFromBody(t *testing.T, raw []byte) string {
	t.Helper()
	var resp struct {
		Data struct {
			Session struct {
				Token string `json:"token"`
			} `json:"session"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if resp.Data.Session.Token == "" {
		t.Fatal("expected session token")
	}
	return resp.Data.Session.Token
}
