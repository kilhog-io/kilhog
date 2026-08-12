package handler

import (
	"net/http"

	kilhoglog "github.com/kilhog-io/kilhog/internal/log"
	"github.com/kilhog-io/kilhog/internal/metrics"
	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/service"
)

type Dependencies struct {
	Store          *db.Store
	NetworkService *service.NetworkService
	SubnetService  *service.SubnetService
	APIKey         string
	Metrics        *metrics.Provider
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(deps.Store))
	if deps.Metrics != nil {
		mux.Handle("GET /metrics", deps.Metrics.Handler())
	}

	protected := http.NewServeMux()
	if deps.NetworkService != nil {
		registerNetworkRoutes(protected, deps.NetworkService)
	}
	if deps.SubnetService != nil {
		registerSubnetRoutes(protected, deps.SubnetService)
	}

	protectedHandler := apiKeyMiddleware(deps.APIKey, protected)
	mux.Handle("/", protectedHandler)

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
