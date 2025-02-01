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
	_, span := tracer.Start(
		otc.Ctx,
		fmt.Sprintf("%s %s", req.Method, req.URL.Path),
		trace.WithAttributes(
			attribute.String("hostname", req.Host),
		),
	)
	defer span.End()

	parentId := req.Header.Get(OTEL_TRACE_HEADER)
	if parentId == "" {
		req.Header.Set(OTEL_TRACE_HEADER, span.SpanContext().TraceID().String())
	} else {
		traceID, _ := trace.TraceIDFromHex(parentId)
		parentSpanContext := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    traceID,
			SpanID:     span.SpanContext().SpanID(),
			TraceFlags: trace.FlagsSampled,
			Remote:     true,
		})
		otc.Ctx = trace.ContextWithSpanContext(otc.Ctx, parentSpanContext)
	}

	resp, err := http.DefaultTransport.RoundTrip(req)
	elapsed := time.Since(start)
	if err != nil {
		otc.Logger.Error(
			fmt.Sprintf("Validation for book failed in %d miliseconds", elapsed.Milliseconds()),
			slog.String("TraceId", parentId),
			slog.String("SpanId", span.SpanContext().TraceID().String()),
		)
	} else {
		otc.Logger.Info(
			fmt.Sprintf("Validation for book succeded in %d miliseconds", elapsed.Milliseconds()),
			slog.String("TraceId", parentId),
			slog.String("SpanId", span.SpanContext().TraceID().String()),
		)
	}

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
	}

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
		otlpmetricgrpc.WithEndpoint("localhost:14317"),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("could not create metric exporter: %w", err)
	}
	logExporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint("localhost:14317"),
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
