package obs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Options struct {
	Level         string
	TraceExporter string
	OTLPEndpoint  string
	Service       string
	Instance      string
	LogOut        io.Writer
	TraceOut      io.Writer
}

type shutdowns []func(context.Context) error

func (s shutdowns) run(ctx context.Context) error {
	var errs []error
	for i := len(s) - 1; i >= 0; i-- {
		errs = append(errs, s[i](ctx))
	}
	return errors.Join(errs...)
}

func Setup(o Options) (func(context.Context) error, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(o.Level)); err != nil {
		return nil, fmt.Errorf("obs: log level %q: %w", o.Level, err)
	}
	switch o.TraceExporter {
	case "console", "none", "":
	default:
		return nil, fmt.Errorf("obs: unknown trace exporter %q", o.TraceExporter)
	}
	otel.SetTextMapPropagator(propagation.TraceContext{})

	res, err := newResource(o.Service, o.Instance)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(o.OTLPEndpoint, "/")
	var done shutdowns

	var processors []sdktrace.TracerProviderOption
	if o.TraceExporter == "console" {
		exp, err := stdouttrace.New(stdouttrace.WithWriter(o.TraceOut))
		if err != nil {
			return nil, err
		}
		processors = append(processors, sdktrace.WithBatcher(exp))
	}
	if endpoint != "" {
		exp, err := otlptracehttp.New(context.Background(), otlptracehttp.WithEndpointURL(endpoint+"/v1/traces"))
		if err != nil {
			return nil, fmt.Errorf("obs: OTLP trace exporter: %w", err)
		}
		processors = append(processors, sdktrace.WithBatcher(exp))
	}
	if len(processors) > 0 {
		tp := sdktrace.NewTracerProvider(append(processors, sdktrace.WithResource(res))...)
		otel.SetTracerProvider(tp)
		done = append(done, tp.Shutdown)
	}

	handler := NewHandler(o.LogOut, lvl)
	if endpoint != "" {
		exp, err := otlploghttp.New(context.Background(), otlploghttp.WithEndpointURL(endpoint+"/v1/logs"))
		if err != nil {
			return nil, fmt.Errorf("obs: OTLP log exporter: %w", err)
		}
		lp := sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)))
		done = append(done, lp.Shutdown)
		handler = fanout{handler, otlpHandler(lp, lvl)}
	}
	slog.SetDefault(slog.New(handler))
	return done.run, nil
}

func otlpHandler(lp *sdklog.LoggerProvider, level slog.Leveler) slog.Handler {
	bridge := otelslog.NewHandler(tracerName, otelslog.WithLoggerProvider(lp))
	return correlationHandler{masking{leveled{bridge, level}}}
}

func newResource(service, instance string) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{attribute.String("service.namespace", "datanest")}
	if service != "" {
		attrs = append(attrs, attribute.String("service.name", service))
	}
	if instance != "" {
		attrs = append(attrs, attribute.String("service.instance.id", instance))
	}
	return resource.New(context.Background(), resource.WithAttributes(attrs...), resource.WithTelemetrySDK(), resource.WithHost())
}
