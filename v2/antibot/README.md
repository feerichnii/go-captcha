# antibot

AntiBot layer around go-captcha: one-shot geometry, HMAC IP identity, escalating freeze, adaptive PoW, Dynamic Hard Mode, and trajectory risk scoring.

```
AntiBot layer
├── Challenge Manager   crypto ID, Redis/Memory, TTL 90s, one-shot geometry (no MaxAttempts)
├── IP epoch + active   single live challenge per /32; bad answer bumps epoch (invalidates prefetch)
├── Answer storage      AES-256-GCM, bound to challenge id — never plaintext, never sent to client
├── Binding             session cookie (ClientKey) + HMAC exact IPv4 /32 (IPHash)
├── Freeze              /32 freeze survives cookie rotation; escalating 2s→5s→15s→60s→300s
├── Atomic ops          IssueChallenge / ClaimGeometry / FinalizeFailure (Lua or memory mutex)
├── Bound PoW           SHA-256(challengeID:sessionBind:salt:nonce); Hard Mode adds bits
├── Bound JS gate       SHA-256(nonce|challengeID|probe); exact probe; non-browser UA reject
├── Dynamic Hard Mode   global bad/issue spike → shorter TTL, extra PoW, denser-slot hint
├── Piece press         slide/rotate require piece_down — opt out AllowMissingPiecePress
├── Trajectory scoring  order, monotonic t, jumps, PointerEvent meta → RISK signal
├── Rate limits         hard: session + /32 + global; soft: /24 (+ ASN stub) → risk only
├── Server-side timing  MinSolveTime, trajectory-vs-elapsed consistency, input size caps
├── Adaptive PoW        risk level → difficulty + probe PoW + jitter
├── Success reputation  clean solve slowly lowers session + /32 risk
└── Telemetry           issue/verify events + Calibrator (ROC / F1 suggestions)
```

## Quick start

```go
import (
    "encoding/json"
    "os"
    "time"

    "github.com/feerichnii/go-captcha/v2/antibot"
    "github.com/feerichnii/go-captcha/v2/slide"
    "github.com/redis/go-redis/v9"
)

secret := []byte(os.Getenv("CAPTCHA_SECRET")) // required: >= 32 high-entropy bytes
rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
layer, err := antibot.New(antibot.NewRedisStore(rdb), antibot.Config{
    SecretKey:  secret,
    TTL:        90 * time.Second,
    GeoLockTTL: 5 * time.Second, // in-flight geometry lock (default 5s)
    // TrustedProxies: nil → RemoteAddr only (spoofed XFF ignored)
})

sess, _, _ := antibot.EnsureSessionCookie(w, r, secret, antibot.DefaultSessionCookie, antibot.DefaultSessionTTL)
signals := antibot.SignalsFromRequestTrusted(r, sess, nil) // authoritative ClientSignals.IP

answer, _ := json.Marshal(captData.GetData())
iss, err := layer.Issue(ctx, antibot.IssueRequest{
    Kind:      antibot.KindSlide,
    Answer:    answer,
    ClientKey: sess.ClientKey,
    Signals:   signals, // IP required
})
// → iss.ID, public data, images, iss.PoW, iss.JSChallenge
// ErrLocked + retry_after_ms when /32 is frozen

res, err := layer.Verify(ctx, antibot.VerifyRequest{
    ID:         iss.ID,
    ClientKey:  sess.ClientKey,
    Signals:    signals,
    Browser:    browserFromJSON, // js_challenge_response when required
    Answer:     mustJSON(antibot.SlideSubmit{X: ux, Y: uy}),
    Trajectory: antibot.Trajectory{Points: points, Events: events, PieceDown: pieceDown},
    PoWNonce:   nonce,
})
// Wrong geometry → ErrBadAnswer + freeze + epoch bump (retry_after_ms)
// Tech failures (PoW/JS/timing) keep the challenge (no consume)
```

### Verify order (P0.1)

1. Validate + hard rate limits + authoritative IP  
2. Check `/32` freeze → `ErrLocked`  
3. Read-only GET challenge; bind session + IPHash + **IPEpoch**  
4. Tech gates (timing, PoW, JS, piece_down, trajectory) — **do not consume**  
5. `ClaimGeometryAtomic` (geo lock, consume challenge, clear active)  
6. Decrypt → on internal error **`FinalizeAbort`** (release geo only)  
7. Geometry → wrong: **`FinalizeFailureAtomic`** (freeze + epoch); correct: **`FinalizeSuccess`**

**1 visual challenge = 1 geometry attempt.** Prefetch: only one `active` challenge per `/32`; Issue replaces the previous. Bad answer increments IP epoch so older stamped records cannot be claimed.

### Identity and freeze

| Concept | Role |
|---------|------|
| Session cookie (`ClientKey`) | Challenge binding + soft risk / warmup |
| Exact IPv4 `/32` (HMAC) | Hard RL, freeze identity, geo lock, epoch |
| `freeze:{ip}` | Survives cookie delete; blocks Issue and Verify |
| Escalation | 1→2s, 2→5s, 3→15s, 4→60s, 5+→300s |

### Rate / risk buckets

- **Hard:** session, exact `/32`, global  
- **Soft:** IPv4 `/24`, ASN (pluggable) → risk / PoW only (never hard-block alone)  
- Distinct **session rotation** per `/32` (`sessseen` SET NX → `sessrot`); risk bump at `sessrot >= 8`

### Redis Cluster

IP transaction keys use hash tag `{ipHash}` so Issue / Claim / Finalize Lua scripts stay on one slot, e.g. `…:ip:{<hash>}:freeze`, `…:ch:<id>`, `…:geo`.

### Browser gate / piece press / SecretKey / ClientKey

Unchanged behavior: see defaults `RequireBrowser()`, `RequirePiecePress()`, weak-key rejection, and session-id `ClientKey` (never raw IP). Prefer `SignalsFromRequestTrusted` with `TrustedProxies`.

### What the client must NOT receive

`GetData()`, `IssueRequest.Answer`, store records. Only `ID`, `GetPublicData()`, images, `PoW`, `JSChallenge`.

## Behavior score is risk, not proof

Geometry + JS + piece press (+ PoW when required) are hard gates. Trajectory and browser signals move persistent risk. Warmup clears only on **clean** success. Calibrate with `Calibrator` before tightening `HardRejectScore`.

## Browser side

[`client/antibot-client.js`](client/) — PointerEvent + piece press, JS challenge, PoW, verify POST. Demo surfaces `retry_after_ms` on locked / bad_answer.

PoW: `sha256(challengeID + ":" + sessionBind + ":" + salt + ":" + nonce)` with `difficulty` leading zero bits.  
Optional high-risk **stretch** PoW (`kind=stretch`) mixes a memory buffer before the same leading-zero check.  
JS: rotating workloads (`probe` / `loop` / `mix`) → `sha256(nonce|id|token|workDigest|probeValue)`.  
Hard Mode: when global bad-answer rate spikes, Issue returns `hard_mode` (extra PoW bits, shorter TTL, suggested denser slide slots).  
Replay: successful solves fingerprint trajectories per `/32` (soft risk on reuse).  
`RequireBrowserSignals()` is the preferred name for UA/JS gates (RejectObviousAutomation — not attestation).  
`ReputationProvider` plugs external DeviceKey/account risk into Issue.  
Invisible: `EnableInvisible` + `PreferInvisible` when risk ≤ `InvisibleMaxRisk`.  
A11Y: `AllowA11YKeyboard` + client `buildA11YTrajectory`. Soft fingerprints optional via `collectSoftFingerprints`.  
`GenerateSecretKey` / `PrivacyHash` for key minting and hashed identifier storage.

### SecretKey

Must be **≥ 32 cryptographically random bytes** — prefer `GenerateSecretKey(32)`. Weak keys are rejected (`ErrWeakSecretKey`).

## Store requirements

`Store` must provide atomic `Incr` / `IncrBy`, `GetDel`, and `SetNX`. `GeometryStore` adds `IssueChallengeAtomic`, `ClaimGeometryAtomic`, `FinalizeFailureAtomic`, `ReleaseGeoLock`. `RedisStore` uses Lua + `GETDEL` (Redis ≥ 6.2). `MemoryStore` is single-process only.
