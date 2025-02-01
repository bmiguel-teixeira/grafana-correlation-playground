package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	myotel "client/internal/otel"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

var (
	APP1_URL = "http://localhost:8081/reserve"
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
		_, err := x.Get(APP1_URL)
		if err != nil {
			fmt.Println(err)
			time.Sleep(1 * time.Millisecond)
		}
	}
}
