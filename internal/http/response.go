package httpapi

import (
	"encoding/json"
	"net/http"
)

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type ResponseEnvelope struct {
	Data  any           `json:"data,omitempty"`
	Meta  any           `json:"meta,omitempty"`
	Error *ErrorPayload `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, data any, meta any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ResponseEnvelope{Data: data, Meta: meta})
}

func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ResponseEnvelope{
		Error: &ErrorPayload{Code: code, Message: message, Details: details},
	})
}
