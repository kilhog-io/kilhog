package handler

import (
	"encoding/json"
	"net/http"
)

type successResponse struct {
	Status string `json:"status"`
	Data   any    `json:"data"`
}

type errorResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, `{"status":"error","message":"failed to encode response","code":500}`, http.StatusInternalServerError)
	}
}

func writeSuccess(w http.ResponseWriter, statusCode int, data any) {
	writeJSON(w, statusCode, successResponse{
		Status: "success",
		Data:   data,
	})
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, errorResponse{
		Status:  "error",
		Message: message,
		Code:    statusCode,
	})
}
