package main

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/sdk/resource"
	"io"
	"log"
	fib "opentelemetry-fib/fib"
	"opentelemetry-fib/http"
	"os"
	"os/signal"
	"syscall"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
)

func main() {
	l := log.New(os.Stdout, "", 0)

	// Write telemetry data to a file.
	f, err := os.Create("traces.yaml")
	if err != nil {
		l.Fatal(err)
	}

	exp, err := newExporter(f)
	if err != nil {
		l.Fatal(err)
	}

	client := otlptracegrpc.NewClient()
	exporter, err := otlptrace.New(context.Background(), client)
	if err != nil {
		l.Fatal("creating OTLP trace exporter: %w", err)
	}

	// You have your application instrumented to produce telemetry data and you have an exporter to send that data to the console, but how are they connected?
	// The pipelines that receive and ultimately transmit data to exporters are called SpanProcessor
	// This is done with a BatchSpanProcessor when it is passed to the trace.WithBatcher option. Batching data is a good practice and will help not overload systems downstream.
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithBatcher(exp), //configured to have multiple span processors
		trace.WithResource(newResource()),
	)
	// you are deferring a function to flush and stop it
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			l.Fatal(err)
		}
	}()
	//registering it as the global OpenTelemetry TracerProvider.
	otel.SetTracerProvider(tp)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	errCh := make(chan error)
	app := fib.NewApp(os.Stdin, l)
	go func() {
		errCh <- app.Run(context.Background())
	}()

	//create http
	go func() {
		http.StartHttp()
	}()

	select {
	case <-sigCh:
		l.Println("\ngoodbye")
		return
	case err := <-errCh:
		if err != nil {
			l.Fatal(err)
		}
	}
}

// newExporter returns a console exporter.
// The SDK connects telemetry from the OpenTelemetry API to exporters.
// Exporters are packages that allow telemetry data to be emitted somewhere - either to the console (which is what we’re doing here),
// or to a remote system or collector for further analysis and/or enrichment
// OpenTelemetry supports a variety of exporters through its ecosystem including popular open source tools like Jaeger, Zipkin, and Prometheus.
func newExporter(w io.Writer) (trace.SpanExporter, error) {
	return stdouttrace.New(
		stdouttrace.WithWriter(w),
		// Use human-readable output.
		stdouttrace.WithPrettyPrint(),
		// Do not print timestamps for the demo.
		stdouttrace.WithoutTimestamps(),
	)
}

// newResource returns a resource describing this application.
// The catch is, you need a way to identify what service, or even what service instance, that data is coming from.
// OpenTelemetry uses a Resource to represent the entity producing telemetry.
func newResource() *resource.Resource {
	r, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("fib"),
			semconv.ServiceVersionKey.String("v0.1.0"),
			attribute.String("environment", "demo"),
		),
	)
	return r
}
