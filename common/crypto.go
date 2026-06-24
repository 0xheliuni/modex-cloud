// Package common holds small shared utilities. The password and token helpers
// here are lifted from new-api (common/crypto.go, common/utils.go) so behavior
// matches the reference implementation we're modeling.
package common

import (
	crand "crypto/rand"
	"math/big"

	"golang.org/x/crypto/bcrypt"
)

// Password2Hash hashes a plaintext password with bcrypt (default cost).
// Lifted from new-api common/crypto.go.
func Password2Hash(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hashed), err
}

// ValidatePasswordAndHash reports whether password matches the bcrypt hash.
func ValidatePasswordAndHash(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

const keyChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// GenerateRandomCharsKey returns a cryptographically-random alphanumeric string
// of the given length. Lifted from new-api common/utils.go.
func GenerateRandomCharsKey(length int) (string, error) {
	b := make([]byte, length)
	maxI := big.NewInt(int64(len(keyChars)))
	for i := range b {
		n, err := crand.Int(crand.Reader, maxI)
		if err != nil {
			return "", err
		}
		b[i] = keyChars[n.Int64()]
	}
	return string(b), nil
}

// GenerateAccessToken returns a 48-char random token for API authentication.
func GenerateAccessToken() (string, error) {
	return GenerateRandomCharsKey(48)
}
