package main

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type fakeDatabase struct {
	calls  int
	errors []error
	args   []any
}

func (db *fakeDatabase) Ping(context.Context) error { return nil }

func (db *fakeDatabase) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	db.args = args
	db.calls++
	if db.calls <= len(db.errors) && db.errors[db.calls-1] != nil {
		return pgconn.CommandTag{}, db.errors[db.calls-1]
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func TestCreateShortLink(t *testing.T) {
	collision := &pgconn.PgError{Code: "23505", ConstraintName: "urls_short_code_key"}
	for _, tc := range []struct {
		name      string
		errors    []error
		wantCalls int
		wantError bool
	}{
		{"success", nil, 1, false},
		{"collision then success", []error{collision}, 2, false},
		{"retry limit", []error{collision, collision, collision, collision, collision}, 5, true},
		{"database failure", []error{errors.New("connection lost")}, 1, true},
		{"unrelated unique violation", []error{&pgconn.PgError{Code: "23505", ConstraintName: "urls_pkey"}}, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeDatabase{errors: tc.errors}
			generated := 0
			code, err := createShortLink(context.Background(), db, "https://example.com/?q='", func() (string, error) {
				generated++
				if generated == 1 {
					return "aaaaaaaa", nil
				}
				return "bbbbbbbb", nil
			})
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v", err)
			}
			if db.calls != tc.wantCalls || generated != tc.wantCalls {
				t.Fatalf("inserts=%d, generated=%d; want %d", db.calls, generated, tc.wantCalls)
			}
			if !tc.wantError && (db.args[0] != code || db.args[1] != "https://example.com/?q='") {
				t.Fatalf("wrong saved mapping: %v", db.args)
			}
			if tc.name == "collision then success" && code != "bbbbbbbb" {
				t.Fatal("collision did not generate a new code")
			}
		})
	}
}

func TestCreateShortLinkGenerationFailure(t *testing.T) {
	db := &fakeDatabase{}
	_, err := createShortLink(context.Background(), db, "https://example.com", func() (string, error) {
		return "", errors.New("random source failed")
	})
	if err == nil || db.calls != 0 {
		t.Fatal("generation failure must not insert")
	}
}

func TestCreateShortLinkCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	db := &fakeDatabase{}
	_, err := createShortLink(ctx, db, "https://example.com", generateShortCode)
	if !errors.Is(err, context.Canceled) || db.calls != 0 {
		t.Fatal("cancelled request must not insert")
	}
}
