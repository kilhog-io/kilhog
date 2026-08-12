package handler

import "net/http"

const handlerTestAPIKey = "handler-test-api-key"

// newAuthedTestRouter builds a router with a test API key and injects that key
// on requests that do not already carry credentials.
func newAuthedTestRouter(deps Dependencies) http.Handler {
	if deps.APIKey == "" {
		deps.APIKey = handlerTestAPIKey
	}
	return &autoAuthHandler{next: NewRouter(deps), apiKey: deps.APIKey}
}

type autoAuthHandler struct {
	next   http.Handler
	apiKey string
}

func (h *autoAuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") == "" && r.Header.Get("X-API-Key") == "" {
		r.Header.Set("Authorization", "Bearer "+h.apiKey)
	}
	h.next.ServeHTTP(w, r)
}
