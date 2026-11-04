// Package inspect adds read-only, bounded v2 request/response inspection on the
// pass-through path. It never rewrites the bytes it forwards.
package inspect

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/alertmanager/pkg/labels"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// MaxBody bounds how much of a v2 body is parsed. Larger bodies are forwarded
// unchanged and reported as skipped.
const MaxBody = 1 << 20

const (
	maxAlertNames    = 32
	maxAlertNameSize = 128
)

// Observer is the telemetry surface used by the inspector. Runtime implements it.
type Observer interface {
	Alerts(ctx context.Context, count int64)
	Parameters(ctx context.Context, attrs ...attribute.KeyValue)
	SpanParameters(ctx context.Context, attrs ...attribute.KeyValue)
	ParametersOn(ctx context.Context, span trace.Span, attrs ...attribute.KeyValue)
	InspectionSkipped(ctx context.Context, reason string)
}

type serverSpanKey struct{}

// WithServerSpan carries the inbound server span through the outbound request
// context; response inspection runs under the client span and must attach to
// the server span instead.
func WithServerSpan(ctx context.Context, span trace.Span) context.Context {
	return context.WithValue(ctx, serverSpanKey{}, span)
}

type postableAlert struct {
	Labels   map[string]string `json:"labels"`
	StartsAt time.Time         `json:"startsAt"`
	EndsAt   time.Time         `json:"endsAt"`
}

type postableSilence struct {
	ID        string          `json:"id,omitempty"`
	Matchers  labels.Matchers `json:"matchers"`
	CreatedBy string          `json:"createdBy"`
	Comment   string          `json:"comment"`
}

// Request inspects a v2 pass-through request in place. It buffers at most
// MaxBody bytes and reconstructs the exact original body for forwarding.
func Request(req *http.Request, obs Observer) {
	if obs == nil {
		return
	}
	path := req.URL.Path
	switch {
	case req.Method == http.MethodPost && path == "/api/v2/alerts":
		body, ok := readRequestBody(req, obs)
		if !ok {
			return
		}
		var alerts []postableAlert
		if err := json.Unmarshal(body, &alerts); err != nil {
			obs.InspectionSkipped(req.Context(), "alerts_parse_failed")
			return
		}
		recordAlerts(req.Context(), obs, alerts)
	case req.Method == http.MethodGet &&
		(path == "/api/v2/alerts" || path == "/api/v2/alerts/groups"):
		recordAlertQuery(req.Context(), obs, req)
	case req.Method == http.MethodGet && path == "/api/v2/silences":
		recordSilenceList(req.Context(), obs, req)
	case req.Method == http.MethodPost && path == "/api/v2/silences":
		body, ok := readRequestBody(req, obs)
		if !ok {
			return
		}
		var silence postableSilence
		if err := json.Unmarshal(body, &silence); err != nil {
			obs.InspectionSkipped(req.Context(), "silence_parse_failed")
			return
		}
		recordSilenceRequest(req.Context(), obs, silence)
	case req.Method == http.MethodGet:
		if id, ok := silencePathID(path); ok {
			obs.Parameters(req.Context(), attribute.String("am.silence.operation", "get"),
				attribute.String("am.silence.id", id))
		}
	case req.Method == http.MethodDelete:
		if id, ok := silencePathID(path); ok {
			obs.Parameters(req.Context(), attribute.String("am.silence.operation", "delete"),
				attribute.String("am.silence.id", id))
		}
	}
}

// Response inspects the small v2 responses that carry IDs or matchers needed
// for parity with the translated v1 span attributes. The body is replaced with
// the exact bytes read.
func Response(resp *http.Response, obs Observer) {
	if obs == nil || resp == nil || resp.Request == nil {
		return
	}
	req := resp.Request
	path := req.URL.Path

	switch {
	case req.Method == http.MethodPost && path == "/api/v2/silences":
		raw, ok := readResponseBody(resp)
		if !ok {
			return
		}
		var body struct {
			SilenceID string `json:"silenceID"`
		}
		if json.Unmarshal(raw, &body) == nil && body.SilenceID != "" {
			parametersOn(req.Context(), obs, attribute.String("am.silence.id", body.SilenceID))
		}
	case req.Method == http.MethodGet:
		if _, ok := silencePathID(path); !ok {
			return
		}
		raw, ok := readResponseBody(resp)
		if !ok {
			return
		}
		var body struct {
			Matchers labels.Matchers `json:"matchers"`
		}
		if json.Unmarshal(raw, &body) == nil && len(body.Matchers) > 0 {
			parametersOn(req.Context(), obs, attribute.String("am.silence.matchers", body.Matchers.String()))
		}
	}
}

func parametersOn(ctx context.Context, obs Observer, attrs ...attribute.KeyValue) {
	span, _ := ctx.Value(serverSpanKey{}).(trace.Span)
	if span != nil {
		obs.ParametersOn(ctx, span, attrs...)
	}
}

func recordAlerts(ctx context.Context, obs Observer, alerts []postableAlert) {
	now := time.Now()
	var firing, resolved int64
	seen := map[string]bool{}
	names := make([]string, 0, min(len(alerts), maxAlertNames))
	for _, alert := range alerts {
		if !alert.EndsAt.IsZero() && alert.EndsAt.Before(now) {
			resolved++
		} else {
			firing++
		}
		name := alert.Labels["alertname"]
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
	obs.Alerts(ctx, int64(len(alerts)))
	obs.Parameters(ctx, attribute.Int64("am.alerts.firing", firing),
		attribute.Int64("am.alerts.resolved", resolved))
	if len(names) > 0 {
		obs.SpanParameters(ctx, attribute.StringSlice("am.alerts.names", names))
	}
}

func recordAlertQuery(ctx context.Context, obs Observer, req *http.Request) {
	query := req.URL.Query()
	if filters := query["filter"]; len(filters) > 0 {
		obs.Parameters(ctx, attribute.String("am.v1.filter", strings.Join(filters, ",")))
	}
	if receiver := query.Get("receiver"); receiver != "" {
		obs.Parameters(ctx, attribute.String("am.v1.receiver", receiver))
	}
	for key, name := range map[string]string{
		"active": "active", "silenced": "silenced",
		"inhibited": "inhibited", "unprocessed": "unprocessed",
	} {
		value := true
		if raw := query.Get(key); raw != "" {
			parsed, err := strconv.ParseBool(raw)
			if err != nil {
				continue
			}
			value = parsed
		}
		obs.Parameters(ctx, attribute.Bool("am.v1."+name, value))
	}
}

func recordSilenceList(ctx context.Context, obs Observer, req *http.Request) {
	obs.Parameters(ctx, attribute.String("am.silence.operation", "list"))
	if filters := req.URL.Query()["filter"]; len(filters) > 0 {
		obs.Parameters(ctx, attribute.String("am.v1.filter", strings.Join(filters, ",")))
	}
}

func recordSilenceRequest(ctx context.Context, obs Observer, silence postableSilence) {
	operation := "create"
	if silence.ID != "" {
		operation = "update"
	}
	attrs := []attribute.KeyValue{attribute.String("am.silence.operation", operation)}
	if silence.ID != "" {
		attrs = append(attrs, attribute.String("am.silence.id", silence.ID))
	}
	if len(silence.Matchers) > 0 {
		attrs = append(attrs, attribute.String("am.silence.matchers", silence.Matchers.String()))
	}
	if silence.CreatedBy != "" {
		attrs = append(attrs, attribute.String("am.silence.createdBy", silence.CreatedBy))
	}
	if silence.Comment != "" {
		attrs = append(attrs, attribute.String("am.silence.comment", silence.Comment))
	}
	obs.Parameters(ctx, attrs...)
}

func silencePathID(path string) (string, bool) {
	const prefix = "/api/v2/silence/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	id := strings.TrimPrefix(path, prefix)
	return id, id != "" && !strings.Contains(id, "/")
}

func readRequestBody(req *http.Request, obs Observer) ([]byte, bool) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, false
	}
	original := req.Body
	body, err := io.ReadAll(io.LimitReader(original, MaxBody+1))
	if err != nil {
		req.Body = &replayReadCloser{Reader: io.MultiReader(bytes.NewReader(body), original), Closer: original}
		obs.InspectionSkipped(req.Context(), "request_read_failed")
		return nil, false
	}
	if len(body) > MaxBody {
		req.Body = &replayReadCloser{Reader: io.MultiReader(bytes.NewReader(body), original), Closer: original}
		obs.InspectionSkipped(req.Context(), "request_too_large")
		return nil, false
	}
	_ = original.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	return body, true
}

func readResponseBody(resp *http.Response) ([]byte, bool) {
	if resp.Body == nil || resp.Body == http.NoBody {
		return nil, false
	}
	original := resp.Body
	raw, err := io.ReadAll(io.LimitReader(original, MaxBody+1))
	if err != nil {
		resp.Body = &replayReadCloser{Reader: io.MultiReader(bytes.NewReader(raw), original), Closer: original}
		return nil, false
	}
	if len(raw) > MaxBody {
		resp.Body = &replayReadCloser{Reader: io.MultiReader(bytes.NewReader(raw), original), Closer: original}
		return nil, false
	}
	_ = original.Close()
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	return raw, true
}

type replayReadCloser struct {
	io.Reader
	io.Closer
}

func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}
