package antibot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
)

// TrajectoryFingerprint quantizes a trajectory for replay detection.
// Coarse bins so identical bot replays collide; humans rarely match.
func TrajectoryFingerprint(tr Trajectory) string {
	if len(tr.Points) < 3 {
		return ""
	}
	h := sha256.New()
	n := len(tr.Points)
	step := 1
	if n > 32 {
		step = n / 32
	}
	for i := 0; i < n; i += step {
		p := tr.Points[i]
		// 4px / 16ms bins
		fmt.Fprintf(h, "%d,%d,%d;", int(math.Round(p.X/4)), int(math.Round(p.Y/4)), int(p.T/16))
	}
	// Relative shape: durations between first/last
	fmt.Fprintf(h, "d=%d;n=%d", tr.DurationMs()/50, n/5)
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

func (l *Layer) replayKey(ipHash, fp string) string {
	return l.cfg.KeyPrefix + "ip:{" + ipHash + "}:replay:" + fp
}

// CheckTrajectoryReplay returns true if this fingerprint was already seen for the IP.
func (l *Layer) CheckTrajectoryReplay(ctx context.Context, ipHash string, tr Trajectory) (replay bool, err error) {
	if l.cfg.DisableReplayCheck {
		return false, nil
	}
	fp := TrajectoryFingerprint(tr)
	if fp == "" {
		return false, nil
	}
	_, err = l.store.Get(ctx, l.replayKey(ipHash, fp))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, wrapStore(err)
}

// NoteTrajectoryReplay records a fingerprint after a successful solve (SET NX).
func (l *Layer) NoteTrajectoryReplay(ctx context.Context, ipHash string, tr Trajectory) {
	if l.cfg.DisableReplayCheck {
		return
	}
	fp := TrajectoryFingerprint(tr)
	if fp == "" {
		return
	}
	ttl := l.cfg.ReplayTTL
	if ttl <= 0 {
		ttl = l.cfg.FailRateWindow
	}
	_, _ = l.store.SetNX(ctx, l.replayKey(ipHash, fp), []byte("1"), ttl)
}
