// Package v1 serves the Alertmanager API v1 surface, translated to v2.
//
// Until each endpoint's story lands (040-070), every v1 route answers 501 in
// the v1 error envelope, so a legacy client sees a clear "not yet" from the
// proxy rather than the upstream's 410 "removed" body.
package v1

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Prefix is the path prefix owned by this package.
const Prefix = "/api/v1"

// Owns reports whether path belongs to the v1 API.
func Owns(path string) bool {
	return path == Prefix || strings.HasPrefix(path, Prefix+"/")
}

// errorBody is the v1 error envelope, as recorded from Alertmanager v0.26.0:
// {"status":"error","errorType":"bad_data","error":"..."}.
type errorBody struct {
	Status    string `json:"status"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
}

// NewRouter returns the v1 handler.
func NewRouter() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotImplemented, "not_implemented",
			"alertmanager-proxy: "+r.Method+" "+r.URL.Path+" is not translated to API v2 yet")
	})
}

func writeError(w http.ResponseWriter, status int, errorType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Status: "error", ErrorType: errorType, Error: msg})
}
