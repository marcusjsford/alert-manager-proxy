package v1

import (
	"net/http"

	"alertmanager-proxy/internal/v1/envelope"
	"alertmanager-proxy/internal/v1/upstream"
)

func (r *Router) getReceivers(w http.ResponseWriter, req *http.Request) {
	v2Req, err := r.upstream.Request(req.Context(), http.MethodGet, "/api/v2/receivers", nil, nil)
	if err != nil {
		upstream.WriteFailure(w, req, err, r.logger)
		return
	}
	upstream.ForwardHeaders(v2Req.Header, req.Header)
	var receivers []struct {
		Name string `json:"name"`
	}
	if err := r.upstream.DoJSON(v2Req, &receivers); err != nil {
		r.writeAlertsUpstreamError(w, req, err)
		return
	}
	names := make([]string, 0, len(receivers))
	for _, receiver := range receivers {
		names = append(names, receiver.Name)
	}
	envelope.SuccessData(w, names)
}
