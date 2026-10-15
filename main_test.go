package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestShortenHandler(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		dbError error
		status  int
		calls   int
	}{
		{"valid", `{"url":" https://example.com/path "}`, nil, 201, 1},
		{"bad JSON", `not-json`, nil, 400, 0},
		{"missing URL", `{}`, nil, 400, 0},
		{"null", `null`, nil, 400, 0},
		{"unknown field", `{"urls":"https://example.com"}`, nil, 400, 0},
		{"bad URL", `{"url":"hello"}`, nil, 400, 0},
		{"wrong scheme", `{"url":"ftp://example.com"}`, nil, 400, 0},
		{"missing hostname", `{"url":"http://:80"}`, nil, 400, 0},
		{"extra JSON", `{"url":"https://example.com"} {}`, nil, 400, 0},
		{"extra text", `{"url":"https://example.com"} garbage`, nil, 400, 0},
		{"large body", `{"url":"https://example.com/` + strings.Repeat("a", 17000) + `"}`, nil, 400, 0},
		{"database failure", `{"url":"https://example.com"}`, errors.New("private connection detail"), 500, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeDatabase{errors: []error{tc.dbError}}
			app := &application{db: db}
			mux := http.NewServeMux()
			mux.HandleFunc("POST /shorten", app.shortenHandler)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(tc.body)))
			if response.Code != tc.status || db.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, db.calls, response.Body)
			}
			if response.Header().Get("Content-Type") != "application/json" {
				t.Fatal("expected JSON")
			}
			if tc.status == 201 {
				var result shortenResponse
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if len(result.Code) != 8 || result.Code != db.args[0] || result.OriginalURL != "https://example.com/path" || result.OriginalURL != db.args[1] {
					t.Fatalf("wrong response: %+v", result)
				}
			} else if strings.Contains(response.Body.String(), "private connection detail") {
				t.Fatal("database details leaked")
			}
		})
	}
}
