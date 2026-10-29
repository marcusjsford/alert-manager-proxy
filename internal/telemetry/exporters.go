package telemetry

import (
	"context"
	"errors"
	"strings"

	"alertmanager-proxy/internal/config"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func newTraceExporter(ctx context.Context, cfg config.Telemetry) (sdktrace.SpanExporter, error) {
	if cfg.Protocol == "grpc" {
		return otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(cfg.Endpoint),
			otlptracegrpc.WithTimeout(cfg.ExportTimeout), otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{Enabled: false}))
	}
	return otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(strings.TrimRight(cfg.Endpoint, "/")+"/v1/traces"),
		otlptracehttp.WithTimeout(cfg.ExportTimeout), otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false}))
}

func newMetricExporter(ctx context.Context, cfg config.Telemetry) (sdkmetric.Exporter, error) {
	if cfg.Protocol == "grpc" {
		return otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpointURL(cfg.Endpoint),
			otlpmetricgrpc.WithTimeout(cfg.ExportTimeout), otlpmetricgrpc.WithRetry(otlpmetricgrpc.RetryConfig{Enabled: false}))
	}
	return otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(strings.TrimRight(cfg.Endpoint, "/")+"/v1/metrics"),
		otlpmetrichttp.WithTimeout(cfg.ExportTimeout), otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig{Enabled: false}))
}

func newLogExporter(ctx context.Context, cfg config.Telemetry) (sdklog.Exporter, error) {
	if cfg.Protocol == "grpc" {
		return otlploggrpc.New(ctx, otlploggrpc.WithEndpointURL(cfg.Endpoint),
			otlploggrpc.WithTimeout(cfg.ExportTimeout), otlploggrpc.WithRetry(otlploggrpc.RetryConfig{Enabled: false}))
	}
	return otlploghttp.New(ctx, otlploghttp.WithEndpointURL(strings.TrimRight(cfg.Endpoint, "/")+"/v1/logs"),
		otlploghttp.WithTimeout(cfg.ExportTimeout), otlploghttp.WithRetry(otlploghttp.RetryConfig{Enabled: false}))
}

type traceExporter struct {
	sdktrace.SpanExporter
	redactor redactor
	failed   func(string)
}

func (e traceExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	clean := make([]sdktrace.ReadOnlySpan, len(spans))
	for i, span := range spans {
		clean[i] = cleanSpan{ReadOnlySpan: span, redactor: e.redactor}
	}
	err := e.SpanExporter.ExportSpans(ctx, clean)
	if err != nil {
		e.failed("traces")
	}
	return err
}

type metricExporter struct {
	sdkmetric.Exporter
	failed func(string)
}

func (e metricExporter) Export(ctx context.Context, data *metricdata.ResourceMetrics) error {
	err := e.Exporter.Export(ctx, data)
	if err != nil {
		e.failed("metrics")
	}
	return err
}

type logExporter struct {
	sdklog.Exporter
	redactor redactor
	failed   func(string)
}

func (e logExporter) Export(ctx context.Context, records []sdklog.Record) error {
	clean := make([]sdklog.Record, len(records))
	for i, record := range records {
		clean[i] = record.Clone()
		var attrs []attribute.KeyValue
		record.WalkAttributes(func(kv attribute.KeyValue) bool { attrs = append(attrs, kv); return true })
		clean[i].SetAttributes(e.redactor.attributes(attrs)...)
		clean[i].SetBody(e.redactor.value(record.Body()))
	}
	err := e.Exporter.Export(ctx, clean)
	if err != nil {
		e.failed("logs")
	}
	return err
}

func closeExporters(ctx context.Context, traces sdktrace.SpanExporter, metrics sdkmetric.Exporter, logs sdklog.Exporter) error {
	var errs []error
	if traces != nil {
		errs = append(errs, traces.Shutdown(ctx))
	}
	if metrics != nil {
		errs = append(errs, metrics.Shutdown(ctx))
	}
	if logs != nil {
		errs = append(errs, logs.Shutdown(ctx))
	}
	return errors.Join(errs...)
}
