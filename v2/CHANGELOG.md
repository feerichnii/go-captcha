# Changelog

All notable changes to go-captcha v2 (AntiBot + captcha packages) are documented here.

## Unreleased

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
