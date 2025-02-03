package otel

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/log"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	OTEL_TRACE_HEADER = "x-otel-custom-id"
	OTEL_SPAN_HEADER  = "x-otel-span-id"
)

type OtelClient struct {
	Ctx                   context.Context
	Tracer                *sdktrace.TracerProvider
	Metrics               *metricsdk.MeterProvider
	HttpRequestTotalMeter metric.Int64Counter
	Logger                *slog.Logger
}

func (otc *OtelClient) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	tracer := otc.Tracer.Tracer("opentelemetry.io/sdk")
	parentId := req.Header.Get(OTEL_TRACE_HEADER)
	spanId := req.Header.Get(OTEL_SPAN_HEADER)

	traceID, _ := trace.TraceIDFromHex(parentId)
	spanID, _ := trace.SpanIDFromHex(spanId)
	parentSpanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), parentSpanContext)

	_, span := tracer.Start(
		ctx,
		fmt.Sprintf("%s %s", req.Method, req.URL.Path),
		trace.WithAttributes(
			attribute.String("hostname", req.Host),
		),
	)
	defer span.End()

	req.Header.Set(OTEL_TRACE_HEADER, parentId)
	req.Header.Set(OTEL_SPAN_HEADER, span.SpanContext().SpanID().String())

	resp, err := http.DefaultTransport.RoundTrip(req)
	elapsed := time.Since(start)
	status := "-1"
	if resp != nil {
		status = fmt.Sprintf("%d", resp.StatusCode)
	}
	otc.HttpRequestTotalMeter.Add(otc.Ctx, 1, metric.WithAttributes(
		attribute.String("method", req.Method),
		attribute.String("path", req.URL.Path),
		attribute.String("code", status),
	))

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		otc.Logger.Error(
			fmt.Sprintf("Request for [%s] failed in %d miliseconds", req.URL.Path, elapsed.Milliseconds()),
			slog.String("TraceId", parentId),
			slog.String("SpanId", span.SpanContext().TraceID().String()),
		)
		return nil, err

	}

	if status != "200" {
		span.SetStatus(codes.Error, fmt.Sprintf("Server returned [%d]", resp.StatusCode))
		otc.Logger.Error(
			fmt.Sprintf("Request for [%s] failed in %d miliseconds", req.URL.Path, elapsed.Milliseconds()),
			slog.String("TraceId", parentId),
			slog.String("SpanId", span.SpanContext().TraceID().String()),
		)
		return nil, err
	}

	otc.Logger.Info(
		fmt.Sprintf("Request for [%s] succeded in %d miliseconds", req.URL.Path, elapsed.Milliseconds()),
		slog.String("TraceId", parentId),
		slog.String("SpanId", span.SpanContext().TraceID().String()),
	)
	return resp, err
}

func NewOtelClient(ctx context.Context, collectorUrl string, attr ...attribute.KeyValue) (*OtelClient, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(attr...),
	)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.DialContext(ctx, collectorUrl, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, err
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, err
	}

	metricsExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint("collector:14317"),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("could not create metric exporter: %w", err)
	}
	logExporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint("collector:14317"),
		otlploggrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("could not create metric exporter: %w", err)
	}

	periodicReader := metricsdk.NewPeriodicReader(metricsExporter, metricsdk.WithInterval(5*time.Second))

	batchSpanProcessor := sdktrace.NewSimpleSpanProcessor(exporter)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(batchSpanProcessor),
	)
	metricsProvider := metricsdk.NewMeterProvider(
		metricsdk.WithResource(res),
		metricsdk.WithReader(periodicReader),
	)
	metricsProvider.Meter("tracetest")

	lp := log.NewLoggerProvider(
		log.WithProcessor(
			log.NewSimpleProcessor(logExporter),
		),
		log.WithResource(res),
	)
	global.SetLoggerProvider(lp)
	logger := otelslog.NewLogger("asd")
	logger.Info("Logger started")

	otel.SetTracerProvider(tracerProvider)

	c, err := metricsProvider.Meter("asdsda").Int64Counter("http.requests.total")
	if err != nil {
		return nil, err
	}
	return &OtelClient{
		Ctx:                   ctx,
		Tracer:                tracerProvider,
		Metrics:               metricsProvider,
		HttpRequestTotalMeter: c,
		Logger:                logger,
	}, nil
}
