package handlers

import (
	"encoding/json"
	"log"
	"net/http"
)

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes an error JSON response
func writeError(w http.ResponseWriter, r *http.Request, status int, code, msg string, extra map[string]any) {
	body := map[string]any{"error": code}
	if msg != "" {
		body["message"] = msg
	}
	for k, v := range extra {
		body[k] = v
	}
	if status >= 500 {
		log.Printf("HTTP %d error code=%s msg=%s path=%s", status, code, msg, r.URL.Path)
	}
	writeJSON(w, status, body)
}
