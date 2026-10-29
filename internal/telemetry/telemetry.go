// Package telemetry owns asynchronous OTLP signals and the bounded request model.
package telemetry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"

	"alertmanager-proxy/internal/clientid"
	"alertmanager-proxy/internal/config"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Runtime owns its providers, so tests/instances do not change global OTel state.
type Runtime struct {
	Enabled  bool
	Logger   *slog.Logger
	Instance string
	Resolver *clientid.Resolver
	traces   *sdktrace.TracerProvider
	metrics  *sdkmetric.MeterProvider
	logs     *sdklog.LoggerProvider
	redactor redactor
	prop     propagation.TextMapPropagator
	requests metric.Int64Counter
	duration metric.Float64Histogram
	upstream metric.Float64Histogram
	lossy    metric.Int64Counter
	alerts   metric.Int64Counter
}

// New configures lazy exporters; a collector connection is not a startup gate.
func New(ctx context.Context, cfg config.Config, version string, output io.Writer) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	resolver, err := clientid.New(cfg.Clients)
	if err != nil {
		return nil, err
	}
	r := &Runtime{Enabled: cfg.Telemetry.Enabled, Resolver: resolver, redactor: redactor{cfg.Redaction}, prop: propagation.TraceContext{}}
	r.Instance = cfg.Telemetry.InstanceID
	if r.Instance == "" {
		r.Instance = cfg.UpstreamURL().Host
	}
	jsonHandler := slog.NewJSONHandler(output, nil)
	r.Logger = slog.New(logHandler{handlers: []slog.Handler{jsonHandler}, redactor: r.redactor})
	if !r.Enabled {
		return r, nil
	}
	traces, err := newTraceExporter(ctx, cfg.Telemetry)
	if err != nil {
		return nil, err
	}
	metrics, err := newMetricExporter(ctx, cfg.Telemetry)
	if err != nil {
		return nil, errors.Join(err, closeExporters(ctx, traces, nil, nil))
	}
	logs, err := newLogExporter(ctx, cfg.Telemetry)
	if err != nil {
		return nil, errors.Join(err, closeExporters(ctx, traces, metrics, nil))
	}
	failed := func(signal string) {
		// Do not feed exporter failures back into the failing log exporter.
		slog.New(jsonHandler).Warn("telemetry export failed", "signal", signal)
	}
	res := resource.NewSchemaless(
		attribute.String("service.name", cfg.Telemetry.ServiceName),
		attribute.String("service.version", version),
		attribute.String("service.instance.id", r.Instance),
		attribute.String("deployment.environment.name", cfg.Telemetry.Environment),
	)
	r.traces = sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(traceExporter{traces, r.redactor, failed}, sdktrace.WithMaxQueueSize(2048),
			sdktrace.WithBatchTimeout(cfg.Telemetry.BatchInterval), sdktrace.WithExportTimeout(cfg.Telemetry.ExportTimeout)))
	reader := sdkmetric.NewPeriodicReader(metricExporter{metrics, failed}, sdkmetric.WithInterval(cfg.Telemetry.MetricInterval), sdkmetric.WithTimeout(cfg.Telemetry.ExportTimeout))
	r.metrics = sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(reader),
		// Values come only from configured names, fixed routes/methods and codes.
		// Avoid the SDK's extra otel.metric.overflow label, outside the ADR model.
		sdkmetric.WithCardinalityLimit(0), sdkmetric.WithView(sdkmetric.NewView(sdkmetric.Instrument{Name: "*"},
			sdkmetric.Stream{AttributeFilter: func(kv attribute.KeyValue) bool { return metricLabel(string(kv.Key)) }})))
	r.logs = sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewBatchProcessor(
		logExporter{logs, r.redactor, failed}, sdklog.WithMaxQueueSize(2048), sdklog.WithExportInterval(cfg.Telemetry.BatchInterval),
		sdklog.WithExportTimeout(cfg.Telemetry.ExportTimeout))))
	r.Logger = slog.New(logHandler{handlers: []slog.Handler{jsonHandler, otelslog.NewHandler("alertmanager-proxy", otelslog.WithLoggerProvider(r.logs))}, redactor: r.redactor})
	meter := r.metrics.Meter("alertmanager-proxy")
	var errs []error
	r.requests, err = meter.Int64Counter("am_proxy_requests_total")
	errs = append(errs, err)
	r.duration, err = meter.Float64Histogram("am_proxy_request_duration_seconds", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.001, .005, .01, .05, .1, .5, 1, 5))
	errs = append(errs, err)
	r.upstream, err = meter.Float64Histogram("am_proxy_upstream_duration_seconds", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.001, .005, .01, .05, .1, .5, 1, 5))
	errs = append(errs, err)
	r.lossy, err = meter.Int64Counter("am_proxy_translation_lossy_total")
	errs = append(errs, err)
	r.alerts, err = meter.Int64Counter("am_proxy_alerts_received_total")
	errs = append(errs, err)
	if err := errors.Join(errs...); err != nil {
		return nil, errors.Join(err, r.Shutdown(ctx))
	}
	return r, nil
}

func metricLabel(key string) bool {
	switch key {
	case "client", "api_version", "endpoint", "method", "status_code", "instance":
		return true
	default:
		return false
	}
}

func parallel(ctx context.Context, calls ...func(context.Context) error) error {
	results := make(chan error, len(calls))
	var wg sync.WaitGroup
	for _, call := range calls {
		wg.Go(func() { results <- call(ctx) })
	}
	wg.Wait()
	close(results)
	var errs []error
	for err := range results {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Flush is an explicit diagnostic/test operation; requests never wait for export.
func (r *Runtime) Flush(ctx context.Context) error {
	if !r.Enabled {
		return nil
	}
	return parallel(ctx, r.traces.ForceFlush, r.metrics.ForceFlush, r.logs.ForceFlush)
}

// Shutdown shares one caller-supplied deadline across all signal providers.
func (r *Runtime) Shutdown(ctx context.Context) error {
	if !r.Enabled {
		return nil
	}
	return parallel(ctx, r.traces.Shutdown, r.metrics.Shutdown, r.logs.Shutdown)
}
