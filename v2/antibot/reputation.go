package antibot

import "net/netip"

// ReputationProvider supplies soft risk from an external DeviceKey / account /
// install-ID / TLS fingerprint system. AntiBot never requires built-in browser
// fingerprinting — wire your own source here.
type ReputationProvider interface {
	// RiskDelta returns how many risk levels to add for this identity (may be 0).
	// Negative values are ignored (use success-path de-escalation instead).
	RiskDelta(addr netip.Addr, sessionKey, deviceKey string) int
}

// DeviceKey is an optional opaque external identifier on ClientSignals.
// (Defined on ClientSignals in session.go.)
