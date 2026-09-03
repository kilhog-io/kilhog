package kilhog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestClientHealth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]string{
				"status": "ok",
			},
		})
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	status, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}
	if status.Status != "ok" {
		t.Fatalf("unexpected status: %q", status.Status)
	}
}

func TestClientAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "error",
			"message": "network not found",
			"code":    404,
		})
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(ClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	_, err = client.GetNetwork(context.Background(), uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"))
	if err == nil {
		t.Fatal("expected error")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected status code: %d", apiErr.StatusCode)
	}
	if apiErr.Message != "network not found" {
		t.Fatalf("unexpected message: %q", apiErr.Message)
	}
}

func TestGatewayErrorMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		raw        string
		wantSubstr string
		wantExact  string
	}{
		{
			name:       "forbidden html includes waf hint",
			statusCode: http.StatusForbidden,
			raw:        "<h1>Error: Forbidden</h1>",
			wantSubstr: "jsonParsing=STANDARD",
		},
		{
			name:       "forbidden empty body uses status text",
			statusCode: http.StatusForbidden,
			raw:        "  ",
			wantSubstr: "Forbidden",
		},
		{
			name:       "non-forbidden keeps raw body",
			statusCode: http.StatusBadGateway,
			raw:        "upstream timeout",
			wantExact:  "upstream timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := gatewayErrorMessage(tt.statusCode, []byte(tt.raw))
			if tt.wantExact != "" && got != tt.wantExact {
				t.Fatalf("gatewayErrorMessage() = %q, want %q", got, tt.wantExact)
			}
			if tt.wantSubstr != "" && !strings.Contains(got, tt.wantSubstr) {
				t.Fatalf("gatewayErrorMessage() = %q, want substring %q", got, tt.wantSubstr)
			}
		})
	}
}

func TestClientGatewayForbiddenHint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<h1>Error: Forbidden</h1>"))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(ClientConfig{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	_, err = client.CreateSubnetInNetwork(context.Background(), uuid.MustParse("2e58e87d-7d75-4f8a-acad-5056caacbfea"), CreateSubnetInput{
		Name:    "dmz",
		Prefix:  24,
		Address: "10.0.0.0",
		Type:    AddressTypeIPv4,
	})
	if err == nil {
		t.Fatal("expected error")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("unexpected status code: %d", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Message, "jsonParsing=STANDARD") {
		t.Fatalf("expected Cloud Armor JSON parsing hint, got %q", apiErr.Message)
	}
	if !strings.Contains(apiErr.Message, "942200") {
		t.Fatalf("expected OWASP CRS 942200 hint, got %q", apiErr.Message)
	}
}
