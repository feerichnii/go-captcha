package antibot

import "errors"

var (
	ErrNotFound = errors.New("antibot: challenge not found, expired or already used")
	// Deprecated: one-shot geometry uses ErrNotFound / ErrLocked instead.
	ErrMaxAttempts        = errors.New("antibot: max attempts exceeded")
	ErrLocked             = errors.New("antibot: ip or binding frozen")
	ErrRateLimited        = errors.New("antibot: rate limited")
	ErrBadAnswer          = errors.New("antibot: bad answer")
	ErrLowScore           = errors.New("antibot: behavior score below hard reject threshold")
	ErrPoWInvalid         = errors.New("antibot: invalid proof-of-work")
	ErrTooFast            = errors.New("antibot: solved faster than MinSolveTime")
	ErrInvalidRequest     = errors.New("antibot: invalid request")
	ErrNoSecretKey        = errors.New("antibot: Config.SecretKey is required")
	ErrStore              = errors.New("antibot: store failure")
	ErrBadTrajectory      = errors.New("antibot: trajectory failed structural checks")
	ErrJSChallengeFailed  = errors.New("antibot: JS / DOM challenge failed")
	ErrBrowserRequired    = errors.New("antibot: browser attestation required")
	ErrPiecePressRequired = errors.New("antibot: puzzle piece press required before drag")
	ErrMissingClientIP    = errors.New("antibot: authoritative client IP required")
)

// Machine-readable error_code values for HTTP/API responses.
const (
	CodePoWInvalid         = "pow_invalid"
	CodeJSFailed           = "js_failed"
	CodeTooFast            = "too_fast"
	CodeBadGeometry        = "bad_geometry"
	CodeLocked             = "locked"
	CodeNotFound           = "not_found"
	CodeRateLimited        = "rate_limited"
	CodeLowScore           = "low_score"
	CodeBrowserRequired    = "browser_required"
	CodePiecePressRequired = "piece_press_required"
	CodeBadTrajectory      = "bad_trajectory"
	CodeInvalidRequest     = "invalid_request"
	CodeInternal           = "internal"
)

// ErrorCode returns a stable machine-readable code for API clients.
// Unknown / store failures map to "internal".
func ErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrPoWInvalid):
		return CodePoWInvalid
	case errors.Is(err, ErrJSChallengeFailed):
		return CodeJSFailed
	case errors.Is(err, ErrTooFast):
		return CodeTooFast
	case errors.Is(err, ErrBadAnswer):
		return CodeBadGeometry
	case errors.Is(err, ErrLocked):
		return CodeLocked
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrMaxAttempts):
		return CodeNotFound
	case errors.Is(err, ErrRateLimited):
		return CodeRateLimited
	case errors.Is(err, ErrLowScore):
		return CodeLowScore
	case errors.Is(err, ErrBrowserRequired):
		return CodeBrowserRequired
	case errors.Is(err, ErrPiecePressRequired):
		return CodePiecePressRequired
	case errors.Is(err, ErrBadTrajectory):
		return CodeBadTrajectory
	case errors.Is(err, ErrInvalidRequest),
		errors.Is(err, ErrClientKeyLooksLikeIP),
		errors.Is(err, ErrBadSession),
		errors.Is(err, ErrMissingClientIP),
		errors.Is(err, ErrWeakSecretKey),
		errors.Is(err, ErrNoSecretKey):
		return CodeInvalidRequest
	default:
		return CodeInternal
	}
}

// IsClientError reports whether err should be shown to the end user as a
// client-facing captcha failure (true) vs. logged as an internal problem (false).
func IsClientError(err error) bool {
	code := ErrorCode(err)
	return code != "" && code != CodeInternal
}
