package antibot

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultSessionCookie is the recommended cookie name for antibot sessions.
	DefaultSessionCookie = "gocaptcha_sid"
	// DefaultSessionTTL for issued session cookies (default 24h).
	DefaultSessionTTL = 24 * time.Hour
	sessionVersion    = 1
)

var (
	// ErrBadSession is returned when a session cookie is missing or forged.
	ErrBadSession = errors.New("antibot: bad session")
	// ErrClientKeyLooksLikeIP rejects ClientKey values that are IP:port / raw IPs.
	// Use a server-issued session id instead; pass IP via ClientSignals.
	ErrClientKeyLooksLikeIP = errors.New("antibot: ClientKey must be a session id, not an IP address")
)

// ClientSignals are optional side-channel inputs for risk scoring.
// They MUST NOT be used as ClientKey — ClientKey is a server-issued session id.
type ClientSignals struct {
	// IP is the client address WITHOUT port (use ParseIP).
	IP string
	// UserAgent is the raw UA string.
	UserAgent string
	// ASN is the origin ASN if known (0 = unknown).
	ASN int
	// SessionIssuedAtMs is when the session cookie was minted (0 = unknown).
	SessionIssuedAtMs int64
}

// Session holds a verified antibot session.
type Session struct {
	ID        string
	IssuedAt  time.Time
	ClientKey string // "sid:<id>" — pass this as IssueRequest/VerifyRequest.ClientKey
}

// LooksLikeIP reports whether s is an IP or IP:port (and therefore unfit as ClientKey).
func LooksLikeIP(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// Strip optional "ip:" / "sid:" prefixes used in older examples.
	if strings.HasPrefix(s, "ip:") {
		return true
	}
	host := s
	if h, _, err := net.SplitHostPort(s); err == nil {
		host = h
	}
	return net.ParseIP(host) != nil
}

// ParseIP extracts the host from an address that may include a port
// (net/http's RemoteAddr is "IP:port"). Prefer a trusted reverse-proxy header
// over RemoteAddr in production.
func ParseIP(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// MintSession creates a new opaque session id and signed cookie value.
// Cookie format: base64url(version | issuedAtUnix | id | mac).
func MintSession(secret []byte, ttl time.Duration) (Session, string, error) {
	if err := ValidateSecretKey(secret); err != nil {
		return Session{}, "", err
	}
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	var idBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return Session{}, "", err
	}
	id := base64.RawURLEncoding.EncodeToString(idBytes[:])
	issued := time.Now().UTC()
	payload := encodeSessionPayload(sessionVersion, issued.Unix(), id)
	mac := sessionMAC(secret, payload)
	cookie := base64.RawURLEncoding.EncodeToString(append(payload, mac...))
	return Session{
		ID:        id,
		IssuedAt:  issued,
		ClientKey: "sid:" + id,
	}, cookie, nil
}

// ParseSession verifies the cookie and returns the session. expired cookies
// return ErrBadSession.
func ParseSession(secret []byte, cookie string, maxAge time.Duration) (Session, error) {
	if err := ValidateSecretKey(secret); err != nil {
		return Session{}, err
	}
	if maxAge <= 0 {
		maxAge = DefaultSessionTTL
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie)
	if err != nil || len(raw) < 1+8+1+32 {
		return Session{}, ErrBadSession
	}
	payload, mac := raw[:len(raw)-32], raw[len(raw)-32:]
	expect := sessionMAC(secret, payload)
	if subtle.ConstantTimeCompare(mac, expect) != 1 {
		return Session{}, ErrBadSession
	}
	ver, issuedUnix, id, err := decodeSessionPayload(payload)
	if err != nil || ver != sessionVersion || id == "" {
		return Session{}, ErrBadSession
	}
	issued := time.Unix(issuedUnix, 0).UTC()
	if time.Since(issued) > maxAge || issued.After(time.Now().Add(time.Minute)) {
		return Session{}, ErrBadSession
	}
	return Session{ID: id, IssuedAt: issued, ClientKey: "sid:" + id}, nil
}

func encodeSessionPayload(ver int, issuedUnix int64, id string) []byte {
	idBytes := []byte(id)
	buf := make([]byte, 1+8+len(idBytes))
	buf[0] = byte(ver)
	binary.BigEndian.PutUint64(buf[1:9], uint64(issuedUnix))
	copy(buf[9:], idBytes)
	return buf
}

func decodeSessionPayload(buf []byte) (ver int, issuedUnix int64, id string, err error) {
	if len(buf) < 1+8 {
		return 0, 0, "", ErrBadSession
	}
	ver = int(buf[0])
	issuedUnix = int64(binary.BigEndian.Uint64(buf[1:9]))
	id = string(buf[9:])
	return ver, issuedUnix, id, nil
}

func sessionMAC(secret, payload []byte) []byte {
	m := hmac.New(sha256.New, secret)
	_, _ = m.Write([]byte("gocaptcha-session-v1:"))
	_, _ = m.Write(payload)
	return m.Sum(nil)
}

// EnsureSessionCookie reads the antibot session cookie or mints a new one.
// Returns the session and whether a Set-Cookie was written.
func EnsureSessionCookie(w http.ResponseWriter, r *http.Request, secret []byte, cookieName string, ttl time.Duration) (Session, bool, error) {
	if cookieName == "" {
		cookieName = DefaultSessionCookie
	}
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		if sess, err := ParseSession(secret, c.Value, ttl); err == nil {
			return sess, false, nil
		}
	}
	sess, value, err := MintSession(secret, ttl)
	if err != nil {
		return Session{}, false, err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		MaxAge:   int(ttl.Seconds()),
	})
	return sess, true, nil
}

// SignalsFromRequest builds ClientSignals from an HTTP request.
// Prefer X-Forwarded-For / X-Real-IP only behind a trusted proxy.
func SignalsFromRequest(r *http.Request, sess Session) ClientSignals {
	ip := r.Header.Get("X-Real-IP")
	if ip == "" {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	if ip == "" {
		ip = ParseIP(r.RemoteAddr)
	}
	var issuedMs int64
	if !sess.IssuedAt.IsZero() {
		issuedMs = sess.IssuedAt.UnixMilli()
	}
	return ClientSignals{
		IP:                ip,
		UserAgent:         r.UserAgent(),
		SessionIssuedAtMs: issuedMs,
	}
}
