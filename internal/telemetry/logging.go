package telemetry

import (
	"context"
	"errors"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// logHandler applies redaction and correlation before both JSON and OTLP output.
type logHandler struct {
	handlers []slog.Handler
	redactor redactor
}

func (h logHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h logHandler) attrs(attrs []slog.Attr) []slog.Attr {
	clean := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		attr.Value = attr.Value.Resolve()
		if h.redactor.private(attr.Key) {
			continue
		}
		if attr.Value.Kind() == slog.KindGroup {
			attr.Value = slog.GroupValue(h.attrs(attr.Value.Group())...)
		}
		clean = append(clean, attr)
	}
	return clean
}

func (h logHandler) Handle(ctx context.Context, record slog.Record) error {
	var attrs []slog.Attr
	record.Attrs(func(attr slog.Attr) bool { attrs = append(attrs, attr); return true })
	clean := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	clean.AddAttrs(h.attrs(attrs)...)
	span := trace.SpanContextFromContext(ctx)
	if span.IsValid() {
		clean.AddAttrs(slog.String("trace_id", span.TraceID().String()), slog.String("span_id", span.SpanID().String()))
	}
	var errs []error
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, record.Level) {
			errs = append(errs, handler.Handle(ctx, clean.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (h logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := logHandler{redactor: h.redactor}
	for _, handler := range h.handlers {
		out.handlers = append(out.handlers, handler.WithAttrs(h.attrs(attrs)))
	}
	return out
}

func (h logHandler) WithGroup(name string) slog.Handler {
	out := logHandler{redactor: h.redactor}
	for _, handler := range h.handlers {
		out.handlers = append(out.handlers, handler.WithGroup(name))
	}
	return out
}
