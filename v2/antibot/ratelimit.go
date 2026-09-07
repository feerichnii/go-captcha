package antibot

import (
	"context"
	"net/netip"
)

func (l *Layer) checkRateHash(ctx context.Context, bucket, idHash string, max int) error {
	if max <= 0 || idHash == "" {
		return nil
	}
	key := l.cfg.KeyPrefix + bucket + ":" + idHash
	n, err := l.store.Incr(ctx, key, l.cfg.RateWindow)
	if err != nil {
		return wrapStore(err)
	}
	if n > int64(max) {
		return ErrRateLimited
	}
	return nil
}

// CheckIssueRate applies hard rate limits: session, exact IP /32, global.
func (l *Layer) CheckIssueRate(ctx context.Context, clientKey, ipHash string) error {
	if clientKey == "" {
		return ErrInvalidRequest
	}
	if err := l.checkRateHash(ctx, "rli", hashClient(clientKey), l.cfg.IssueRateMax); err != nil {
		return err
	}
	if ipHash != "" {
		if err := l.checkRateHash(ctx, "rliip", ipHash, l.cfg.IssueRateMax); err != nil {
			return err
		}
	}
	return l.checkRateHash(ctx, "rlig", "global", l.cfg.GlobalIssueRateMax)
}

// CheckVerifyRate applies hard verify limits: session, exact IP /32, global.
func (l *Layer) CheckVerifyRate(ctx context.Context, clientKey, ipHash string) error {
	if clientKey == "" {
		return ErrInvalidRequest
	}
	if err := l.checkRateHash(ctx, "rlv", hashClient(clientKey), l.cfg.VerifyRateMax); err != nil {
		return err
	}
	if ipHash != "" {
		if err := l.checkRateHash(ctx, "rlvip", ipHash, l.cfg.VerifyRateMax); err != nil {
			return err
		}
	}
	return l.checkRateHash(ctx, "rlvg", "global", l.cfg.GlobalVerifyRateMax)
}

// softPrefixRisk bumps soft risk when /24 issue volume is high.
func (l *Layer) softPrefixRisk(ctx context.Context, addr netip.Addr) int {
	pfx := IPv4Slash24(addr)
	if pfx == "" {
		return 0
	}
	h := HashIPPrefix(l.cfg.SecretKey, pfx)
	key := l.cfg.KeyPrefix + "rli24:" + h
	n, err := l.store.Incr(ctx, key, l.cfg.RateWindow)
	if err != nil {
		return 0
	}
	if l.cfg.SoftPrefixIssueSoft > 0 && n > int64(l.cfg.SoftPrefixIssueSoft) {
		return 1
	}
	return 0
}
