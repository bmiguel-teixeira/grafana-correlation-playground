package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	myotel "app2/internal/otel"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	OTEL_SPAN_HEADER = "x-otel-span-id"
)

type app2 struct {
	HttpClient *http.Client
	otc        *myotel.OtelClient
}

func (a *app2) GetBook(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	traceId := r.Header.Get(myotel.OTEL_TRACE_HEADER)
	spanId := r.Header.Get(myotel.OTEL_SPAN_HEADER)

	traceID, _ := trace.TraceIDFromHex(traceId)
	spanID, _ := trace.SpanIDFromHex(spanId)
	parentSpanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), parentSpanContext)

	tracer := a.otc.Tracer.Tracer("opentelemetry.io/sdk")
	_, span := tracer.Start(
		ctx,
		"/available",
		trace.WithAttributes(
			attribute.String("hostname", "locahost"),
		),
	)
	defer span.End()
	time.Sleep(200 * time.Millisecond)

	elapsed := time.Since(start)
	a.otc.Logger.Info(
		fmt.Sprintf("Validation for book succeded in %d miliseconds", elapsed.Milliseconds()),
		slog.String("TraceId", traceID.String()),
		slog.String("SpanId", span.SpanContext().TraceID().String()),
	)

	a.otc.HttpRequestTotalMeter.Add(a.otc.Ctx, 1, metric.WithAttributes(
		attribute.String("method", "GET"),
		attribute.String("path", "/validate"),
		attribute.String("code", "200"),
	))

	io.WriteString(w, "GOOD!")
}

func main() {
	ctx := context.TODO()
	otelClient, err := myotel.NewOtelClient(
		ctx,
		"localhost:14317",
		semconv.ServiceNameKey.String("app2"),
		attribute.String("version", "1.0.0"),
	)
	if err != nil {
		panic(err)
	}

	app2 := app2{
		otc: otelClient,
	}
	http.HandleFunc("/available", app2.GetBook)
	err = http.ListenAndServe(":8082", nil)
	if err != nil {
		panic(err)
	}
}
