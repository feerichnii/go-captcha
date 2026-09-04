package antibot

import "errors"

var (
	ErrNotFound       = errors.New("antibot: challenge not found or expired")
	ErrConsumed       = errors.New("antibot: challenge already used")
	ErrMaxAttempts    = errors.New("antibot: max attempts exceeded")
	ErrRateLimited    = errors.New("antibot: rate limited")
	ErrBadAnswer      = errors.New("antibot: bad answer")
	ErrLowScore       = errors.New("antibot: behavior score too low")
	ErrPoWRequired    = errors.New("antibot: proof-of-work required")
	ErrPoWInvalid     = errors.New("antibot: invalid proof-of-work")
	ErrInvalidRequest = errors.New("antibot: invalid request")
)
