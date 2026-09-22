package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type application struct {
	db *pgxpool.Pool
}

func (app *application) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := app.db.Ping(ctx); err != nil {
		log.Printf("database health check failed: %v", err)

		w.WriteHeader(http.StatusServiceUnavailable)

		if _, writeErr := w.Write([]byte(`{"status":"unavailable","database":"unavailable"}`)); writeErr != nil {
			log.Printf("failed to write health response: %v", writeErr)
		}

		return
	}

	w.WriteHeader(http.StatusOK)

	if _, err := w.Write([]byte(`{"status":"ok","database":"ok"}`)); err != nil {
		log.Printf("failed to write health response: %v", err)
	}
}

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	startupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := pgxpool.New(startupCtx, databaseURL)
	if err != nil {
		log.Fatal("failed to create database pool:", err)
	}
	defer db.Close()

	if err := db.Ping(startupCtx); err != nil {
		log.Fatal("failed to connect to database:", err)
	}

	log.Println("connected to PostgreSQL")

	app := &application{
		db: db,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", app.healthHandler)

	log.Println("server listening on http://localhost:8080")

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}