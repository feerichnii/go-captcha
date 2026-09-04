/**
 * Challenge helpers: seal captcha answers with AEAD so stored/transported
 * tokens never expose GetData() coordinates/angles in plaintext.
 **/

package challenge

import (
	"crypto/aes"
	"crypto/cipher"
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
	ErrBadMAC       = errors.New("challenge: authentication failed")
	ErrEmptyKey     = errors.New("challenge: empty key")
)

const tokenVersion = "v2"

// Payload is the server-only answer blob sealed into a token.
type Payload struct {
	// ID binds the payload to a challenge id.
	ID string `json:"id,omitempty"`
	// Kind is click | slide | rotate
	Kind string `json:"kind"`
	// Data is a JSON encoding of the captcha answer (GetData()).
	Data json.RawMessage `json:"data"`
	// Exp is unix expiry seconds; 0 means no expiry in the token itself.
	Exp int64 `json:"exp,omitempty"`
}

// NewID returns a cryptographically random challenge id (hex, 32 chars).
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// IsValidID reports whether s looks like an id produced by NewID.
func IsValidID(s string) bool {
	if len(s) != 32 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// deriveKey turns any non-empty secret into a 32-byte AES-256 key.
func deriveKey(key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, ErrEmptyKey
	}
	sum := sha256.Sum256(append([]byte("gocaptcha-aead-v2:"), key...))
	return sum[:], nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	k, err := deriveKey(key)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Encrypt seals plaintext with AES-256-GCM. aad is authenticated but not encrypted
// (use it to bind ciphertext to a challenge id). Output: nonce || ciphertext.
func Encrypt(key, plaintext, aad []byte) ([]byte, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(nonce)+len(plaintext)+aead.Overhead())
	out = append(out, nonce...)
	return aead.Seal(out, nonce, plaintext, aad), nil
}

// Decrypt opens data produced by Encrypt with the same key and aad.
func Decrypt(key, data, aad []byte) ([]byte, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	ns := aead.NonceSize()
	if len(data) < ns+aead.Overhead() {
		return nil, ErrInvalidToken
	}
	pt, err := aead.Open(nil, data[:ns], data[ns:], aad)
	if err != nil {
		return nil, ErrBadMAC
	}
	return pt, nil
}

// Seal encrypts payload and returns an opaque token: "v2.<base64url(nonce||ct)>".
// The answer inside is confidential — safe to store, and safe to hand to a
// client only as an opaque blob (it cannot read or forge it).
func Seal(key []byte, p Payload) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	ct, err := Encrypt(key, raw, []byte(p.ID))
	if err != nil {
		return "", err
	}
	return tokenVersion + "." + base64.RawURLEncoding.EncodeToString(ct), nil
}

// SealWithTTL sets Exp = now+ttl and seals.
func SealWithTTL(key []byte, p Payload, ttl time.Duration) (string, error) {
	if ttl > 0 {
		p.Exp = time.Now().Add(ttl).Unix()
	}
	return Seal(key, p)
}

// Open decrypts and authenticates a token. If expectID is non-empty, the
// payload must be bound to that challenge id.
func Open(key []byte, token string, expectID string) (Payload, error) {
	var zero Payload
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || parts[0] != tokenVersion {
		return zero, ErrInvalidToken
	}
	ct, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return zero, ErrInvalidToken
	}
	raw, err := Decrypt(key, ct, []byte(expectID))
	if err != nil {
		return zero, err
	}
	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return zero, ErrInvalidToken
	}
	if expectID != "" && p.ID != expectID {
		return zero, ErrBadMAC
	}
	if p.Exp > 0 && time.Now().Unix() > p.Exp {
		return zero, ErrExpired
	}
	return p, nil
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

// ParseInt is a tiny helper for apps parsing click coords from forms.
func ParseInt(s string) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("challenge: bad int %q: %w", s, err)
	}
	return v, nil
}
