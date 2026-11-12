package handler

import (
	"net/http"

	kilhoglog "github.com/kilhog-io/kilhog/internal/log"
	"github.com/kilhog-io/kilhog/internal/metrics"
	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/service"
)

type Dependencies struct {
	Store                  *db.Store
	NetworkService         *service.NetworkService
	SubnetService          *service.SubnetService
	AuthService            *service.AuthService
	UserService            *service.UserService
	IdentityPoolService    *service.IdentityPoolService
	MachineIdentityService *service.MachineIdentityService
	GrantService           *service.GrantService
	Authz                  *service.AuthorizationService
	APIKey                 string
	Metrics                *metrics.Provider
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(deps.Store))
	if deps.Metrics != nil {
		mux.Handle("GET /metrics", deps.Metrics.Handler())
	}
	registerAuthRoutes(mux, deps.AuthService, deps.IdentityPoolService)

	protected := http.NewServeMux()
	if deps.NetworkService != nil {
		registerNetworkRoutes(protected, deps.NetworkService, deps.Authz, deps.GrantService)
	}
	if deps.SubnetService != nil {
		registerSubnetRoutes(protected, deps.SubnetService, deps.Authz)
	}
	if deps.UserService != nil {
		protected.HandleFunc("POST /users/me/password", changeOwnPasswordHandler(deps.UserService))
	}
	registerUserAdminRoutes(protected, deps.UserService)
	registerIdentityPoolRoutes(protected, deps.IdentityPoolService)
	registerMachineIdentityRoutes(protected, deps.MachineIdentityService)
	registerGrantRoutes(protected, deps.GrantService, deps.Authz)

	mux.Handle("/", authMiddleware(deps.AuthService, deps.APIKey, protected))

	var handler http.Handler = mux
	if deps.Metrics != nil && deps.Metrics.HTTP != nil {
		handler = deps.Metrics.HTTP.Middleware(handler)
	}
	return kilhoglog.HTTPMiddleware(handler)
}

func healthHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if store != nil {
			if err := store.Ping(r.Context()); err != nil {
				writeError(w, http.StatusServiceUnavailable, "database unavailable")
				return
			}
		}

		writeSuccess(w, http.StatusOK, healthData{Status: "ok"})
	}
}

type healthData struct {
	Status string `json:"status"`
}
