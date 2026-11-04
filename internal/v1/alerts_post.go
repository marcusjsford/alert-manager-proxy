package v1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"alertmanager-proxy/internal/v1/envelope"
	"alertmanager-proxy/internal/v1/model"
	"alertmanager-proxy/internal/v1/types"
	"alertmanager-proxy/internal/v1/upstream"

	"go.opentelemetry.io/otel/attribute"
)

const (
	maxAlertNames    = 32
	maxAlertNameSize = 128
)

// postableAlert is the v2 alert shape. Missing times stay missing so the
// upstream applies the same now/resolve_timeout defaults as v1.
type postableAlert struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	StartsAt     *time.Time        `json:"startsAt,omitempty"`
	EndsAt       *time.Time        `json:"endsAt,omitempty"`
	GeneratorURL string            `json:"generatorURL,omitempty"`
}

func (r *Router) postAlerts(w http.ResponseWriter, req *http.Request) {
	var alerts []*types.Alert
	if err := json.NewDecoder(req.Body).Decode(&alerts); err != nil {
		envelope.Error(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	for _, alert := range alerts {
		if alert == nil {
			// 0.26 panicked on a null element; reject it rather than reproduce
			// that crash. See findings.
			envelope.Error(w, http.StatusBadRequest, "bad_data", "alert must not be null")
			return
		}
		if len(alert.Labels) == 0 {
			envelope.Error(w, http.StatusBadRequest, "bad_data", "at least one label pair required")
			return
		}
	}

	r.recordAlerts(req, alerts)

	payload, err := json.Marshal(postableAlerts(alerts))
	if err != nil {
		envelope.Error(w, http.StatusInternalServerError, "internal", "failed to encode alerts")
		return
	}
	v2Req, err := r.upstream.Request(req.Context(), http.MethodPost, "/api/v2/alerts",
		bytes.NewReader(payload), nil)
	if err != nil {
		upstream.WriteFailure(w, req, err, r.logger)
		return
	}
	upstream.ForwardHeaders(v2Req.Header, req.Header)
	v2Req.Header.Set("Content-Type", "application/json")
	if err := r.upstream.DoJSON(v2Req, nil); err != nil {
		r.writeAlertsUpstreamError(w, req, err)
		return
	}
	envelope.Success(w)
}

func (r *Router) recordAlerts(req *http.Request, alerts []*types.Alert) {
	if r.obs == nil {
		return
	}
	ctx := req.Context()
	r.obs.Alerts(ctx, int64(len(alerts)))
	firing, resolved := alertBreakdown(alerts)
	r.obs.Parameters(ctx, attribute.Int64("am.alerts.firing", firing),
		attribute.Int64("am.alerts.resolved", resolved))
	if names := alertNames(alerts); len(names) > 0 {
		r.obs.SpanParameters(ctx, attribute.StringSlice("am.alerts.names", names))
	}
	if hasSubMillisecondTimes(alerts) {
		r.obs.Lossy(ctx, "alert.timestamps", "millisecond precision")
	}
}

func (r *Router) writeAlertsUpstreamError(w http.ResponseWriter, req *http.Request, err error) {
	var responseErr *upstream.ResponseError
	if !errors.As(err, &responseErr) {
		upstream.WriteFailure(w, req, err, r.logger)
		return
	}
	if responseErr.Status >= 400 && responseErr.Status < 500 {
		envelope.Error(w, http.StatusBadRequest, "bad_data", upstreamMessage(responseErr))
		return
	}
	upstream.WriteFailure(w, req, err, r.logger)
}

func upstreamMessage(err *upstream.ResponseError) string {
	var message string
	if json.Unmarshal(err.Body, &message) == nil && message != "" {
		return message
	}
	var body struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(err.Body, &body) == nil && body.Message != "" {
		return body.Message
	}
	return fmt.Sprintf("upstream Alertmanager returned %d", err.Status)
}

func postableAlerts(alerts []*types.Alert) []postableAlert {
	out := make([]postableAlert, len(alerts))
	for i, alert := range alerts {
		out[i] = postableAlert{
			Labels:       stringMap(alert.Labels),
			Annotations:  stringMap(alert.Annotations),
			StartsAt:     timePtr(alert.StartsAt),
			EndsAt:       timePtr(alert.EndsAt),
			GeneratorURL: alert.GeneratorURL,
		}
	}
	return out
}

func stringMap(labels model.LabelSet) map[string]string {
	if labels == nil {
		return nil
	}
	out := make(map[string]string, len(labels))
	for name, value := range labels {
		out[string(name)] = string(value)
	}
	return out
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func alertNames(alerts []*types.Alert) []string {
	seen := map[string]bool{}
	names := make([]string, 0, min(len(alerts), maxAlertNames))
	for _, alert := range alerts {
		name := string(alert.Labels[model.LabelName("alertname")])
		if name == "" {
			continue
		}
		name = truncate(name, maxAlertNameSize)
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
		if len(names) == maxAlertNames {
			break
		}
	}
	return names
}

func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

func alertBreakdown(alerts []*types.Alert) (firing, resolved int64) {
	now := time.Now()
	for _, alert := range alerts {
		if !alert.EndsAt.IsZero() && alert.EndsAt.Before(now) {
			resolved++
		} else {
			firing++
		}
	}
	return firing, resolved
}

func hasSubMillisecondTimes(alerts []*types.Alert) bool {
	for _, alert := range alerts {
		for _, t := range []time.Time{alert.StartsAt, alert.EndsAt} {
			if !t.IsZero() && t.Nanosecond()%int(time.Millisecond) != 0 {
				return true
			}
		}
	}
	return false
}
