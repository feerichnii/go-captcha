package antibot

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/feerichnii/go-captcha/v2/base/challenge"
)

// PrecheckRecord is Stage-1 checkbox state (not a geometry ChallengeRecord).
type PrecheckRecord struct {
	ID          string `json:"id"`
	ClientHash  string `json:"client_hash"`
	IPHash      string `json:"ip_hash"`
	CreatedAtMs int64  `json:"created_at_ms"`
	ExpiresAtMs int64  `json:"expires_at_ms"`
	RiskAtIssue int    `json:"risk_at_issue"`

	JSNonce     string `json:"js_nonce,omitempty"`
	JSProbe     string `json:"js_probe,omitempty"`
	JSToken     string `json:"js_token,omitempty"`
	JSWorkload  string `json:"js_workload,omitempty"`
	JSSeed      string `json:"js_seed,omitempty"`
	JSLoopCount int    `json:"js_loop_count,omitempty"`

	PoWDiff     int    `json:"pow_diff,omitempty"`
	PoWSalt     string `json:"pow_salt,omitempty"`
	PoWKind     string `json:"pow_kind,omitempty"`
	PoWMemoryMB int    `json:"pow_memory_mb,omitempty"`
	PoWRounds   int    `json:"pow_rounds,omitempty"`
}

// PrecheckIssueRequest mints Stage-1 after the user clicks the checkbox.
type PrecheckIssueRequest struct {
	ClientKey    string
	Signals      ClientSignals
	Browser      BrowserSignals
	Capabilities ClientCapabilities
	Suspicious   bool
}

// PrecheckIssueResponse is safe to return to the client.
type PrecheckIssueResponse struct {
	PrecheckID  string        `json:"precheck_id"`
	ExpiresInMs int64         `json:"expires_in_ms"`
	JSChallenge *JSChallenge  `json:"js_challenge,omitempty"`
	PoW         *PoWChallenge `json:"pow,omitempty"`
	RiskLevel   int           `json:"-"`
}

// PrecheckInteraction is untrusted client timing/coords around the checkbox click.
type PrecheckInteraction struct {
	PointerDownMs int64   `json:"pointer_down_ms,omitempty"`
	PointerUpMs   int64   `json:"pointer_up_ms,omitempty"`
	ClickX        float64 `json:"click_x,omitempty"`
	ClickY        float64 `json:"click_y,omitempty"`
	WidgetW       float64 `json:"widget_w,omitempty"`
	WidgetH       float64 `json:"widget_h,omitempty"`
	HadFocus      bool    `json:"had_focus,omitempty"`
	Visible       bool    `json:"visible,omitempty"`
}

// PrecheckVerifyRequest submits Stage-1 JS/PoW solutions.
type PrecheckVerifyRequest struct {
	PrecheckID  string
	ClientKey   string
	Signals     ClientSignals
	Browser     BrowserSignals
	PoWNonce    string
	JSResponse  string
	Interaction PrecheckInteraction
}

// PrecheckVerifyResult is returned on successful Stage-1 verification.
// Passing Precheck does not prove humanity and is not final CAPTCHA success.
type PrecheckVerifyResult struct {
	Status     string `json:"status"` // "challenge_required"
	PrecheckID string `json:"precheck_id"`
	RiskLevel  int    `json:"risk_level"`
}

func encodePrecheck(r *PrecheckRecord) ([]byte, error) { return json.Marshal(r) }

func decodePrecheck(b []byte) (*PrecheckRecord, error) {
	var r PrecheckRecord
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// PrecheckIssue creates a short-lived Stage-1 challenge (JS/PoW). No images.
func (l *Layer) PrecheckIssue(ctx context.Context, req PrecheckIssueRequest) (*PrecheckIssueResponse, error) {
	if err := validateClientKey(req.ClientKey); err != nil {
		return nil, err
	}
	addr, ipHash, err := l.requireIPHash(req.Signals)
	if err != nil {
		return nil, err
	}
	if err := l.CheckFrozen(ctx, ipHash); err != nil {
		return nil, err
	}
	if err := l.CheckPrecheckIssueRate(ctx, req.ClientKey, ipHash); err != nil {
		return nil, err
	}
	if l.cfg.RequireBrowser() && LooksLikeNonBrowserUA(req.Signals.UserAgent) {
		return nil, ErrBrowserRequired
	}
	if len(req.Capabilities.PoW) > 0 && !clientSupportsPoW(req.Capabilities, PoWAlgoSHA256V1) &&
		!clientSupportsPoW(req.Capabilities, PoWAlgoStretchV2) {
		return nil, ErrUnsupportedClient
	}

	hash := hashClient(req.ClientKey)
	l.noteSessionRotation(ctx, ipHash, hash)
	if !l.cfg.DisableSessionWarmup {
		l.ensureWarmup(ctx, hash)
	}

	level, err := l.effectiveRiskAtIssue(ctx, hash, ipHash, addr, IssueRequest{
		ClientKey: req.ClientKey, Signals: req.Signals, Browser: req.Browser, Suspicious: req.Suspicious,
	})
	if err != nil {
		return nil, err
	}

	id, err := challenge.NewID()
	if err != nil {
		return nil, err
	}
	jsCh, err := NewJSChallenge()
	if err != nil {
		return nil, err
	}
	jsCh.ChallengeID = id

	ttl := l.cfg.PrecheckTTL
	now := l.now()
	rec := &PrecheckRecord{
		ID:          id,
		ClientHash:  hash,
		IPHash:      ipHash,
		CreatedAtMs: now.UnixMilli(),
		ExpiresAtMs: now.Add(ttl).UnixMilli(),
		RiskAtIssue: level,
		JSNonce:     jsCh.Nonce,
		JSProbe:     jsCh.Probe,
		JSToken:     jsCh.Token,
		JSWorkload:  jsCh.Workload,
		JSSeed:      jsCh.Seed,
		JSLoopCount: jsCh.LoopCount,
	}

	diff := l.cfg.choosePoW(level)
	hm := l.HardMode(ctx)
	if hm.Active && hm.ExtraPoWBits > 0 {
		if diff <= 0 {
			diff = l.cfg.PoWBaseDifficulty
		}
		diff += hm.ExtraPoWBits
		if diff > l.cfg.PoWMaxDifficulty {
			diff = l.cfg.PoWMaxDifficulty
		}
	}
	// Precheck always attaches at least a light PoW so checkbox alone is not free.
	if diff <= 0 {
		diff = l.cfg.PoWProbeDifficulty
		if diff <= 0 {
			diff = 10
		}
	}
	powKind := PoWKindSHA256
	if l.cfg.StretchPoWRiskMin > 0 &&
		level >= l.cfg.StretchPoWRiskMin &&
		clientSupportsPoW(req.Capabilities, PoWAlgoStretchV2) {
		powKind = PoWKindStretch
		rec.PoWMemoryMB = l.cfg.StretchMemoryMB
		rec.PoWRounds = l.cfg.StretchRounds
		if diff > 10 {
			diff = 10
		}
		if diff < 6 {
			diff = 6
		}
	} else if !clientSupportsPoW(req.Capabilities, PoWAlgoSHA256V1) && len(req.Capabilities.PoW) > 0 {
		return nil, ErrUnsupportedClient
	}

	salt, err := CreatePoW()
	if err != nil {
		return nil, err
	}
	rec.PoWDiff = diff
	rec.PoWSalt = salt
	rec.PoWKind = powKind

	raw, err := encodePrecheck(rec)
	if err != nil {
		return nil, err
	}
	if err := l.store.Set(ctx, l.precheckKey(ipHash, id), raw, ttl); err != nil {
		return nil, wrapStore(err)
	}

	l.cfg.Telemetry.OnPrecheck(PrecheckEvent{
		PrecheckID: id, ClientHash: hash, Outcome: "issued",
		RiskLevel: level, PoWDifficulty: diff, PoWKind: powKind,
	})

	powOut := &PoWChallenge{
		Salt: salt, Difficulty: diff, ChallengeID: id, Bind: hash,
		Kind: powKind, MemoryMB: rec.PoWMemoryMB, Rounds: rec.PoWRounds,
	}
	return &PrecheckIssueResponse{
		PrecheckID:  id,
		ExpiresInMs: ttl.Milliseconds(),
		JSChallenge: &jsCh,
		PoW:         powOut,
		RiskLevel:   level,
	}, nil
}

// PrecheckVerify validates Stage-1 and marks session+IP as precheck-passed (one-shot record consumed).
// Failures raise soft risk / rate only — never geometry freeze / badgeo / epoch.
func (l *Layer) PrecheckVerify(ctx context.Context, req PrecheckVerifyRequest) (*PrecheckVerifyResult, error) {
	if req.PrecheckID == "" || !challenge.IsValidID(req.PrecheckID) {
		return nil, ErrInvalidRequest
	}
	if err := validateClientKey(req.ClientKey); err != nil {
		return nil, err
	}
	if len(req.PoWNonce) > l.cfg.MaxNonceLen {
		return nil, ErrInvalidRequest
	}
	_, ipHash, err := l.requireIPHash(req.Signals)
	if err != nil {
		return nil, err
	}
	if err := l.CheckFrozen(ctx, ipHash); err != nil {
		return nil, err
	}
	if err := l.CheckPrecheckVerifyRate(ctx, req.ClientKey, ipHash); err != nil {
		return nil, err
	}
	if l.cfg.RequireBrowser() && LooksLikeNonBrowserUA(req.Signals.UserAgent) {
		return nil, ErrBrowserRequired
	}

	hash := hashClient(req.ClientKey)
	ev := PrecheckEvent{PrecheckID: req.PrecheckID, ClientHash: hash}
	fail := func(err error) (*PrecheckVerifyResult, error) {
		ev.Outcome = "failed"
		ev.ErrorCode = ErrorCode(err)
		l.cfg.Telemetry.OnPrecheck(ev)
		l.notePrecheckFail(ctx, hash)
		return nil, err
	}

	key := l.precheckKey(ipHash, req.PrecheckID)
	raw, err := l.store.GetDel(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return fail(ErrPrecheckExpired)
	}
	if err != nil {
		return fail(wrapStore(err))
	}
	rec, err := decodePrecheck(raw)
	if err != nil {
		return fail(fmt.Errorf("%w: corrupt precheck", ErrStore))
	}

	nowMs := l.now().UnixMilli()
	if rec.ExpiresAtMs > 0 && nowMs > rec.ExpiresAtMs {
		return fail(ErrPrecheckExpired)
	}
	if subtle.ConstantTimeCompare([]byte(rec.ClientHash), []byte(hash)) != 1 ||
		subtle.ConstantTimeCompare([]byte(rec.IPHash), []byte(ipHash)) != 1 {
		return fail(ErrPrecheckExpired)
	}

	elapsed := nowMs - rec.CreatedAtMs
	ev.ElapsedMs = elapsed
	ev.RiskLevel = rec.RiskAtIssue
	ev.PoWDifficulty = rec.PoWDiff
	ev.PoWKind = rec.PoWKind
	if elapsed < l.cfg.PrecheckMinSolveTime.Milliseconds() {
		return fail(ErrTooFast)
	}

	if rec.PoWDiff > 0 {
		ok := false
		if rec.PoWKind == PoWKindStretch {
			ok = VerifyStretchPoW(rec.ID, hash, rec.PoWSalt, req.PoWNonce, rec.PoWDiff, rec.PoWMemoryMB, rec.PoWRounds, l.cfg.MaxNonceLen)
		} else {
			ok = VerifyPoW(rec.ID, hash, rec.PoWSalt, req.PoWNonce, rec.PoWDiff, l.cfg.MaxNonceLen)
		}
		if !ok {
			return fail(ErrPoWInvalid)
		}
	}

	jsOK := true
	if rec.JSNonce != "" {
		resp := req.JSResponse
		if resp == "" {
			resp = req.Browser.JSChallengeResponse
		}
		if resp == "" {
			if l.cfg.RequireBrowserSignals() {
				return fail(ErrJSChallengeFailed)
			}
			jsOK = false
		} else {
			candidates := ProbeCandidates(req.Browser, rec.JSProbe)
			ch := JSChallenge{
				Nonce: rec.JSNonce, Probe: rec.JSProbe, Token: rec.JSToken,
				Seed: rec.JSSeed, Workload: rec.JSWorkload, LoopCount: rec.JSLoopCount,
			}
			jsOK = CheckJSChallenge(rec.ID, ch, resp, candidates...)
			if !jsOK && l.cfg.RequireBrowserSignals() {
				return fail(ErrJSChallengeFailed)
			}
		}
	}

	bDelta, bReasons := BrowserRisk(req.Browser, jsOK)
	if cDelta, cReasons := BrowserConsistencyRisk(req.Browser, req.Signals.UserAgent); cDelta > 0 {
		bDelta += cDelta
		bReasons = append(bReasons, cReasons...)
	}
	if fDelta, fReasons := FingerprintRisk(req.Browser); fDelta > 0 {
		bDelta += fDelta
		bReasons = append(bReasons, fReasons...)
	}
	bDelta += precheckInteractionRisk(req.Interaction)

	passed := &precheckPassed{
		PrecheckID:  rec.ID,
		ClientHash:  hash,
		IPHash:      ipHash,
		CreatedAtMs: nowMs,
		RiskAtIssue: rec.RiskAtIssue,
	}
	passedRaw, err := json.Marshal(passed)
	if err != nil {
		return fail(err)
	}
	if err := l.store.Set(ctx, l.precheckPassedKey(ipHash, hash), passedRaw, l.cfg.PrecheckTTL); err != nil {
		return fail(wrapStore(err))
	}

	if bDelta > 0 {
		_, _ = l.EvaluateRisk(ctx, hash, RiskInputs{
			Score: ScoreResult{Score: 0.6, Consistent: true}, BrowserDelta: bDelta, BrowserReasons: bReasons,
			Signals: req.Signals, NowMs: nowMs,
		})
	}

	levelAfter, _ := l.riskLevel(ctx, hash)
	ev.Outcome = "success"
	ev.RiskLevel = levelAfter
	l.cfg.Telemetry.OnPrecheck(ev)

	return &PrecheckVerifyResult{
		Status:     "challenge_required",
		PrecheckID: rec.ID,
		RiskLevel:  levelAfter,
	}, nil
}

type precheckPassed struct {
	PrecheckID  string `json:"precheck_id"`
	ClientHash  string `json:"client_hash"`
	IPHash      string `json:"ip_hash"`
	CreatedAtMs int64  `json:"created_at_ms"`
	RiskAtIssue int    `json:"risk_at_issue"`
}

func (l *Layer) notePrecheckFail(ctx context.Context, sessionHash string) {
	_, _ = l.store.Incr(ctx, l.cfg.KeyPrefix+"precheckfail:"+sessionHash, l.cfg.FailRateWindow)
	_, _ = l.bumpRisk(ctx, sessionHash, 1)
}

func precheckInteractionRisk(in PrecheckInteraction) int {
	delta := 0
	if in.PointerDownMs > 0 && in.PointerUpMs > 0 {
		dwell := in.PointerUpMs - in.PointerDownMs
		if dwell < 8 || dwell > 60_000 {
			delta++
		}
	}
	if in.WidgetW > 0 && in.WidgetH > 0 {
		if in.ClickX < 0 || in.ClickY < 0 || in.ClickX > in.WidgetW || in.ClickY > in.WidgetH {
			delta++
		}
	}
	return delta
}

// consumePrecheckPassed removes and validates precheck-passed for Issue.
func (l *Layer) consumePrecheckPassed(ctx context.Context, sessionHash, ipHash string) (*precheckPassed, error) {
	raw, err := l.store.GetDel(ctx, l.precheckPassedKey(ipHash, sessionHash))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrPrecheckRequired
	}
	if err != nil {
		return nil, wrapStore(err)
	}
	var p precheckPassed
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("%w: corrupt precheck-passed", ErrStore)
	}
	if subtle.ConstantTimeCompare([]byte(p.ClientHash), []byte(sessionHash)) != 1 ||
		subtle.ConstantTimeCompare([]byte(p.IPHash), []byte(ipHash)) != 1 {
		return nil, ErrPrecheckRequired
	}
	nowMs := l.now().UnixMilli()
	if p.CreatedAtMs > 0 && nowMs-p.CreatedAtMs > l.cfg.PrecheckTTL.Milliseconds() {
		return nil, ErrPrecheckExpired
	}
	return &p, nil
}
