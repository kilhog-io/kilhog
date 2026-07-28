package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		wantStatusCode int
		wantBody       map[string]any
	}{
		{
			name:           "GET returns ok",
			method:         http.MethodGet,
			wantStatusCode: http.StatusOK,
			wantBody: map[string]any{
				"status": "success",
				"data": map[string]any{
					"status": "ok",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/healthz", nil)
			rec := httptest.NewRecorder()

			NewRouter(Dependencies{}).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Fatalf("status code = %d, want %d", rec.Code, tt.wantStatusCode)
			}

			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("content type = %q, want application/json", got)
			}

			var gotBody map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			gotStatus, _ := gotBody["status"].(string)
			if gotStatus != tt.wantBody["status"] {
				t.Fatalf("status = %q, want %q", gotStatus, tt.wantBody["status"])
			}

			gotData, ok := gotBody["data"].(map[string]any)
			if !ok {
				t.Fatalf("data = %#v, want map", gotBody["data"])
			}

			wantData := tt.wantBody["data"].(map[string]any)
			if gotData["status"] != wantData["status"] {
				t.Fatalf("data.status = %v, want %v", gotData["status"], wantData["status"])
			}
		})
	}
}
