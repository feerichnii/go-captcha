# Security hardening notes

GoCaptcha generates interactive CAPTCHA images. Bot resistance depends on (1) never leaking answers to clients and (2) challenge lifecycle controls. This fork ships both: hardened generation in `click`/`slide`/`rotate`, and the lifecycle layer in [`v2/antibot`](v2/antibot).

## Critical: never return `GetData()` to clients

| Mode   | Secret fields                          |
|--------|----------------------------------------|
| Click  | `x`, `y`, `text` / `shape`             |
| Slide  | `x`, `y` (target drop position)        |
| Rotate | `angle`                                |

Use **`GetPublicData()`** in API responses. Hand `GetData()` to `antibot.Issue`, which encrypts it (AES-256-GCM, bound to the challenge id) before storing.

If you need a stateless token instead of a store, `challenge.Seal`/`Open` produce AEAD tokens (confidential + authenticated). Even so, prefer keeping answers server-side.

## Lifecycle controls (`v2/antibot`)

- Atomic attempt counter (`Incr` before evaluation) and atomic consume (`GetDel`): concurrent guesses cannot exceed `MaxAttempts`; concurrent correct answers succeed exactly once.
- Challenge bound to the issuing client key; rate limits on both Issue and Verify.
- Server-side timing: `MinSolveTime`, trajectory duration must fit inside server-observed elapsed time.
- Size caps on trajectory, PoW nonce and answer payload; padding tolerances are server-side only.
- Behavior score is a **risk signal** feeding a persistent per-client risk level and adaptive PoW — not a proof of humanity. See [v2/antibot/README.md](v2/antibot/README.md).

## Generation hardening

- Answer geometry, characters and ordering use `crypto/rand` (`random.RandInt` / `Perm`, buffered, unbiased).
- Click: thumb glyph deformation on by default; interference lines/speckles on the master.
- Slide: decoy shadows (default 3); alpha jitter on tile edges.
- Rotate: independent luminance noise fields on master and thumb (they do not align under any rotation, so plain correlation solvers need to average it out).
- JPEG defaults use quality 85 instead of 100.

## Recommended app controls

1. Use a session id as `ClientKey` when available; otherwise hash(IP, UA).
2. Show a single generic failure to users; log the typed error (`antibot.IsClientError`).
3. Rotate `SecretKey` periodically; in-flight challenges (≤ TTL) fail closed on rotation.
4. Diverse assets: many backgrounds / fonts / graphs; small fixed asset sets help solvers.
5. Calibrate `RiskThreshold` on your own traffic before enabling `HardRejectScore`.

## Out of scope

HTTP/gRPC transport, WAF, ML risk models — wire those in your app or [go-captcha-service](https://github.com/wenlng/go-captcha-service).
