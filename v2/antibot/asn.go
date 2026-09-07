package antibot

import "net/netip"

// ASNProvider resolves an ASN for soft risk aggregation. Nil/zero = unknown.
type ASNProvider interface {
	ASN(ip netip.Addr) int
}

// NoopASN always returns 0.
type NoopASN struct{}

func (NoopASN) ASN(netip.Addr) int { return 0 }
