package antibot

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// BrowserSignals are client-reported environment hints. All fields are
// untrusted: use them only as risk inputs, never as sole reject criteria.
type BrowserSignals struct {
	// WebDriver is navigator.webdriver.
	WebDriver bool `json:"webdriver,omitempty"`
	// HeadlessHints lists detected headless markers (e.g. "ua.headless", "chrome.runtime_missing").
	HeadlessHints []string `json:"headless_hints,omitempty"`
	Languages     []string `json:"languages,omitempty"`
	Platform      string   `json:"platform,omitempty"`
	// HardwareConcurrency / DeviceMemory from navigator.
	HardwareConcurrency int     `json:"hardware_concurrency,omitempty"`
	DeviceMemory        float64 `json:"device_memory,omitempty"`
	// CoalescedTotal is the sum of getCoalescedEvents().length across moves.
	CoalescedTotal int `json:"coalesced_total,omitempty"`
	// OuterVsInner reports window.outerWidth/Height == 0 (common headless tell).
	OuterZero bool `json:"outer_zero,omitempty"`
	// PluginCount is navigator.plugins.length.
	PluginCount int `json:"plugin_count,omitempty"`
	// JSChallengeResponse is the solution to IssueResponse.JSChallenge.
	JSChallengeResponse string `json:"js_challenge_response,omitempty"`
}

// JSChallenge is a tiny DOM/JS puzzle issued with the captcha. The browser
// must compute SHA-256(nonce + "|" + probe) and send it back as
// BrowserSignals.JSChallengeResponse. probe is a property the client reads
// from the live environment (e.g. String(navigator.languages.length)).
type JSChallenge struct {
	Nonce string `json:"nonce"`
	// Probe tells the client which value to append. Supported:
	//   "languages.length" (default), "platform", "hw"
	Probe string `json:"probe"`
}

// NewJSChallenge mints a fresh challenge.
func NewJSChallenge() (JSChallenge, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return JSChallenge{}, err
	}
	return JSChallenge{Nonce: hex.EncodeToString(b[:]), Probe: "languages.length"}, nil
}

// ExpectedJSResponse returns the hex SHA-256 the client must produce for the
// given probe value (already stringified).
func ExpectedJSResponse(nonce, probeValue string) string {
	sum := sha256.Sum256([]byte(nonce + "|" + probeValue))
	return hex.EncodeToString(sum[:])
}

// CheckJSChallenge verifies the client response against known probe candidates.
// Returns true if any candidate matches (constant-time per candidate).
func CheckJSChallenge(ch JSChallenge, response string, candidates ...string) bool {
	if ch.Nonce == "" || response == "" {
		return false
	}
	for _, c := range candidates {
		expect := ExpectedJSResponse(ch.Nonce, c)
		if subtle.ConstantTimeCompare([]byte(strings.ToLower(response)), []byte(expect)) == 1 {
			return true
		}
	}
	return false
}

// BrowserRisk returns how many risk levels to add based on browser signals
// (0 = clean, higher = worse). jsOK is whether the JS challenge passed.
func BrowserRisk(sig BrowserSignals, jsOK bool) (delta int, reasons []string) {
	if sig.WebDriver {
		delta++
		reasons = append(reasons, "webdriver")
	}
	if len(sig.HeadlessHints) > 0 {
		delta++
		reasons = append(reasons, "headless")
	}
	if sig.OuterZero {
		delta++
		reasons = append(reasons, "outer_zero")
	}
	if !jsOK {
		delta++
		reasons = append(reasons, "js_challenge_failed")
	}
	// Unrealistic hardware fingerprints.
	if sig.HardwareConcurrency == 1 && sig.DeviceMemory > 0 && sig.DeviceMemory <= 0.5 {
		delta++
		reasons = append(reasons, "tiny_device")
	}
	if sig.PluginCount == 0 && looksLikeDesktop(sig.Platform) {
		// Soft signal only — many privacy browsers also report 0.
		reasons = append(reasons, "no_plugins")
	}
	return delta, reasons
}

func looksLikeDesktop(platform string) bool {
	p := strings.ToLower(platform)
	return strings.Contains(p, "win") || strings.Contains(p, "mac") || strings.Contains(p, "linux")
}

// ProbeCandidates builds likely probe values from the reported signals so the
// server can verify the JS challenge without a round-trip round of trust.
func ProbeCandidates(sig BrowserSignals, probe string) []string {
	switch probe {
	case "platform":
		if sig.Platform != "" {
			return []string{sig.Platform}
		}
	case "hw":
		if sig.HardwareConcurrency > 0 {
			return []string{strconv.Itoa(sig.HardwareConcurrency)}
		}
	default: // languages.length
		return []string{strconv.Itoa(len(sig.Languages)), "1", "2", "3"}
	}
	return []string{"0"}
}

// FormatUAHint extracts cheap UA-level headless markers.
func FormatUAHint(ua string) []string {
	var out []string
	l := strings.ToLower(ua)
	for _, h := range []string{"headless", "phantomjs", "selenium", "webdriver", "puppeteer", "playwright"} {
		if strings.Contains(l, h) {
			out = append(out, "ua."+h)
		}
	}
	return out
}

// SummarizeSignals is a short debug string for telemetry.
func SummarizeSignals(sig BrowserSignals) string {
	return fmt.Sprintf("wd=%v hints=%d plugins=%d hw=%d",
		sig.WebDriver, len(sig.HeadlessHints), sig.PluginCount, sig.HardwareConcurrency)
}
