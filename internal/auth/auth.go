// Package auth holds credential hashing and token generation. Passwords are
// verified in SQL via pgcrypto's crypt() (see store); this package covers bot
// API keys and human session tokens, both stored as SHA-256 hex.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// NewSessionToken returns a fresh random token and the hash to store for it.
func NewSessionToken() (plain, hash string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failing means the host is broken
	}
	plain = "apssess_" + hex.EncodeToString(b)
	return plain, HashSecret(plain)
}

// HashSecret hashes an API key or session token for storage/lookup.
func HashSecret(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
