package antibot

import (
	"context"
	"time"
)

// HardModeState is returned on Issue when global anomaly adaptation is active.
type HardModeState struct {
	Active            bool `json:"active"`
	ExtraPoWBits      int  `json:"extra_pow_bits,omitempty"`
	TTLMillis         int  `json:"ttl_ms,omitempty"` // shortened challenge TTL when active
	CandidateSlotsMin int  `json:"candidate_slots_min,omitempty"`
}

func (l *Layer) globalIssueKey() string { return l.cfg.KeyPrefix + "global:issues" }
func (l *Layer) globalBadKey() string   { return l.cfg.KeyPrefix + "global:badgeo" }

func (l *Layer) noteGlobalIssue(ctx context.Context) {
	_, _ = l.store.Incr(ctx, l.globalIssueKey(), l.cfg.FailRateWindow)
}

func (l *Layer) noteGlobalBad(ctx context.Context) {
	_, _ = l.store.Incr(ctx, l.globalBadKey(), l.cfg.FailRateWindow)
}

// HardMode evaluates global issue/bad-answer rates and returns adaptation hints.
// Active when badgeo count is high relative to issues, or absolute bad spike.
func (l *Layer) HardMode(ctx context.Context) HardModeState {
	if l.cfg.DisableHardMode {
		return HardModeState{}
	}
	issues, _ := l.store.IncrBy(ctx, l.globalIssueKey(), 0, l.cfg.FailRateWindow)
	bads, _ := l.store.IncrBy(ctx, l.globalBadKey(), 0, l.cfg.FailRateWindow)
	threshold := l.cfg.HardModeBadRate
	if threshold <= 0 {
		threshold = 0.45
	}
	minIssues := int64(l.cfg.HardModeMinIssues)
	if minIssues <= 0 {
		minIssues = 20
	}
	absBad := int64(l.cfg.HardModeAbsBad)
	if absBad <= 0 {
		absBad = 40
	}

	active := false
	if bads >= absBad {
		active = true
	}
	if issues >= minIssues && float64(bads)/float64(issues) >= threshold {
		active = true
	}
	if !active {
		return HardModeState{}
	}
	ttl := l.cfg.TTL
	if l.cfg.HardModeTTLFactor > 0 && l.cfg.HardModeTTLFactor < 1 {
		ttl = time.Duration(float64(ttl) * l.cfg.HardModeTTLFactor)
	} else {
		ttl = ttl / 2
	}
	if ttl < 15*time.Second {
		ttl = 15 * time.Second
	}
	extra := l.cfg.HardModeExtraPoWBits
	if extra <= 0 {
		extra = 2
	}
	slots := l.cfg.HardModeSlotsMin
	if slots <= 0 {
		slots = 6
	}
	return HardModeState{
		Active:            true,
		ExtraPoWBits:      extra,
		TTLMillis:         int(ttl / time.Millisecond),
		CandidateSlotsMin: slots,
	}
}
