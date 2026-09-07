package antibot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP extracts the authoritative client address.
// If the peer (RemoteAddr) is in TrustedProxies, trusted proxy headers may be used;
// otherwise RemoteAddr is used and spoofed X-Forwarded-For is ignored.
func ClientIP(r *http.Request, trusted []netip.Prefix) (netip.Addr, error) {
	peerHost := ParseIP(r.RemoteAddr)
	peer, err := netip.ParseAddr(peerHost)
	if err != nil {
		return netip.Addr{}, err
	}
	peer = peer.Unmap()

	if !peerInTrusted(peer, trusted) {
		return peer, nil
	}

	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if a, err := parseCanonicalIP(xri); err == nil {
			return a, nil
		}
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Left-most is the original client when proxies append.
		parts := strings.Split(xff, ",")
		if a, err := parseCanonicalIP(strings.TrimSpace(parts[0])); err == nil {
			return a, nil
		}
	}
	return peer, nil
}

func peerInTrusted(peer netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(peer) {
			return true
		}
	}
	return false
}

func parseCanonicalIP(s string) (netip.Addr, error) {
	s = strings.TrimSpace(s)
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, err
	}
	return a.Unmap(), nil
}

// CanonicalIPString returns a stable string form for hashing.
func CanonicalIPString(a netip.Addr) string {
	return a.Unmap().String()
}

// HashIP returns a 128-bit hex HMAC of the exact client IP (anti-enumeration).
func HashIP(secret []byte, a netip.Addr) string {
	return hmacLabel(secret, "ip:"+CanonicalIPString(a))
}

// HashIPPrefix returns a 128-bit hex HMAC of an aggregate prefix identifier.
func HashIPPrefix(secret []byte, prefix string) string {
	return hmacLabel(secret, "pfx:"+prefix)
}

func hmacLabel(secret []byte, label string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(label))
	sum := mac.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

// IPv4Slash24 returns the /24 prefix string for soft aggregation, or "" if not IPv4.
func IPv4Slash24(a netip.Addr) string {
	a = a.Unmap()
	if !a.Is4() {
		return ""
	}
	ip4 := a.As4()
	return netip.PrefixFrom(netip.AddrFrom4([4]byte{ip4[0], ip4[1], ip4[2], 0}), 24).String()
}

// IPv6Slash64 returns the /64 prefix string for soft aggregation, or "" if not IPv6.
func IPv6Slash64(a netip.Addr) string {
	a = a.Unmap()
	if !a.Is6() || a.Is4In6() {
		return ""
	}
	p, err := a.Prefix(64)
	if err != nil {
		return ""
	}
	return p.String()
}
