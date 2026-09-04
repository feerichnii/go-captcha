package antibot

import "context"

// CheckRate increments the per-client counter and returns ErrRateLimited when over max.
func (l *Layer) CheckRate(ctx context.Context, clientKey string) error {
	if clientKey == "" {
		clientKey = "anonymous"
	}
	key := l.cfg.KeyPrefix + "rl:" + clientKey
	n, err := l.store.Incr(ctx, key, l.cfg.RateLimitWindow)
	if err != nil {
		return err
	}
	if n > int64(l.cfg.RateLimitMax) {
		return ErrRateLimited
	}
	return nil
}
