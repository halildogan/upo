/*
Copyright 2026 The Unified Platform Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package telemetry wires OpenTelemetry tracing into the operator. The global
// TracerProvider is configured with service resource attributes and a
// parent-based, ratio sampler. A span exporter is intentionally pluggable: set
// OTEL_EXPORTER_OTLP_ENDPOINT and register an OTLP exporter (see docs) in
// production. Without an exporter the SDK is a no-op beyond span creation, so
// the Reconcile hooks remain free in development.
package telemetry

import (
	"context"
	"os"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// tracerName is the instrumentation scope name for all operator spans.
const tracerName = "github.com/halildogan/upo"

// Setup configures and installs the global TracerProvider. It returns a
// shutdown function that flushes any pending spans; callers should defer it.
func Setup(ctx context.Context, serviceVersion string) (func(context.Context) error, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("unified-platform-operator"),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(samplingRatio()))),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// Tracer returns the operator's tracer. Controllers call this to open spans.
func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

// samplingRatio reads OTEL_TRACES_SAMPLER_ARG (0.0-1.0), defaulting to 0.1.
func samplingRatio() float64 {
	const def = 0.1
	v := os.Getenv("OTEL_TRACES_SAMPLER_ARG")
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 || f > 1 {
		return def
	}
	return f
}
