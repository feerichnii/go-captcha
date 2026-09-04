# Security hardening notes

GoCaptcha generates interactive CAPTCHA images. It does **not** implement sessions, rate limits, or HTTP by itself. Bot resistance depends on (1) never leaking answers to clients and (2) app-level TTL / one-shot / attempt limits.

## Critical: never return `GetData()` to clients

`GetData()` contains the secret answer:

| Mode   | Secret fields                          |
|--------|----------------------------------------|
| Click  | `x`, `y`, `text` / `shape`             |
| Slide  | `x`, `y` (target drop position)        |
| Rotate | `angle`                                |

Use **`GetPublicData()`** in API responses (plus images). Persist `GetData()` only in server storage (Redis/DB/session), or seal it with `v2/base/challenge`.

```go
id, _ := challenge.NewID()
raw, _ := json.Marshal(captData.GetData())
token, _ := challenge.SealWithTTL(serverKey, challenge.Payload{
    Kind: "slide",
    Data: raw,
}, 2*time.Minute)

// Client gets: id, token (opaque), images, GetPublicData()
// Server keeps: nothing except key — or stores id→token once
```

## Recommended app controls

1. **TTL** — expire challenges (e.g. 1–2 minutes).
2. **One-shot** — delete the challenge after first successful verify.
3. **Attempt limit** — e.g. 5 fails then issue a new challenge (`challenge.FormatAttemptKey`).
4. **Ordered clicks** — use `click.ValidateOrdered` instead of unordered point checks.
5. **Tight padding** — library validators clamp padding (`DefaultMaxPadding`); do not raise it without reason.
6. **Diverse assets** — many backgrounds / graphs; small fixed asset sets help solvers.

## Library hardening in this fork

- Public vs secret APIs: `GetPublicData()`
- Answer geometry uses `crypto/rand` (`random.RandInt` / `Perm`)
- Click: thumb deformation on by default; master interference noise
- Slide: decoy shadows (default 3); edge jitter on tiles; slight graph angle
- Rotate: rim noise on master circle
- JPEG defaults use quality level 2 (85) instead of 100
- `challenge.Seal` / `Open` HMAC helpers for opaque answer tokens

## Still out of scope (use your service layer)

IP rate limits, WAF, behavioral trajectory checks, ML risk scores, Redis clustering — use [go-captcha-service](https://github.com/wenlng/go-captcha-service) or your own API.
