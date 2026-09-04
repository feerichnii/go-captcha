package antibot

import "context"

func (l *Layer) checkRate(ctx context.Context, bucket, clientKey string, max int) error {
	key := l.cfg.KeyPrefix + bucket + ":" + hashClient(clientKey)
	n, err := l.store.Incr(ctx, key, l.cfg.RateWindow)
	if err != nil {
		return wrapStore(err)
	}
	if n > int64(max) {
		return ErrRateLimited
	}
	return nil
}

// CheckIssueRate applies the issuance limit for a client key.
func (l *Layer) CheckIssueRate(ctx context.Context, clientKey string) error {
	if clientKey == "" {
		return ErrInvalidRequest
	}
	return l.checkRate(ctx, "rli", clientKey, l.cfg.IssueRateMax)
}

// CheckVerifyRate applies the verification limit for a client key.
func (l *Layer) CheckVerifyRate(ctx context.Context, clientKey string) error {
	if clientKey == "" {
		return ErrInvalidRequest
	}
	return l.checkRate(ctx, "rlv", clientKey, l.cfg.VerifyRateMax)
}
