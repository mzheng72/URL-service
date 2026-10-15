package main

import (
	"strings"
	"testing"
)

func TestGenerateShortCode(t *testing.T) {
	// Run multiple times because the function produces random output.
	for range 100 {
		code, err := generateShortCode()
		if err != nil {
			t.Fatalf("generateShortCode() returned an error: %v", err)
		}

		if len(code) != shortCodeLength {
			t.Errorf(
				"generateShortCode() length = %d, want %d",
				len(code),
				shortCodeLength,
			)
		}

		for _, character := range code {
			if !strings.ContainsRune(base62Alphabet, character) {
				t.Errorf(
					"generateShortCode() returned invalid character %q",
					character,
				)
			}
		}
	}
}
