package main

import (
	"context"
	"fmt"

	myotel "client/internal/otel"
	"client/myhttp"

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

	httpClient, err := myhttp.NewHttpClient(otelClient)
	if err != nil {
		panic(err)
	}
	httpClient.Get("https://jn.pt")

	fmt.Println(otelClient)
}
