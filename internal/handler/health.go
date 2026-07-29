package handler

import (
	"net/http"

	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/service"
)

type Dependencies struct {
	Store          *db.Store
	NetworkService *service.NetworkService
	SubnetService  *service.SubnetService
	APIKey         string
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(deps.Store))

	protected := http.NewServeMux()
	if deps.NetworkService != nil {
		registerNetworkRoutes(protected, deps.NetworkService)
	}
	if deps.SubnetService != nil {
		registerSubnetRoutes(protected, deps.SubnetService)
	}

	protectedHandler := http.Handler(protected)
	if deps.APIKey != "" {
		protectedHandler = apiKeyMiddleware(deps.APIKey, protected)
	}
	mux.Handle("/", protectedHandler)

	return mux
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
