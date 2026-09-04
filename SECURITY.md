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

- **SecretKey** ≥ 32 high-entropy bytes (`ValidateSecretKey`); weak/short keys are rejected.
- **ClientKey** is a server-issued session cookie (`EnsureSessionCookie` / `sid:…`). IP:port / raw IPs are rejected; pass IP/UA via `ClientSignals` only.
- Atomic attempt counter (`Incr`) and atomic risk counter (`IncrBy`); atomic consume (`GetDel`).
- Challenge bound to the issuing session; rate limits on both Issue and Verify.
- Trajectory validation: event order, monotonic timestamps, jump size, final-point check, PointerEvent fields / coalesced events.
- Browser signals: webdriver/headless hints + DOM/JS challenge; fail-rate / issue-frequency / session age feed the risk engine.
- Adaptive PoW with probe probability + jitter; `MaxRiskLevel` is derived so `PoWMaxDifficulty` is reachable.
- Behavior score is a **risk signal** — not a proof of humanity. See [v2/antibot/README.md](v2/antibot/README.md).

## Generation hardening

- Answer geometry, characters and ordering use `crypto/rand` (`random.RandInt` / `Perm`, buffered, unbiased).
- Click: thumb glyph deformation on by default; interference lines/speckles on the master.
- Slide: decoy shadows (default 3); alpha jitter on tile edges.
- Rotate: independent luminance noise fields on master and thumb (they do not align under any rotation, so plain correlation solvers need to average it out).
- JPEG defaults use quality 85 instead of 100.

## Recommended app controls

1. Always use `EnsureSessionCookie` (or equivalent) for `ClientKey`; never `r.RemoteAddr`. Prefer trusted proxy headers for `ClientSignals.IP`.
2. Generate `SecretKey` with `crypto/rand` (32+ bytes); rotate periodically — in-flight challenges (≤ TTL) fail closed on rotation.
3. Show a single generic failure to users; log the typed error (`antibot.IsClientError`).
4. Diverse assets: many backgrounds / fonts / graphs; small fixed asset sets help solvers.
5. Calibrate `RiskThreshold` on your own traffic before enabling `HardRejectScore`.
6. Keep the bundled JS client (`antibot-client.js`) so PointerEvent meta, coalesced events and the JS challenge are actually sent.
## Out of scope

HTTP/gRPC transport, WAF, ML risk models — wire those in your app or [go-captcha-service](https://github.com/wenlng/go-captcha-service).
