package antibot

import "errors"

var (
	ErrNotFound       = errors.New("antibot: challenge not found, expired or already used")
	// Deprecated: one-shot geometry uses ErrNotFound / ErrLocked instead.
	ErrMaxAttempts = errors.New("antibot: max attempts exceeded")
	ErrLocked      = errors.New("antibot: ip or binding frozen")
	ErrRateLimited = errors.New("antibot: rate limited")
	ErrBadAnswer   = errors.New("antibot: bad answer")
	ErrLowScore    = errors.New("antibot: behavior score below hard reject threshold")
	ErrPoWInvalid  = errors.New("antibot: invalid proof-of-work")
	ErrTooFast     = errors.New("antibot: solved faster than MinSolveTime")
	ErrInvalidRequest = errors.New("antibot: invalid request")
	ErrNoSecretKey    = errors.New("antibot: Config.SecretKey is required")
	ErrStore              = errors.New("antibot: store failure")
	ErrBadTrajectory      = errors.New("antibot: trajectory failed structural checks")
	ErrJSChallengeFailed  = errors.New("antibot: JS / DOM challenge failed")
	ErrBrowserRequired    = errors.New("antibot: browser attestation required")
	ErrPiecePressRequired = errors.New("antibot: puzzle piece press required before drag")
	ErrMissingClientIP    = errors.New("antibot: authoritative client IP required")
)

// IsClientError reports whether err should be shown to the end user as a
// generic "captcha failed" (true) vs. logged as an internal problem (false).
func IsClientError(err error) bool {
	switch {
	case errors.Is(err, ErrNotFound),
		errors.Is(err, ErrMaxAttempts),
		errors.Is(err, ErrLocked),
		errors.Is(err, ErrRateLimited),
		errors.Is(err, ErrBadAnswer),
		errors.Is(err, ErrLowScore),
		errors.Is(err, ErrPoWInvalid),
		errors.Is(err, ErrTooFast),
		errors.Is(err, ErrInvalidRequest),
		errors.Is(err, ErrClientKeyLooksLikeIP),
		errors.Is(err, ErrBadSession),
		errors.Is(err, ErrBadTrajectory),
		errors.Is(err, ErrWeakSecretKey),
		errors.Is(err, ErrJSChallengeFailed),
		errors.Is(err, ErrBrowserRequired),
		errors.Is(err, ErrPiecePressRequired),
		errors.Is(err, ErrMissingClientIP):
		return true
	}
	return false
}
