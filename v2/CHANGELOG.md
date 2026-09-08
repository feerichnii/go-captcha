# Changelog

All notable changes to go-captcha v2 (AntiBot + captcha packages) are documented here.

## Unreleased

### AntiBot checkbox Precheck (P1 UX)
- Stage 1 `PrecheckIssue` / `PrecheckVerify` (separate record, rates, TTL); `RequirePrecheck` gates Issue
- Merged demo path: checkbox → precheck → Generate+Issue → auto Verify on pointerup
- Precheck failures never trigger geometry freeze / badgeo / epoch
- Client `runPrecheck` + protocol **2**; telemetry `OnPrecheck`
- Error codes: `precheck_required`, `precheck_expired`, `unsupported_client`

### AntiBot P0.2
- `StretchPoWRiskMin=0` is truly off (default); stretch issuance requires opt-in + client `stretch-v2` capability
- Client `capabilities.pow` negotiation (`sha256-v1`); JS client single-flight verify + terminal challenge state
- Machine-readable `ErrorCode` / `error_code` in demo + example HTTP handlers
- `PreflightIssue` before image generation; Issue also checks freeze early
- Do not ignore `FinalizeFailure` / `EvaluateRisk` errors on bad geometry
- `GeometryDurationMs` timed from ClaimGeometry only; `HardRejectScore` checked before FinalizeSuccess

### Docs
- Expand `v2/antibot/README.md`: decision flow, hard vs soft checks, delays/TTLs, risk/PoW
- Sync root `README.md` AntiBot section (drop MaxAttempts=3 / attempt-caps wording)

### AntiBot P3
- Optional soft fingerprints (`canvas_hash` / `webgl_hash` / `audio_hash`) — risk only, never required
- `PrivacyHash` + `GenerateSecretKey` / `MustReadCrypto` helpers
- `base/random` fail-closed on `crypto/rand` failure (no time-seed fallback for `RandInt`)
- Invisible mode (`EnableInvisible` + `PreferInvisible`) for low-risk sessions
- A11Y keyboard trajectory helper + React/Vue thin wrappers
- Benchmarks: PoW verify, Issue/Verify happy path, trajectory fingerprint

### AntiBot P2
- Rotating JS workloads (`probe` / `loop` / `mix`) with per-challenge token
- Browser consistency soft-signals; `RequireBrowserSignals()` alias
- Trajectory intervals + dynamics scoring; `isTrusted` diagnostic
- Trajectory replay detection (record on success)
- Optional stretch PoW; `PoWDifficultyForP95Ms`; `ReputationProvider`

### AntiBot P1
- Slide real-slot top-K textured selection
- Bound PoW `challengeID:sessionBind:salt:nonce`
- JS bind to challenge ID; drop `"1","2","3"` fillers
- Success reputation; Dynamic Hard Mode; rotate photometric anti-correlation
- Calibrator ROC / F1 helpers

### AntiBot P0 / P0.1
- One-shot geometry after tech gates; IP `/32` freeze + epoch + single active
- Atomic Issue / Claim / FinalizeFailure / Abort; Redis Cluster `{ipHash}` + cjson
- Distinct session rotation; GeoLockTTL default 5s; README rewrite
