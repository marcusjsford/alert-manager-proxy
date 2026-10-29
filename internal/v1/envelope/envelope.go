// Package envelope writes the HTTP contract recorded from Alertmanager
// v0.26.0: CORS/cache headers, the success and error JSON envelopes, and the
// occasional plain-text response.
package envelope

import (
	"encoding/json"
	"net/http"
)

// Contract header values recorded from Alertmanager v0.26.0.
const (
	AllowHeaders    = "Accept, Authorization, Content-Type, Origin"
	AllowMethods    = "GET, POST, DELETE, OPTIONS"
	ExposeHeaders   = "Date"
	CacheControl    = "no-cache, no-store, must-revalidate"
	ContentTypeJSON = "application/json"
	ContentTypeText = "text/plain; charset=utf-8"
)

type successBody struct {
	Status string `json:"status"`
}

type successDataBody struct {
	Status string `json:"status"`
	Data   any    `json:"data"`
}

type errorBody struct {
	Status    string `json:"status"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
}

// SetContractHeaders adds the CORS and cache headers v0.26 sent with every
// implemented v1 response.
func SetContractHeaders(h http.Header) {
	h.Set("Access-Control-Allow-Headers", AllowHeaders)
	h.Set("Access-Control-Allow-Methods", AllowMethods)
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Expose-Headers", ExposeHeaders)
	h.Set("Cache-Control", CacheControl)
}

// Preflight writes the shared CORS preflight response: 200, no body, and no
// Content-Type, independent of whether the route is implemented.
func Preflight(w http.ResponseWriter) {
	SetContractHeaders(w.Header())
	w.WriteHeader(http.StatusOK)
}

// Success writes {"status":"success"}.
func Success(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, successBody{Status: "success"})
}

// SuccessData writes {"status":"success","data":...}; data is always present,
// including when it is nil.
func SuccessData(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, successDataBody{Status: "success", Data: data})
}

// Error writes the v1 error envelope.
func Error(w http.ResponseWriter, status int, errorType, message string) {
	writeJSON(w, status, errorBody{Status: "error", ErrorType: errorType, Error: message})
}

// Text writes a plain-text v1 response with the shared contract headers.
func Text(w http.ResponseWriter, status int, body string) {
	SetContractHeaders(w.Header())
	w.Header().Set("Content-Type", ContentTypeText)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		status = http.StatusInternalServerError
		raw = []byte(`{"status":"error","errorType":"internal","error":"failed to encode response"}`)
	}
	SetContractHeaders(w.Header())
	w.Header().Set("Content-Type", ContentTypeJSON)
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}
