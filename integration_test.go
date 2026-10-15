package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Use a separate schema so this test never changes existing short links.
func TestPostgresCreateShortLink(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	suffix, err := generateShortCode()
	if err != nil {
		t.Fatal(err)
	}
	schema := "shortener_test_" + strings.ToLower(suffix)
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("clean test schema: %v", err)
		}
	}()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migration, err := os.ReadFile("migrations/000001_create_urls.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}

	app := &application{db: db}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(`{"url":"https://example.com/integration"}`))
	app.shortenHandler(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	var result shortenResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	var savedURL string
	if err := db.QueryRow(ctx, "SELECT original_url FROM urls WHERE short_code = $1", result.Code).Scan(&savedURL); err != nil {
		t.Fatal(err)
	}
	if savedURL != "https://example.com/integration" {
		t.Fatalf("saved URL = %q", savedURL)
	}

	// Force a real unique violation, then check that the next code is saved.
	attempts := 0
	code, err := createShortLink(ctx, db, "https://example.com/retry", func() (string, error) {
		attempts++
		if attempts == 1 {
			return result.Code, nil
		}
		return generateShortCode()
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || code == result.Code {
		t.Fatal("expected a fresh code after a collision")
	}
	if err := db.QueryRow(ctx, "SELECT original_url FROM urls WHERE short_code = $1", code).Scan(&savedURL); err != nil {
		t.Fatal(err)
	}
	if savedURL != "https://example.com/retry" {
		t.Fatalf("saved retry URL = %q", savedURL)
	}
}
