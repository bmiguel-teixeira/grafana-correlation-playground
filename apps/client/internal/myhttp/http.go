package myhttp

import (
	myotel "client/internal/otel"
	"fmt"
	"io/ioutil"
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

func Get(url string, client *http.Client) {
	start := time.Now()
	resp, http_err := client.Get(url)

	if http_err != nil {
		fmt.Printf("Error: %v\n", http_err)
		return
	}

	//Read now to capture time to read full response, not just headers
	_, read_err := ioutil.ReadAll(resp.Body)
	elapsed := time.Since(start)

	if resp != nil {
		defer resp.Body.Close()
	}

	if read_err != nil {
		fmt.Printf("Error: %v, Time taken: %v\n", read_err, elapsed)
		return
	}

	fmt.Printf("Status: %v, Time taken: %v\n", resp.Status, elapsed)
}
