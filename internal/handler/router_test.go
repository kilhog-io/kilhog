package handler

import (
	"net/http"

	"github.com/kilhog-io/kilhog/internal/repository"
	"github.com/kilhog-io/kilhog/internal/service"
)

const handlerTestAPIKey = "handler-test-api-key"

// newAuthedTestRouter builds a router with a test API key and injects that key
// on requests that do not already carry credentials.
func newAuthedTestRouter(deps Dependencies) http.Handler {
	if deps.APIKey == "" {
		deps.APIKey = handlerTestAPIKey
	}
	return &autoAuthHandler{next: NewRouter(deps), apiKey: deps.APIKey}
}

func authDepsFromRepos(repos *repository.Repositories, apiKey string) Dependencies {
	auth := service.NewAuthService(
		repos.Users,
		repos.IdentityPools,
		repos.Sessions,
		repos.OIDCStates,
		service.AuthConfig{APIKey: apiKey},
	)
	return Dependencies{
		Store:               repos.Store,
		AuthService:         auth,
		UserService:         service.NewUserService(repos.Users),
		IdentityPoolService: service.NewIdentityPoolService(repos.IdentityPools),
		APIKey:              apiKey,
	}
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
