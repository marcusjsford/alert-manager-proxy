package v1

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"time"

	"alertmanager-proxy/internal/v1/envelope"
	"alertmanager-proxy/internal/v1/filter"
	"alertmanager-proxy/internal/v1/upstream"

	"go.opentelemetry.io/otel/attribute"
)

type v2GettableAlert struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
	Receivers    []struct {
		Name string `json:"name"`
	} `json:"receivers"`
	Status struct {
		State       string   `json:"state"`
		SilencedBy  []string `json:"silencedBy"`
		InhibitedBy []string `json:"inhibitedBy"`
	} `json:"status"`
}

type v1GettableAlert struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Status       v1AlertStatus     `json:"status"`
	Receivers    []string          `json:"receivers"`
	Fingerprint  string            `json:"fingerprint"`
}

type v1AlertStatus struct {
	State       string   `json:"state"`
	SilencedBy  []string `json:"silencedBy"`
	InhibitedBy []string `json:"inhibitedBy"`
}

func (r *Router) getAlerts(w http.ResponseWriter, req *http.Request) {
	filterValue := req.FormValue("filter")
	matchers, err := filter.Parse(filterValue)
	if err != nil {
		envelope.Error(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	active, activeRaw, err := boolParam(req, "active")
	if err != nil {
		envelope.Error(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	silenced, silencedRaw, err := boolParam(req, "silenced")
	if err != nil {
		envelope.Error(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	inhibited, inhibitedRaw, err := boolParam(req, "inhibited")
	if err != nil {
		envelope.Error(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	unprocessed, unprocessedRaw, err := boolParam(req, "unprocessed")
	if err != nil {
		envelope.Error(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	receiver := req.FormValue("receiver")
	if receiver != "" {
		if _, err := regexp.Compile("^(?:" + receiver + ")$"); err != nil {
			envelope.Error(w, http.StatusBadRequest, "bad_data",
				fmt.Sprintf("failed to parse receiver param: %s", receiver))
			return
		}
	}

	r.recordAlertQuery(req, filterValue, receiver, active, silenced, inhibited, unprocessed)

	query := url.Values{}
	for _, value := range filter.Values(matchers) {
		query.Add("filter", value)
	}
	if receiver != "" {
		query.Set("receiver", receiver)
	}
	if activeRaw != "" {
		query.Set("active", activeRaw)
	}
	if silencedRaw != "" {
		query.Set("silenced", silencedRaw)
	}
	if inhibitedRaw != "" {
		query.Set("inhibited", inhibitedRaw)
	}
	if unprocessedRaw != "" {
		query.Set("unprocessed", unprocessedRaw)
	}

	v2Req, err := r.upstream.Request(req.Context(), http.MethodGet, "/api/v2/alerts", nil, query)
	if err != nil {
		upstream.WriteFailure(w, req, err, r.logger)
		return
	}
	upstream.ForwardHeaders(v2Req.Header, req.Header)
	var v2Alerts []v2GettableAlert
	if err := r.upstream.DoJSON(v2Req, &v2Alerts); err != nil {
		r.writeAlertsUpstreamError(w, req, err)
		return
	}

	alerts := make([]v1GettableAlert, 0, len(v2Alerts))
	for _, alert := range v2Alerts {
		if !unprocessed && alert.Status.State == "unprocessed" {
			continue
		}
		alerts = append(alerts, reshapeAlert(alert))
	}
	sort.Slice(alerts, func(i, j int) bool {
		return alerts[i].Fingerprint < alerts[j].Fingerprint
	})
	if len(alerts) > 0 && r.obs != nil {
		r.obs.Lossy(req.Context(), "alert.timestamps", "millisecond precision")
	}
	envelope.SuccessData(w, alerts)
}

func boolParam(req *http.Request, name string) (value bool, raw string, err error) {
	raw = req.FormValue(name)
	switch raw {
	case "":
		return true, "", nil
	case "true":
		return true, raw, nil
	case "false":
		return false, raw, nil
	default:
		return false, raw, fmt.Errorf("parameter %q can either be 'true' or 'false', not %q", name, raw)
	}
}

func reshapeAlert(alert v2GettableAlert) v1GettableAlert {
	annotations := alert.Annotations
	if len(annotations) == 0 {
		annotations = nil
	}
	receivers := make([]string, 0, len(alert.Receivers))
	for _, receiver := range alert.Receivers {
		receivers = append(receivers, receiver.Name)
	}
	silencedBy, inhibitedBy := alert.Status.SilencedBy, alert.Status.InhibitedBy
	if alert.Status.State == "unprocessed" {
		// v1 exposed empty arrays, not null, before the marker saw the alert.
		if silencedBy == nil {
			silencedBy = []string{}
		}
		if inhibitedBy == nil {
			inhibitedBy = []string{}
		}
	} else {
		// v1's processed marker exposed null for an empty set; v2 normalises
		// both to [].
		if len(silencedBy) == 0 {
			silencedBy = nil
		}
		if len(inhibitedBy) == 0 {
			inhibitedBy = nil
		}
	}
	return v1GettableAlert{
		Labels:       alert.Labels,
		Annotations:  annotations,
		StartsAt:     alert.StartsAt,
		EndsAt:       alert.EndsAt,
		GeneratorURL: alert.GeneratorURL,
		Status: v1AlertStatus{
			State:       alert.Status.State,
			SilencedBy:  silencedBy,
			InhibitedBy: inhibitedBy,
		},
		Receivers:   receivers,
		Fingerprint: alert.Fingerprint,
	}
}

func (r *Router) recordAlertQuery(req *http.Request, filterValue, receiver string, active, silenced, inhibited, unprocessed bool) {
	if r.obs == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.Bool("am.v1.active", active),
		attribute.Bool("am.v1.silenced", silenced),
		attribute.Bool("am.v1.inhibited", inhibited),
		attribute.Bool("am.v1.unprocessed", unprocessed),
	}
	if filterValue != "" {
		attrs = append(attrs, attribute.String("am.v1.filter", filterValue))
	}
	if receiver != "" {
		attrs = append(attrs, attribute.String("am.v1.receiver", receiver))
	}
	r.obs.Parameters(req.Context(), attrs...)
}
