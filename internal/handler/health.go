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
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(deps.Store))
	if deps.NetworkService != nil {
		registerNetworkRoutes(mux, deps.NetworkService)
	}
	if deps.SubnetService != nil {
		registerSubnetRoutes(mux, deps.SubnetService)
	}
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
