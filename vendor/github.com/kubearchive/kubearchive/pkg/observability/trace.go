// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/host"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// the name of the environment variable that will determine if instrumentation needs to be started
const OtelStartEnvVar = "KUBEARCHIVE_OTEL_MODE"
const OtelMetricsInterval = "KUBEARCHIVE_METRICS_INTERVAL"
const OtelLogsEnvVar = "KUBEARCHIVE_OTLP_SEND_LOGS"

// name of the environment variable that if a float, will determine the sampling rate for root spans created by
// KubeArchive
const OtelSamplingRateEnvVar = "OTEL_TRACES_SAMPLER_ARG"

var tp *trace.TracerProvider

// Start creates a Span Processor and exporter, registers them with a TracerProvider, and sets the default
// TracerProvider and SetTextMapPropagator
func Start(serviceName string) error {
	if canSkipInit() {
		return nil
	}

	res, err := resource.New(
		context.Background(),
		resource.WithTelemetrySDK(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
		resource.WithFromEnv(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.K8SPodName(os.Getenv("POD_NAME")),
		),
	)
	if err != nil {
		return err
	}

	traceExporter, err := otlptracehttp.New(context.Background())
	if err != nil {
		return err
	}

	tracerProviderOptions := []trace.TracerProviderOption{
		trace.WithBatcher(traceExporter),
		trace.WithResource(res),
	}

	otelMode := os.Getenv(OtelStartEnvVar)
	sampler := trace.AlwaysSample()
	if sampleRateRaw, exists := os.LookupEnv(OtelSamplingRateEnvVar); exists {
		sampleRate, parseErr := strconv.ParseFloat(sampleRateRaw, 64)
		if parseErr != nil {
			slog.Error(
				"Failed to parse trace sample rate as float. Falling back to always sample.",
				OtelSamplingRateEnvVar,
				sampleRateRaw,
				"error",
				parseErr,
			)
		} else {
			sampler = trace.TraceIDRatioBased(sampleRate)
		}
	}
	if otelMode == "enabled" {
		tracerProviderOptions = append(tracerProviderOptions, trace.WithSampler(sampler))
	} else if otelMode == "delegated" {
		// This is the default, I didn't want to leave an empty block here. This could drift in the future.
		tracerProviderOptions = append(tracerProviderOptions, trace.WithSampler(trace.ParentBased(sampler)))
	} else {
		// "disabled" is not checked in this if/else because the code does not get here when the value is "disabled"
		return fmt.Errorf("value '%s' for '%s' not valid. Use 'disabled', 'enabled' or 'delegated'", otelMode, OtelStartEnvVar)
	}

	tp = trace.NewTracerProvider(tracerProviderOptions...)

	otel.SetTracerProvider(tp)

	metricExporter, err := otlpmetrichttp.New(context.Background())
	if err != nil {
		return err
	}

	metricsInterval := 1 * time.Minute // Default
	metricsIntervalRaw := os.Getenv(OtelMetricsInterval)
	if metricsIntervalRaw != "" {
		metricsInterval, err = time.ParseDuration(metricsIntervalRaw)
		if err != nil {
			return err
		}
	}

	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(metricExporter, metric.WithInterval(metricsInterval))),
	)

	otel.SetMeterProvider(mp)
	err = host.Start(host.WithMeterProvider(mp))
	if err != nil {
		return err
	}

	err = runtime.Start(runtime.WithMeterProvider(mp))
	if err != nil {
		return err
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	if os.Getenv(OtelLogsEnvVar) == "true" {
		slog.Info(fmt.Sprintf("'%s' is set to 'true' so logs will be redirected to the OTLP endpoint", OtelLogsEnvVar))

		logger := otelslog.NewLogger("root")
		slog.SetDefault(logger)

		exporter, err := otlploghttp.New(context.Background())
		if err != nil {
			return err
		}
		processor := log.NewBatchProcessor(exporter)

		loggerProvider := log.NewLoggerProvider(
			log.WithResource(res),
			log.WithProcessor(processor),
		)

		global.SetLoggerProvider(loggerProvider)
	}

	return nil
}

// canSkipInit returns a bool representing if OtelStartEnvVar is set to "disabled" or not. This function is a helper for Start.
// Instrumentation should *ONLY* be started if this function returns false
func canSkipInit() bool {
	startEnv := os.Getenv(OtelStartEnvVar)
	return strings.ToLower(startEnv) == "disabled"
}

// FlushSpanBuffer exports all completed spans that have not been exported for all SpanProcessors registered with the
// global TracerProvider. If the provided context has a timeout or a deadline, it will be respected.
func FlushSpanBuffer(ctx context.Context) error {
	if tp != nil {
		return tp.ForceFlush(ctx)
	}

	return fmt.Errorf("cannot flush spans. No TracerProvider has been configured")
}

// Shutdown shuts down the TracerProvider, any SpanProcessors that have been registered, and exporters associated with
// the SpanProcessors. This should only be called after all spans have been ended. After this function is called, spans
// cannot be created, ended or modified.
func Shutdown(ctx context.Context) error {
	if tp != nil {
		err := FlushSpanBuffer(ctx)
		if err != nil {
			return err
		}
		return tp.Shutdown(ctx)
	}

	return fmt.Errorf("cannot shutdown TracerProvider. None have been started")
}

func Status() string {
	observabilityConfig := os.Getenv(OtelStartEnvVar)
	if observabilityConfig == "" {
		return "disabled"
	}
	return observabilityConfig
}
