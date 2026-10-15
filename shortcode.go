package main

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	base62Alphabet  = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	shortCodeLength = 8
)

// generateShortCode creates a random Base62 code.
func generateShortCode() (string, error) {
	code := make([]byte, shortCodeLength)
	alphabetLength := big.NewInt(int64(len(base62Alphabet)))

	for i := range code {
		randomIndex, err := rand.Int(rand.Reader, alphabetLength)
		if err != nil {
			return "", fmt.Errorf("generate random index: %w", err)
		}

		code[i] = base62Alphabet[randomIndex.Int64()]
	}

	return string(code), nil
}
