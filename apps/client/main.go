package main

import (
	"context"
	"net/http"
	"time"

	myotel "client/internal/otel"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

func main() {
	ctx := context.TODO()
	otelClient, err := myotel.NewOtelClient(
		ctx,
		"localhost:14317",
		semconv.ServiceNameKey.String("client"),
		attribute.String("version", "1.0.0"),
	)
	if err != nil {
		panic(err)
	}

	x := http.Client{
		Transport: otelClient,
	}
	for {
		x.Get("https://jn.pt/")
		time.Sleep(500 * time.Millisecond)
	}
}
