package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/tespio/go-rdap-server/internal/rdap"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/rdap+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status, errorCode int, title, description string) {
	resp := rdap.ErrorResponse{
		Conformance: rdap.NewConformance(),
		ErrorCode:   errorCode,
		Title:       title,
		Description: []string{description},
	}
	writeJSON(w, status, resp)
}
