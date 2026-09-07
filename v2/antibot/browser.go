package antibot

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// BrowserSignals are client-reported environment hints. Fields are untrusted:
// use them as risk inputs; hard gates (JS challenge / non-browser UA) are
// enforced separately when Config.RequireBrowserSignals() is true.
type BrowserSignals struct {
	WebDriver           bool     `json:"webdriver,omitempty"`
	HeadlessHints       []string `json:"headless_hints,omitempty"`
	Languages           []string `json:"languages,omitempty"`
	Platform            string   `json:"platform,omitempty"`
	HardwareConcurrency int      `json:"hardware_concurrency,omitempty"`
	DeviceMemory        float64  `json:"device_memory,omitempty"`
	CoalescedTotal      int      `json:"coalesced_total,omitempty"`
	OuterZero           bool     `json:"outer_zero,omitempty"`
	PluginCount         int      `json:"plugin_count,omitempty"`
	JSChallengeResponse string   `json:"js_challenge_response,omitempty"`
	// SecCHUA is sec-ch-ua when the client forwards it (soft consistency).
	SecCHUA string `json:"sec_ch_ua,omitempty"`
	// MaxTouchPoints from navigator (soft consistency with UA/platform).
	MaxTouchPoints int `json:"max_touch_points,omitempty"`
	// Optional soft fingerprints (never required; empty is fine).
	CanvasHash string `json:"canvas_hash,omitempty"`
	WebGLHash  string `json:"webgl_hash,omitempty"`
	AudioHash  string `json:"audio_hash,omitempty"`
	// A11Y true when the client used the keyboard accessibility path.
	A11Y bool `json:"a11y,omitempty"`
}

// JSChallenge is a rotating DOM/JS workload issued with the captcha.
// Client computes:
//
//	SHA-256(nonce + "|" + challengeID + "|" + token + "|" + workDigest + "|" + probeValue)
//
// workDigest depends on Workload (probe / loop / mix).
type JSChallenge struct {
	Nonce       string `json:"nonce"`
	ChallengeID string `json:"challenge_id,omitempty"`
	Token       string `json:"token"`
	Seed        string `json:"seed"`
	Workload    string `json:"workload"` // probe | loop | mix
	Probe       string `json:"probe"`
	LoopCount   int    `json:"loop_count,omitempty"`
}

const (
	JSWorkloadProbe = "probe"
	JSWorkloadLoop  = "loop"
	JSWorkloadMix   = "mix"
)

// NewJSChallenge mints a fresh rotating workload challenge.
func NewJSChallenge() (JSChallenge, error) {
	var nb, tb, sb [16]byte
	if _, err := rand.Read(nb[:]); err != nil {
		return JSChallenge{}, err
	}
	if _, err := rand.Read(tb[:]); err != nil {
		return JSChallenge{}, err
	}
	if _, err := rand.Read(sb[:]); err != nil {
		return JSChallenge{}, err
	}
	workloads := []string{JSWorkloadProbe, JSWorkloadLoop, JSWorkloadMix}
	probes := []string{"languages.length", "platform", "hw"}
	w := workloads[cryptoIntn(len(workloads))]
	p := probes[cryptoIntn(len(probes))]
	loops := 0
	if w == JSWorkloadLoop {
		loops = 32 + cryptoIntn(33) // 32..64
	}
	return JSChallenge{
		Nonce:     hex.EncodeToString(nb[:]),
		Token:     hex.EncodeToString(tb[:]),
		Seed:      hex.EncodeToString(sb[:]),
		Workload:  w,
		Probe:     p,
		LoopCount: loops,
	}, nil
}

// WorkloadDigest is the deterministic JS-side work output (sans live probe).
func WorkloadDigest(ch JSChallenge) string {
	switch ch.Workload {
	case JSWorkloadLoop:
		n := ch.LoopCount
		if n < 1 {
			n = 32
		}
		h := ch.Seed
		for i := 0; i < n; i++ {
			sum := sha256.Sum256([]byte(h))
			h = hex.EncodeToString(sum[:])
		}
		return h
	case JSWorkloadMix:
		sum := sha256.Sum256([]byte(ch.Token + ":" + ch.Seed))
		return hex.EncodeToString(sum[:8])
	default:
		return "probe"
	}
}

// ExpectedJSResponse returns the hex SHA-256 the client must produce.
func ExpectedJSResponse(nonce, challengeID, token, workDigest, probeValue string) string {
	sum := sha256.Sum256([]byte(nonce + "|" + challengeID + "|" + token + "|" + workDigest + "|" + probeValue))
	return hex.EncodeToString(sum[:])
}

// CheckJSChallenge verifies the client response against known probe candidates.
func CheckJSChallenge(challengeID string, ch JSChallenge, response string, candidates ...string) bool {
	if ch.Nonce == "" || challengeID == "" || ch.Token == "" || response == "" {
		return false
	}
	work := WorkloadDigest(ch)
	for _, c := range candidates {
		expect := ExpectedJSResponse(ch.Nonce, challengeID, ch.Token, work, c)
		if subtle.ConstantTimeCompare([]byte(strings.ToLower(response)), []byte(expect)) == 1 {
			return true
		}
	}
	return false
}

// BrowserRisk returns how many risk levels to add based on browser signals.
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
	if sig.HardwareConcurrency == 1 && sig.DeviceMemory > 0 && sig.DeviceMemory <= 0.5 {
		delta++
		reasons = append(reasons, "tiny_device")
	}
	if sig.PluginCount == 0 && looksLikeDesktop(sig.Platform) {
		reasons = append(reasons, "no_plugins")
	}
	return delta, reasons
}

// BrowserConsistencyRisk soft-scores UA/platform/touch/language mismatches.
// Never a hard fail — only risk / PoW pressure.
func BrowserConsistencyRisk(sig BrowserSignals, ua string) (delta int, reasons []string) {
	uaL := strings.ToLower(ua)
	plat := strings.ToLower(sig.Platform)
	mobileUA := strings.Contains(uaL, "mobile") || strings.Contains(uaL, "android") || strings.Contains(uaL, "iphone")
	desktopPlat := looksLikeDesktop(sig.Platform)

	if mobileUA && desktopPlat && !strings.Contains(plat, "android") {
		delta++
		reasons = append(reasons, "ua_platform_mismatch")
	}
	if !mobileUA && sig.MaxTouchPoints > 5 && desktopPlat {
		// Soft: desktop with many touch points is ok (Surface); only flag zero langs.
	}
	if desktopPlat && len(sig.Languages) == 0 && ua != "" {
		delta++
		reasons = append(reasons, "empty_languages_desktop")
	}
	if mobileUA && sig.MaxTouchPoints == 0 && ua != "" {
		delta++
		reasons = append(reasons, "mobile_no_touch")
	}
	if sig.SecCHUA != "" {
		ch := strings.ToLower(sig.SecCHUA)
		if strings.Contains(uaL, "chrome") && !strings.Contains(ch, "chrom") && !strings.Contains(ch, "google") {
			delta++
			reasons = append(reasons, "sec_ch_ua_mismatch")
		}
	}
	return delta, reasons
}

func looksLikeDesktop(platform string) bool {
	p := strings.ToLower(platform)
	return strings.Contains(p, "win") || strings.Contains(p, "mac") || strings.Contains(p, "linux")
}

// ProbeCandidates builds likely probe values from reported signals.
// Hardcoded filler values ("1","2","3") are intentionally not accepted.
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
		return []string{strconv.Itoa(len(sig.Languages))}
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

var nonBrowserUATokens = []string{
	"curl/", "wget/", "python-requests", "python-urllib", "httpie", "scrapy",
	"go-http-client", "java/", "apache-httpclient", "libwww-perl", "php/",
	"node-fetch", "axios/", "okhttp", "postmanruntime", "insomnia/",
}

// LooksLikeNonBrowserUA reports User-Agents typical of curl/scripts (not browsers).
func LooksLikeNonBrowserUA(ua string) bool {
	ua = strings.TrimSpace(strings.ToLower(ua))
	if ua == "" {
		return false
	}
	for _, t := range nonBrowserUATokens {
		if strings.Contains(ua, t) {
			return true
		}
	}
	return false
}

// AssertBrowserHeaders rejects obvious non-browser HTTP requests
// (RejectObviousAutomation semantics — not browser attestation).
func AssertBrowserHeaders(r *http.Request) error {
	if r == nil {
		return ErrBrowserRequired
	}
	ua := r.Header.Get("User-Agent")
	if LooksLikeNonBrowserUA(ua) {
		return ErrBrowserRequired
	}
	if mode := strings.ToLower(r.Header.Get("Sec-Fetch-Mode")); mode != "" {
		switch mode {
		case "cors", "same-origin", "navigate", "no-cors", "websocket":
		default:
			return ErrBrowserRequired
		}
	}
	accept := r.Header.Get("Accept")
	if accept != "" && !strings.Contains(accept, "json") && !strings.Contains(accept, "*/*") && !strings.Contains(accept, "html") {
		return ErrBrowserRequired
	}
	return nil
}

// SummarizeSignals is a short debug string for telemetry.
func SummarizeSignals(sig BrowserSignals) string {
	return fmt.Sprintf("wd=%v hints=%d plugins=%d hw=%d",
		sig.WebDriver, len(sig.HeadlessHints), sig.PluginCount, sig.HardwareConcurrency)
}
