package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// database allows tests to replace PostgreSQL with a small fake.
type database interface {
	Ping(context.Context) error
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// application stores resources shared by HTTP handlers.
type application struct {
	db database
}

// shortenRequest describes the JSON sent by the client.
type shortenRequest struct {
	URL string `json:"url"`
}

// shortenResponse describes the JSON returned to the client.
type shortenResponse struct {
	Code        string `json:"code"`
	OriginalURL string `json:"original_url"`
}

// errorResponse gives all API errors the same JSON format.
type errorResponse struct {
	Error string `json:"error"`
}

// writeJSON converts a Go value to JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, data any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	return json.NewEncoder(w).Encode(data)
}

// validURL checks that the value is a complete HTTP or HTTPS URL.
func validURL(rawURL string) bool {
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return false
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false
	}

	return parsedURL.Hostname() != ""
}

func (app *application) healthHandler(w http.ResponseWriter, r *http.Request) {
	// Stop waiting if the database does not respond within two seconds.
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := app.db.Ping(ctx); err != nil {
		log.Printf("database health check failed: %v", err)

		response := errorResponse{
			Error: "database unavailable",
		}

		if writeErr := writeJSON(w, http.StatusServiceUnavailable, response); writeErr != nil {
			log.Printf("failed to write health response: %v", writeErr)
		}

		return
	}

	response := map[string]string{
		"status":   "ok",
		"database": "ok",
	}

	if err := writeJSON(w, http.StatusOK, response); err != nil {
		log.Printf("failed to write health response: %v", err)
	}
}

func (app *application) shortenHandler(w http.ResponseWriter, r *http.Request) {
	var input shortenRequest

	// Decode the request body from JSON into a Go struct.
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		response := errorResponse{
			Error: "request body must be valid JSON",
		}

		if writeErr := writeJSON(w, http.StatusBadRequest, response); writeErr != nil {
			log.Printf("failed to write error response: %v", writeErr)
		}

		return
	}

	// Accept exactly one JSON value, with no extra data after it.
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if writeErr := writeJSON(w, http.StatusBadRequest, errorResponse{Error: "request body must contain one JSON object"}); writeErr != nil {
			log.Printf("failed to write error response: %v", writeErr)
		}
		return
	}

	// Remove accidental spaces before validating the URL.
	input.URL = strings.TrimSpace(input.URL)

	if !validURL(input.URL) {
		response := errorResponse{
			Error: "url must be a valid HTTP or HTTPS URL",
		}

		if writeErr := writeJSON(w, http.StatusBadRequest, response); writeErr != nil {
			log.Printf("failed to write error response: %v", writeErr)
		}

		return
	}

	// Limit the total time spent inserting and retrying.
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	code, err := createShortLink(ctx, app.db, input.URL, generateShortCode)
	if err != nil {
		log.Printf("failed to create short link: %v", err)
		if writeErr := writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "could not create short link"}); writeErr != nil {
			log.Printf("failed to write error response: %v", writeErr)
		}
		return
	}

	// Report success only after PostgreSQL has saved the mapping.
	response := shortenResponse{
		Code:        code,
		OriginalURL: input.URL,
	}

	if err := writeJSON(w, http.StatusCreated, response); err != nil {
		log.Printf("failed to write shorten response: %v", err)
	}
}

func main() {
	// Read the database connection string from the environment.
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	// Limit the initial database connection attempt to five seconds.
	startupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a connection pool that can be shared by concurrent requests.
	db, err := pgxpool.New(startupCtx, databaseURL)
	if err != nil {
		log.Fatal("failed to create database pool:", err)
	}
	defer db.Close()

	// Confirm that PostgreSQL is reachable before starting the server.
	if err := db.Ping(startupCtx); err != nil {
		log.Fatal("failed to connect to database:", err)
	}

	log.Println("connected to PostgreSQL")

	app := &application{
		db: db,
	}

	// Register each HTTP method and path with its handler.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", app.healthHandler)
	mux.HandleFunc("POST /shorten", app.shortenHandler)

	log.Println("server listening on http://localhost:8080")

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
