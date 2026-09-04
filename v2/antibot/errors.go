package antibot

import "errors"

var (
	ErrNotFound       = errors.New("antibot: challenge not found, expired or already used")
	ErrMaxAttempts    = errors.New("antibot: max attempts exceeded")
	ErrRateLimited    = errors.New("antibot: rate limited")
	ErrBadAnswer      = errors.New("antibot: bad answer")
	ErrLowScore       = errors.New("antibot: behavior score below hard reject threshold")
	ErrPoWInvalid     = errors.New("antibot: invalid proof-of-work")
	ErrTooFast        = errors.New("antibot: solved faster than MinSolveTime")
	ErrInvalidRequest = errors.New("antibot: invalid request")
	ErrNoSecretKey    = errors.New("antibot: Config.SecretKey is required")
	ErrStore          = errors.New("antibot: store failure")
)

// IsClientError reports whether err should be shown to the end user as a
// generic "captcha failed" (true) vs. logged as an internal problem (false).
func IsClientError(err error) bool {
	switch {
	case errors.Is(err, ErrNotFound),
		errors.Is(err, ErrMaxAttempts),
		errors.Is(err, ErrRateLimited),
		errors.Is(err, ErrBadAnswer),
		errors.Is(err, ErrLowScore),
		errors.Is(err, ErrPoWInvalid),
		errors.Is(err, ErrTooFast),
		errors.Is(err, ErrInvalidRequest):
		return true
	}
	return false
}
