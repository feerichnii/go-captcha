/**
 * Challenge helpers: bind captcha answers to an opaque server-side token
 * so clients never receive raw GetData() coordinates/angles.
 **/

package challenge

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("challenge: invalid token")
	ErrExpired      = errors.New("challenge: expired")
	ErrBadMAC       = errors.New("challenge: bad mac")
)

// Payload is the server-only answer blob sealed into a token.
type Payload struct {
	// Kind is click | slide | rotate
	Kind string `json:"kind"`
	// Data is a JSON encoding of the captcha answer (GetData()).
	Data json.RawMessage `json:"data"`
	// Exp is unix expiry seconds; 0 means no expiry in the token itself.
	Exp int64 `json:"exp,omitempty"`
}

// NewID returns a cryptographically random challenge id (hex).
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Seal HMAC-signs payload with key and returns opaque token: base64url(payload).base64url(mac)
func Seal(key []byte, p Payload) (string, error) {
	if len(key) == 0 {
		return "", errors.New("challenge: empty key")
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(raw)
	sum := mac.Sum(nil)

	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(sum), nil
}

// SealWithTTL sets Exp = now+ttl and seals.
func SealWithTTL(key []byte, p Payload, ttl time.Duration) (string, error) {
	if ttl > 0 {
		p.Exp = time.Now().Add(ttl).Unix()
	}
	return Seal(key, p)
}

// Open verifies MAC and optional Exp, returning the payload.
func Open(key []byte, token string) (Payload, error) {
	var zero Payload
	if len(key) == 0 {
		return zero, errors.New("challenge: empty key")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return zero, ErrInvalidToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return zero, ErrInvalidToken
	}
	sum, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return zero, ErrInvalidToken
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(raw)
	if !hmac.Equal(sum, mac.Sum(nil)) {
		return zero, ErrBadMAC
	}
	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return zero, ErrInvalidToken
	}
	if p.Exp > 0 && time.Now().Unix() > p.Exp {
		return zero, ErrExpired
	}
	return p, nil
}

// FormatAttemptKey builds a storage key for attempt counters.
func FormatAttemptKey(challengeID string) string {
	return "gocaptcha:attempts:" + challengeID
}

// ClampPadding returns padding limited to [0, max].
func ClampPadding(padding, max int) int {
	if max < 0 {
		max = 0
	}
	if padding < 0 {
		return 0
	}
	if padding > max {
		return max
	}
	return padding
}

// MustParseInt is a tiny helper for apps parsing click coords from forms.
func MustParseInt(s string) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("challenge: bad int %q: %w", s, err)
	}
	return v, nil
}
