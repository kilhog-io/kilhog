package handler

import "net/http"

type healthData struct {
	Status string `json:"status"`
}

func Health(w http.ResponseWriter, _ *http.Request) {
	writeSuccess(w, http.StatusOK, healthData{Status: "ok"})
}
