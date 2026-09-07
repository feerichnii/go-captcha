package antibot

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// GenerateSecretKey returns n cryptographically random bytes suitable for
// Config.SecretKey (n defaults to 32; minimum MinSecretKeyLen).
func GenerateSecretKey(n int) ([]byte, error) {
	if n < MinSecretKeyLen {
		n = MinSecretKeyLen
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("antibot: crypto/rand failed: %w", err)
	}
	if err := ValidateSecretKey(b); err != nil {
		// astronomically unlikely with crypto/rand — retry once
		if _, err2 := rand.Read(b); err2 != nil {
			return nil, fmt.Errorf("antibot: crypto/rand failed: %w", err2)
		}
		if err := ValidateSecretKey(b); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// MustReadCrypto fills b from crypto/rand or panics (fail-closed).
func MustReadCrypto(b []byte) {
	if _, err := rand.Read(b); err != nil {
		panic("antibot: crypto/rand failed: " + err.Error())
	}
}

// PrivacyHash returns a 128-bit hex HMAC for storing identifiers with limited TTL.
// domain separates namespaces, e.g. "ip", "fp:canvas", "device".
func PrivacyHash(secret []byte, domain, value string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(domain))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(value))
	sum := mac.Sum(nil)
	return hex.EncodeToString(sum[:16])
}
