package main

import (
	myotel "app3/internal/otel"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

type LibraryClient struct {
	DbClient   *sql.DB
	OtelClient *myotel.OtelClient
}

var (
	BROKEN = false
)

const (
	CONN_STRING = "host=localhost port=5432 user=app3 password=S3cret dbname=library sslmode=disable"
)

func runRawQuery(db *sql.DB, query string) (*sql.Rows, *pq.Error, error) {
	rows, err := db.Query(query)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) {
			return nil, pqErr, nil
		} else {
			return nil, nil, err
		}
	}
	return rows, nil, nil
}

func queryDB(db *sql.DB, otc *myotel.OtelClient, parentId string) (int, error) {
	QUERY := "SELECT * FROM books"
	start := time.Now()
	tracer := otc.Tracer.Tracer("opentelemetry.io/sdk")
	_, span := tracer.Start(
		otc.Ctx,
		"POSTGRESQL",
		trace.WithAttributes(
			attribute.String("db", "/check-reservation"),
			attribute.String("query", QUERY),
		),
	)
	traceID, _ := trace.TraceIDFromHex(parentId)
	parentSpanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     span.SpanContext().SpanID(),
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	otc.Ctx = trace.ContextWithSpanContext(otc.Ctx, parentSpanContext)
	defer span.End()

	if BROKEN {
		errorMsg := fmt.Errorf("too many open connections")
		span.SetStatus(codes.Error, errorMsg.Error())
		otc.PostgreSqlQueriesTotal.Add(otc.Ctx, 1, metric.WithAttributes(
			attribute.String("type", "select"),
			attribute.String("status", "failed"),
		))
		otc.Logger.Error(
			fmt.Sprintf("Database query [%s] failed in %d miliseconds with [%s]", QUERY, time.Since(start), errorMsg),
			slog.String("TraceId", parentId),
			slog.String("SpanId", span.SpanContext().TraceID().String()),
		)
		return 0, errorMsg
	}

	time.Sleep(300 * time.Millisecond)

	rows, pgErr, err := runRawQuery(db, QUERY)
	if pgErr != nil || err != nil {
		span.SetStatus(codes.Error, err.Error())
		otc.PostgreSqlQueriesTotal.Add(otc.Ctx, 1, metric.WithAttributes(
			attribute.String("type", "select"),
			attribute.String("status", "failed"),
		))
		return -1, err
	}
	defer rows.Close()

	// Iterate through results
	i := 0
	for rows.Next() {
		i++
	}

	elapsed := time.Since(start)
	if err != nil || BROKEN {
		otc.Logger.Error(
			fmt.Sprintf("Database query [%s] failed in %d miliseconds", QUERY, elapsed.Milliseconds()),
			slog.String("TraceId", parentId),
			slog.String("SpanId", span.SpanContext().TraceID().String()),
		)
	} else {
		otc.Logger.Info(
			fmt.Sprintf("Database query [%s] succeded in %d miliseconds", QUERY, elapsed.Milliseconds()),
			slog.String("TraceId", parentId),
			slog.String("SpanId", span.SpanContext().TraceID().String()),
		)
	}

	otc.PostgreSqlQueriesTotal.Add(otc.Ctx, 1, metric.WithAttributes(
		attribute.String("type", "select"),
		attribute.String("status", "success"),
	))

	return i, nil
}

func setupDB(db *sql.DB) error {
	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	fmt.Println("Connected to the database successfully!")

	// setup database
	_, qErr, err := runRawQuery(db, `
	CREATE TABLE books (
		name VARCHAR(255),
		author VARCHAR(255),
		year INT
		); 
	`)
	if err != nil {
		return err
	}
	if qErr != nil && qErr.Code == "42P07" {
		fmt.Println("DB already setup all good")
	}

	// setup database
	_, qErr, err = runRawQuery(db, `
		INSERT INTO books (name, author, year)
		VALUES ('Harry Potter', 'J.K. Rowling', 1997);`)
	if qErr != nil || err != nil {
		return err
	}

	return nil
}

func (l *LibraryClient) GetBook(w http.ResponseWriter, r *http.Request) {
	count, err := queryDB(l.DbClient, l.OtelClient, r.Header.Get(myotel.OTEL_TRACE_HEADER))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, err.Error())
		return
	} else {
		io.WriteString(w, fmt.Sprintf("{\"books\": %d}", count))
		return
	}
}

func toggleFailure(w http.ResponseWriter, r *http.Request) {
	BROKEN = !BROKEN
	fmt.Println("Toggle switched to: [%s]", BROKEN)
	io.WriteString(w, fmt.Sprintf("%s", BROKEN))
}

func main() {
	ctx := context.TODO()
	otelClient, err := myotel.NewOtelClient(
		ctx,
		"localhost:14317",
		semconv.ServiceNameKey.String("app3"),
		attribute.String("version", "1.0.0"),
	)
	if err != nil {
		panic(err)
	}

	db, err := sql.Open("postgres", CONN_STRING)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = setupDB(db)
	if err != nil {
		panic(err)
	}

	lib := LibraryClient{
		DbClient:   db,
		OtelClient: otelClient,
	}
	http.HandleFunc("/reserve", lib.GetBook)
	http.HandleFunc("/toggle", toggleFailure)

	err = http.ListenAndServe(":8083", nil)
	if err != nil {
		panic(err)
	}
}
