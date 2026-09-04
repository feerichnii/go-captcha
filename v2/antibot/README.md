# antibot

AntiBot layer around go-captcha: challenge lifecycle, trajectory risk scoring, rate limits, and adaptive proof-of-work.

```
AntiBot layer
├── Challenge Manager   crypto ID, Redis/Memory, TTL 90s, atomic attempts (3), atomic single-use
├── Answer storage      AES-256-GCM, bound to challenge id — never plaintext, never sent to client
├── Session binding     server-issued cookie (sid:…) as ClientKey — never IP:port
├── Client signals      IP / UA / ASN / session age as risk inputs only
├── Trajectory scoring  order, monotonic t, jumps, PointerEvent meta, coalesced → RISK signal
├── Browser signals     webdriver/headless hints + DOM/JS challenge
├── Rate Limiter        per client key, on Issue AND Verify
├── Server-side timing  MinSolveTime, trajectory-vs-elapsed consistency, input size caps
├── Adaptive PoW        risk level → difficulty + probe PoW + jitter (MaxRiskLevel reaches PoWMax)
├── Risk engine         trajectory + fail-rate + issue frequency + session age + UA/ASN
└── Telemetry           events for logging/metrics + Calibrator for threshold tuning
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
    SecretKey:   secret,
    TTL:         90 * time.Second,
    MaxAttempts: 3,
})

// In HTTP handlers: mint/parse a session cookie, never use RemoteAddr as ClientKey.
sess, _, _ := antibot.EnsureSessionCookie(w, r, secret, antibot.DefaultSessionCookie, antibot.DefaultSessionTTL)
signals := antibot.SignalsFromRequest(r, sess)

// Issue — after slide.Generate():
answer, _ := json.Marshal(captData.GetData()) // secret; encrypted at rest
iss, err := layer.Issue(ctx, antibot.IssueRequest{
    Kind:      antibot.KindSlide,
    Answer:    answer,
    ClientKey: sess.ClientKey,
    Signals:   signals,
})
// → client gets: iss.ID, captData.GetPublicData(), images, iss.PoW, iss.JSChallenge

// Verify:
res, err := layer.Verify(ctx, antibot.VerifyRequest{
    ID:         iss.ID,
    ClientKey:  sess.ClientKey,
    Signals:    signals,
    Browser:    browserFromJSON, // webdriver / js_challenge_response / …
    Answer:     mustJSON(antibot.SlideSubmit{X: ux, Y: uy}),
    Trajectory: antibot.Trajectory{Points: points, Events: events},
    PoWNonce:   nonce, // required if iss.PoW != nil
})
```

### SecretKey

Must be **≥ 32 cryptographically random bytes** (`crypto/rand`). Passphrases, repeated characters, and short strings are rejected (`ErrWeakSecretKey`).

### ClientKey

Use a **server-side session id** (`MintSession` / `EnsureSessionCookie` → `sid:<id>`). Raw IPs and `IP:port` (`net/http.RemoteAddr`) are rejected (`ErrClientKeyLooksLikeIP`). Pass IP/UA via `ClientSignals` / `SignalsFromRequest` (prefer trusted proxy headers over `RemoteAddr`).

### What the client must NOT receive

`GetData()`, `IssueRequest.Answer`, anything from the store. Only `ID`, `GetPublicData()`, images, `PoW`, and `JSChallenge`.

## Behavior score is risk, not proof

The trajectory is client-supplied and can be fabricated or replayed. Therefore:

- geometry (+ PoW when required) is the gate;
- trajectory validation checks `down → move → up`, monotonic timestamps, jump size, final-point proximity, and PointerEvent fields;
- the score and browser signals move the client's **persistent risk level** (atomic Redis/`IncrBy` counter);
- risk also rises on high fail-rate, high issue frequency, young sessions, and UA/headless hints;
- risk level ≥ 1 (or an occasional probe) attaches PoW; difficulty can jitter slightly;
- `MaxRiskLevel` is derived so `PoWMaxDifficulty` is reachable;
- `HardRejectScore` (default off) is available if you deliberately want to fail very low scores.

Calibrate before tightening: feed `VerifyEvent.Score` with your own human/bot labels into `Calibrator`.

## Browser side

[`client/antibot-client.js`](client/) records PointerEvent metadata + coalesced events, collects browser signals, solves the JS challenge and PoW, then posts the verify payload. HTTP handler example: [`example_http_test.go`](example_http_test.go).

PoW contract: `sha256(salt + ":" + nonce)` must have `difficulty` leading zero bits; nonce ≤ `MaxNonceLen` (64) bytes.

JS challenge: `sha256(nonce + "|" + probeValue)` hex — probe defaults to `languages.length`.

## Store requirements

`Store.Incr` / `IncrBy` and `Store.GetDel` must be atomic. Risk counters use `IncrBy` (same pattern as attempts). `RedisStore` uses a Lua `INCRBY`+`PEXPIRE` script and `GETDEL` (Redis ≥ 6.2). `MemoryStore` is single-process only.
