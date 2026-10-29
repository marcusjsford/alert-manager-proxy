package telemetry

import (
	"net/url"
	"strings"

	"alertmanager-proxy/internal/config"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type redactor struct{ cfg config.Redaction }

func normalKey(key string) string {
	return strings.NewReplacer("-", "", "_", "", ".", "").Replace(strings.ToLower(key))
}

func (r redactor) private(key string) bool {
	key = normalKey(key)
	if strings.HasSuffix(key, "authorization") {
		return true
	}
	for _, header := range r.cfg.Headers {
		name := normalKey(header)
		if name != "" && (strings.HasSuffix(key, name) || (name == "useragent" && key == "useragentoriginal")) {
			return true
		}
	}
	return (r.cfg.SilenceComment && strings.HasSuffix(key, "comment")) ||
		(r.cfg.SilenceCreatedBy && strings.HasSuffix(key, "createdby"))
}

func (r redactor) attributes(attrs []attribute.KeyValue) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(attrs))
	for _, kv := range attrs {
		key := string(kv.Key)
		if r.private(key) || key == "url.query" || key == "url.fragment" {
			continue
		}
		if key == "url.full" || key == "http.url" || key == "http.target" {
			u, err := url.Parse(kv.Value.AsString())
			if err != nil {
				continue
			}
			u.User, u.RawQuery, u.Fragment = nil, "", ""
			kv.Value = attribute.StringValue(u.String())
		} else if key == "exception.message" || key == "exception.stacktrace" {
			// Transport errors may contain credential-bearing URLs. Status/type
			// attributes remain, but raw exception text is not exported.
			kv.Value = attribute.StringValue("request failed")
		} else {
			kv.Value = r.value(kv.Value)
		}
		out = append(out, kv)
	}
	return out
}

func (r redactor) value(v attribute.Value) attribute.Value {
	switch v.Type() {
	case attribute.MAP:
		return attribute.MapValue(r.attributes(v.AsMap())...)
	case attribute.SLICE:
		values := v.AsSlice()
		out := make([]attribute.Value, len(values))
		for i, item := range values {
			out[i] = r.value(item)
		}
		return attribute.SliceValue(out...)
	default:
		return v
	}
}

type cleanSpan struct {
	sdktrace.ReadOnlySpan
	redactor redactor
}

func (s cleanSpan) Attributes() []attribute.KeyValue {
	attrs := s.redactor.attributes(s.ReadOnlySpan.Attributes())
	// Hijacked upgrades write headers directly to the connection, outside the
	// library's ResponseWriter. Preserve the status actually observed upstream.
	for _, kv := range attrs {
		if kv.Key != "am.response.status_code" {
			continue
		}
		code := kv.Value.AsInt64()
		clean := make([]attribute.KeyValue, 0, len(attrs)+1)
		for _, attr := range attrs {
			if attr.Key != "http.response.status_code" && attr.Key != "http.status_code" {
				clean = append(clean, attr)
			}
		}
		if code > 0 {
			clean = append(clean, attribute.Int64("http.response.status_code", code))
		}
		return clean
	}
	return attrs
}

func (s cleanSpan) Events() []sdktrace.Event {
	events := s.ReadOnlySpan.Events()
	clean := make([]sdktrace.Event, len(events))
	for i, event := range events {
		clean[i] = event
		clean[i].Attributes = s.redactor.attributes(event.Attributes)
	}
	return clean
}

func (s cleanSpan) Links() []sdktrace.Link {
	links := s.ReadOnlySpan.Links()
	clean := make([]sdktrace.Link, len(links))
	for i, link := range links {
		clean[i] = link
		clean[i].Attributes = s.redactor.attributes(link.Attributes)
	}
	return clean
}

func (s cleanSpan) Status() sdktrace.Status {
	status := s.ReadOnlySpan.Status()
	if status.Code == codes.Error {
		status.Description = "request failed"
	}
	return status
}
