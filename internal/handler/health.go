package handler

import (
	"net/http"

	"github.com/kilhog-io/kilhog/internal/repository/db"
)

type Dependencies struct {
	Store *db.Store
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(deps.Store))
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
