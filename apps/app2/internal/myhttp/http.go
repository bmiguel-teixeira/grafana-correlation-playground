package myhttp

import (
	myotel "app2/internal/otel"
	"net/http"
	"time"
)

type MyHttpClient struct {
}

func NewHttpClient(otc *myotel.OtelClient) (*http.Client, error) {
	return &http.Client{
		Transport: otc,
		Timeout:   4 * time.Second,
	}, nil
}
