package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

const maxCodeAttempts = 5

// createShortLink retries only collisions on the short-code constraint.
func createShortLink(ctx context.Context, db database, originalURL string, generate func() (string, error)) (string, error) {
	for attempt := 0; attempt < maxCodeAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		code, err := generate()
		if err != nil {
			return "", fmt.Errorf("generate short code: %w", err)
		}

		// Parameters keep user input separate from SQL instructions.
		_, err = db.Exec(ctx,
			"INSERT INTO urls (short_code, original_url) VALUES ($1, $2)",
			code, originalURL,
		)
		if err == nil {
			return code, nil
		}

		// PostgreSQL, not a separate lookup, guarantees uniqueness.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "urls_short_code_key" {
			continue
		}
		return "", fmt.Errorf("insert short link: %w", err)
	}
	return "", fmt.Errorf("short code collisions after %d attempts", maxCodeAttempts)
}
