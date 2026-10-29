package v1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"alertmanager-proxy/internal/v1/envelope"
	"alertmanager-proxy/internal/v1/filter"
	"alertmanager-proxy/internal/v1/upstream"

	"github.com/prometheus/alertmanager/pkg/labels"
	"go.opentelemetry.io/otel/attribute"
)

type postableSilence struct {
	ID        string          `json:"id,omitempty"`
	Matchers  labels.Matchers `json:"matchers"`
	StartsAt  time.Time       `json:"startsAt"`
	EndsAt    time.Time       `json:"endsAt"`
	CreatedBy string          `json:"createdBy"`
	Comment   string          `json:"comment,omitempty"`
}

type v1Silence struct {
	ID        string          `json:"id"`
	Matchers  labels.Matchers `json:"matchers"`
	StartsAt  time.Time       `json:"startsAt"`
	EndsAt    time.Time       `json:"endsAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
	CreatedBy string          `json:"createdBy"`
	Comment   string          `json:"comment,omitempty"`
	Status    v1SilenceStatus `json:"status"`
}

type v1SilenceStatus struct {
	State string `json:"state"`
}

func (r *Router) postSilences(w http.ResponseWriter, req *http.Request) {
	var silence postableSilence
	if err := json.NewDecoder(req.Body).Decode(&silence); err != nil {
		envelope.Error(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	operation := "create"
	if silence.ID != "" {
		operation = "update"
	}
	r.recordSilenceRequest(req, operation, silence.ID, silence.Matchers, silence.CreatedBy, silence.Comment)
	if err := validateSilence(silence); err != nil {
		envelope.Error(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}

	payload, err := json.Marshal(silence)
	if err != nil {
		envelope.Error(w, http.StatusInternalServerError, "internal", "failed to encode silence")
		return
	}
	v2Req, err := r.upstream.Request(req.Context(), http.MethodPost, "/api/v2/silences",
		bytes.NewReader(payload), nil)
	if err != nil {
		upstream.WriteFailure(w, req, err, r.logger)
		return
	}
	upstream.ForwardHeaders(v2Req.Header, req.Header)
	v2Req.Header.Set("Content-Type", "application/json")
	var created struct {
		SilenceID string `json:"silenceID"`
	}
	if err := r.upstream.DoJSON(v2Req, &created); err != nil {
		r.writeSilenceUpstreamError(w, req, err, operation)
		return
	}
	r.recordSilenceID(req, created.SilenceID)
	envelope.SuccessData(w, struct {
		SilenceID string `json:"silenceId"`
	}{SilenceID: created.SilenceID})
}

func validateSilence(silence postableSilence) error {
	// This follows v0.26's API checks and then silence.validateSilence, in the
	// same order, so error strings and precedence match the fixtures.
	if silence.StartsAt.Equal(silence.EndsAt) {
		return errors.New("start time must not be equal to end time")
	}
	if silence.EndsAt.Before(time.Now()) {
		return errors.New("end time can't be in the past")
	}
	if len(silence.Matchers) == 0 {
		return errors.New("silence invalid: at least one matcher required")
	}
	allMatchEmpty := true
	for i, matcher := range silence.Matchers {
		if matcher == nil {
			return fmt.Errorf("silence invalid: invalid label matcher %d", i)
		}
		allMatchEmpty = allMatchEmpty && matcherMatchesEmpty(matcher)
	}
	if allMatchEmpty {
		return errors.New("silence invalid: at least one matcher must not match the empty string")
	}
	if silence.StartsAt.IsZero() {
		return errors.New("silence invalid: invalid zero start timestamp")
	}
	if silence.EndsAt.IsZero() {
		return errors.New("silence invalid: invalid zero end timestamp")
	}
	if silence.EndsAt.Before(silence.StartsAt) {
		return errors.New("silence invalid: end time must not be before start time")
	}
	return nil
}

func matcherMatchesEmpty(matcher *labels.Matcher) bool {
	switch matcher.Type {
	case labels.MatchEqual:
		return matcher.Value == ""
	case labels.MatchRegexp:
		matched, _ := regexp.MatchString(matcher.Value, "")
		return matched
	default:
		return false
	}
}

func (r *Router) getSilences(w http.ResponseWriter, req *http.Request) {
	filterValue := req.FormValue("filter")
	matchers, err := filter.Parse(filterValue)
	if err != nil {
		envelope.Error(w, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	r.recordSilenceRequest(req, "list", "", nil, "", "")
	if filterValue != "" && r.obs != nil {
		r.obs.Parameters(req.Context(), attribute.String("am.v1.filter", filterValue))
	}

	v2Req, err := r.upstream.Request(req.Context(), http.MethodGet, "/api/v2/silences", nil, nil)
	if err != nil {
		upstream.WriteFailure(w, req, err, r.logger)
		return
	}
	upstream.ForwardHeaders(v2Req.Header, req.Header)
	var silences []v1Silence
	if err := r.upstream.DoJSON(v2Req, &silences); err != nil {
		r.writeSilenceUpstreamError(w, req, err, "list")
		return
	}

	filtered := make([]v1Silence, 0, len(silences))
	for _, silence := range silences {
		if silenceMatchesFilter(silence, matchers) {
			filtered = append(filtered, silence)
		}
	}
	filtered = sortSilences(filtered)
	if len(filtered) > 0 && r.obs != nil {
		r.obs.Lossy(req.Context(), "silence.timestamps", "millisecond precision")
	}
	envelope.SuccessData(w, filtered)
}

func silenceMatchesFilter(silence v1Silence, matchers []*labels.Matcher) bool {
	values := make(map[string]string, len(silence.Matchers))
	for _, matcher := range silence.Matchers {
		if matcher != nil {
			values[matcher.Name] = matcher.Value
		}
	}
	return matchFilterLabels(matchers, values)
}

func matchFilterLabels(matchers []*labels.Matcher, values map[string]string) bool {
	for _, matcher := range matchers {
		value, present := values[matcher.Name]
		switch matcher.Type {
		case labels.MatchNotRegexp, labels.MatchNotEqual:
			if matcher.Value == "" && present {
				continue
			}
			if !matcher.Matches(value) {
				return false
			}
		default:
			if matcher.Value == "" && !present {
				continue
			}
			if !matcher.Matches(value) {
				return false
			}
		}
	}
	return true
}

func sortSilences(silences []v1Silence) []v1Silence {
	var active, pending, expired, other []v1Silence
	for _, silence := range silences {
		switch silence.Status.State {
		case "active":
			active = append(active, silence)
		case "pending":
			pending = append(pending, silence)
		case "expired":
			expired = append(expired, silence)
		default:
			other = append(other, silence)
		}
	}
	sort.SliceStable(active, func(i, j int) bool { return active[i].EndsAt.Before(active[j].EndsAt) })
	sort.SliceStable(pending, func(i, j int) bool { return pending[i].StartsAt.Before(pending[j].StartsAt) })
	sort.SliceStable(expired, func(i, j int) bool { return expired[i].EndsAt.After(expired[j].EndsAt) })
	out := make([]v1Silence, 0, len(silences))
	out = append(out, active...)
	out = append(out, pending...)
	out = append(out, expired...)
	out = append(out, other...)
	return out
}

func (r *Router) getSilence(w http.ResponseWriter, req *http.Request) {
	id := silenceIDFromPath(req.URL.Path)
	r.recordSilenceRequest(req, "get", id, nil, "", "")
	v2Req, err := r.upstream.Request(req.Context(), http.MethodGet,
		"/api/v2/silence/"+url.PathEscape(id), nil, nil)
	if err != nil {
		upstream.WriteFailure(w, req, err, r.logger)
		return
	}
	upstream.ForwardHeaders(v2Req.Header, req.Header)
	var silence v1Silence
	if err := r.upstream.DoJSON(v2Req, &silence); err != nil {
		if isSilenceNotFound(err) {
			envelope.Text(w, http.StatusNotFound, "Error getting silence: <nil>\n")
			return
		}
		upstream.WriteFailure(w, req, err, r.logger)
		return
	}
	if r.obs != nil {
		r.obs.Parameters(req.Context(), attribute.String("am.silence.matchers", silence.Matchers.String()))
		r.obs.Lossy(req.Context(), "silence.timestamps", "millisecond precision")
	}
	envelope.SuccessData(w, silence)
}

func (r *Router) deleteSilence(w http.ResponseWriter, req *http.Request) {
	id := silenceIDFromPath(req.URL.Path)
	r.recordSilenceRequest(req, "delete", id, nil, "", "")
	v2Req, err := r.upstream.Request(req.Context(), http.MethodDelete,
		"/api/v2/silence/"+url.PathEscape(id), nil, nil)
	if err != nil {
		upstream.WriteFailure(w, req, err, r.logger)
		return
	}
	upstream.ForwardHeaders(v2Req.Header, req.Header)
	if err := r.upstream.DoJSON(v2Req, nil); err != nil {
		if isSilenceNotFound(err) {
			envelope.Error(w, http.StatusBadRequest, "bad_data", "silence not found")
			return
		}
		upstream.WriteFailure(w, req, err, r.logger)
		return
	}
	envelope.Success(w)
}

func silenceIDFromPath(path string) string {
	return strings.TrimPrefix(path, silencePrefix)
}

func isSilenceNotFound(err error) bool {
	var responseErr *upstream.ResponseError
	if !errors.As(err, &responseErr) {
		return false
	}
	return responseErr.Status == http.StatusNotFound ||
		responseErr.Status == http.StatusUnprocessableEntity ||
		responseErr.Status == http.StatusBadRequest
}

func (r *Router) writeSilenceUpstreamError(w http.ResponseWriter, req *http.Request, err error, operation string) {
	var responseErr *upstream.ResponseError
	if errors.As(err, &responseErr) && responseErr.Status >= 400 && responseErr.Status < 500 {
		if operation == "update" && responseErr.Status == http.StatusNotFound {
			envelope.Error(w, http.StatusBadRequest, "bad_data", "silence not found")
			return
		}
		envelope.Error(w, http.StatusBadRequest, "bad_data", upstreamMessage(responseErr))
		return
	}
	upstream.WriteFailure(w, req, err, r.logger)
}

func (r *Router) recordSilenceRequest(req *http.Request, operation, id string, matchers labels.Matchers, createdBy, comment string) {
	if r.obs == nil {
		return
	}
	attrs := []attribute.KeyValue{attribute.String("am.silence.operation", operation)}
	if id != "" {
		attrs = append(attrs, attribute.String("am.silence.id", id))
	}
	if matchers != nil {
		attrs = append(attrs, attribute.String("am.silence.matchers", matchers.String()))
	}
	if createdBy != "" {
		attrs = append(attrs, attribute.String("am.silence.createdBy", createdBy))
	}
	if comment != "" {
		attrs = append(attrs, attribute.String("am.silence.comment", comment))
	}
	r.obs.Parameters(req.Context(), attrs...)
}

func (r *Router) recordSilenceID(req *http.Request, id string) {
	if r.obs != nil && id != "" {
		r.obs.Parameters(req.Context(), attribute.String("am.silence.id", id))
	}
}
