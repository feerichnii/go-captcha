# antibot

AntiBot layer around go-captcha: challenge lifecycle, trajectory risk scoring, rate limits, and adaptive proof-of-work.

```
AntiBot layer
├── Challenge Manager   crypto ID, Redis/Memory, TTL 90s, atomic attempts (3), atomic single-use
├── Answer storage      AES-256-GCM, bound to challenge id — never plaintext, never sent to client
├── Client binding      challenge usable only by the session/client that requested it
├── Trajectory scoring  duration, velocity, acceleration, timing, corrections → RISK signal
├── Rate Limiter        per client key, on Issue AND Verify
├── Server-side timing  MinSolveTime, trajectory-vs-elapsed consistency, input size caps
├── Adaptive PoW        persistent per-client risk level → difficulty escalation
└── Telemetry           events for logging/metrics + Calibrator for threshold tuning
```

## Quick start

```go
import (
    "encoding/json"
    "time"

    "github.com/feerichnii/go-captcha/v2/antibot"
    "github.com/feerichnii/go-captcha/v2/slide"
    "github.com/redis/go-redis/v9"
)

rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
layer, err := antibot.New(antibot.NewRedisStore(rdb), antibot.Config{
    SecretKey:   []byte(os.Getenv("CAPTCHA_SECRET")), // required, >= 32 random bytes
    TTL:         90 * time.Second,
    MaxAttempts: 3,
})

// Issue — after slide.Generate():
answer, _ := json.Marshal(captData.GetData()) // secret; encrypted at rest
iss, err := layer.Issue(ctx, antibot.IssueRequest{
    Kind:      antibot.KindSlide,
    Answer:    answer,
    ClientKey: sessionID, // or hash(IP + UA); must be the same at Verify
})
// → client gets: iss.ID, captData.GetPublicData(), images, iss.PoW (when risky)

// Verify:
res, err := layer.Verify(ctx, antibot.VerifyRequest{
    ID:         iss.ID,
    ClientKey:  sessionID,
    Answer:     mustJSON(antibot.SlideSubmit{X: ux, Y: uy}),
    Trajectory: antibot.Trajectory{Points: points, Events: events},
    PoWNonce:   nonce, // required if iss.PoW != nil
})
if err != nil {
    if antibot.IsClientError(err) { /* show generic "captcha failed" */ } else { /* 5xx + log */ }
}
// res.Score / res.Risk / res.RiskLevel / res.RequirePoWNext
```

### What the client must NOT receive

`GetData()`, `IssueRequest.Answer`, anything from the store. Only `ID`, `GetPublicData()`, images and `PoW{salt,difficulty}`.

### Error handling

Typed errors (`ErrBadAnswer`, `ErrPoWInvalid`, `ErrTooFast`, ...) are for your logs/telemetry. Show the end user one generic failure; `IsClientError` separates client faults from store/internal errors.

## Behavior score is risk, not proof

The trajectory is client-supplied and can be fabricated or replayed. Therefore:

- geometry (+ PoW when required) is the gate;
- the score only moves the client's **persistent risk level** (`RiskThreshold`, TTL `RiskTTL`);
- risk level ≥ 1 makes the next `Issue` attach PoW at `PoWBaseDifficulty + (level-1)*PoWStepPerLevel` (capped by `PoWMaxDifficulty`);
- clean solves de-escalate one level;
- `HardRejectScore` (default off) is available if you deliberately want to fail very low scores.

Calibrate before tightening: feed `VerifyEvent.Score` with your own human/bot labels into `Calibrator` and use `Report().SuggestedRiskThreshold`.

## Browser side

[`client/antibot-client.js`](client/) records the trajectory, solves PoW in Web Workers and posts the verify payload. HTTP handler example: [`example_http_test.go`](example_http_test.go).

PoW contract: `sha256(salt + ":" + nonce)` must have `difficulty` leading zero bits; nonce ≤ `MaxNonceLen` (64) bytes.

## Store requirements

`Store.Incr` and `Store.GetDel` must be atomic. `RedisStore` uses a Lua `INCR`+`PEXPIRE` script and `GETDEL` (Redis ≥ 6.2). `MemoryStore` is single-process only.
