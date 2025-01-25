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

	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}

	elapsed := time.Since(start)
	otc.Logger.Info(
		fmt.Sprintf("Request: %s %s in %d miliseconds", req.Method, req.URL.Path, elapsed.Milliseconds()),
	)
	fmt.Println(elapsed)

	otc.HttpRequestTotalMeter.Add(otc.Ctx, 1, metric.WithAttributes(
		attribute.String("method", req.Method),
		attribute.String("path", req.URL.Path),
		attribute.String("code", resp.Status),
	))
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

	batchSpanProcessor := sdktrace.NewBatchSpanProcessor(exporter)
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
			log.NewBatchProcessor(logExporter),
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

/*
func exceptionFunction(ctx context.Context, tracer trace.Tracer) {
	ctx, exceptionSpan := tracer.Start(
		ctx,
		"exceptionSpanName",
		trace.WithAttributes(attribute.String("exceptionAttributeKey1", "exceptionAttributeValue1")))
	defer exceptionSpan.End()
	log.Printf("Call division function.")
	_, err := divide(10, 0)
	if err != nil {
		exceptionSpan.RecordError(err)
		exceptionSpan.SetStatus(codes.Error, err.Error())
	}
}
*/
