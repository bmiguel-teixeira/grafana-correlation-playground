package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	myotel "app1/internal/otel"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

type App1 struct {
	HttpClient *http.Client
	OtcClient  *myotel.OtelClient
}

var (
	APP2_URL = "http://app2:8082/available"
	APP3_URL = "http://app3:8083/reserve"
)

func (a *App1) GetBook(w http.ResponseWriter, r *http.Request) {
	traceId := r.Header.Get(myotel.OTEL_TRACE_HEADER)
	spanId := r.Header.Get(myotel.OTEL_SPAN_HEADER)
	time.Sleep(100 * time.Millisecond)

	// request to apps 2
	req2, err := http.NewRequest("GET", APP2_URL, nil)
	req2.Header.Set(myotel.OTEL_TRACE_HEADER, traceId)
	req2.Header.Set(myotel.OTEL_SPAN_HEADER, spanId)
	resp2, err2 := a.HttpClient.Do(req2)

	// request to apps 3
	req3, err := http.NewRequest("GET", APP3_URL, nil)
	req3.Header.Set(myotel.OTEL_TRACE_HEADER, traceId)
	req3.Header.Set(myotel.OTEL_SPAN_HEADER, spanId)
	resp3, err3 := a.HttpClient.Do(req3)

	if err != nil || err2 != nil || err3 != nil || resp2.StatusCode != 200 || resp3.StatusCode != 200 {
		w.WriteHeader(http.StatusInternalServerError)
		combinedErrors := fmt.Sprintf("%s\n%s\n%s\n", err, err2, err3)
		io.WriteString(w, combinedErrors)
		return
	} else {
		io.WriteString(w, "GOOD!")
		return
	}
}

func main() {
	fmt.Println("Starting app")
	ctx := context.TODO()
	otelClient, err := myotel.NewOtelClient(
		ctx,
		"collector:14317",
		semconv.ServiceNameKey.String("app1"),
		attribute.String("version", "1.0.0"),
	)
	if err != nil {
		panic(err)
	}

	app1 := App1{
		HttpClient: &http.Client{
			Transport: otelClient,
		},
		OtcClient: otelClient,
	}
	http.HandleFunc("/reserve", app1.GetBook)
	err = http.ListenAndServe(":8081", nil)
	if err != nil {
		panic(err)
	}
}
