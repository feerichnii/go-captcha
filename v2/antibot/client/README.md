# antibot-client.js

Browser companion for [`v2/antibot`](../). Single ES module, no dependencies.

| Export | Purpose |
|---|---|
| `TrajectoryTracker` | records PointerEvent fields (`pointer_type`, `buttons`, `pressure`, `coalesced`, …) + down/move/up events; downsampled and capped |
| `collectBrowserSignals` | webdriver / headless / hardware / plugins hints |
| `solveJSChallenge` | `sha256(nonce + "|" + probeValue)` for `IssueResponse.js_challenge` |
| `solvePoW(salt, difficulty)` | leading-zero-bits of `sha256(salt + ":" + nonce)`; Web Workers with inline fallback |
| `solvePoWInline` / `verifyPoW` / `leadingZeroBits` | building blocks, mirror the Go side |
| `AntiBotClient` | `issue()` → PoW + JS challenge in background → `verify()` posts `{id, answer, trajectory, pow_nonce, browser}` |

## Usage

```html
<script type="module">
import { AntiBotClient, TrajectoryTracker } from "/static/antibot-client.js";

const ab = new AntiBotClient({
  issueUrl: "/captcha/issue",
  verifyUrl: "/captcha/verify",
  headers: { "X-CSRF-Token": csrf },
  onPoWStart: () => spinner.show(),
  onPoWDone:  () => spinner.hide(),
});

const ch = await ab.issue({ kind: "slide" });
// render ch.master / ch.tile; server Set-Cookie: gocaptcha_sid (HttpOnly)

const tracker = new TrajectoryTracker(sliderEl).start();
sliderEl.addEventListener("pointerup", async () => {
  tracker.stop();
  const res = await ab.verify(ch, { x: tileX, y: tileY }, tracker.snapshot());
  if (res.ok) { /* proceed */ } else { /* show generic error, re-issue */ }
}, { once: true });
</script>
```

The server handlers this expects are in [`example_http_test.go`](../example_http_test.go) (`EnsureSessionCookie` — never `RemoteAddr` as ClientKey).

## Wire contract

Verify body:

```json
{
  "id": "32-hex",
  "answer": { "x": 123, "y": 80 },
  "trajectory": {
    "points": [{
      "x": 1.5, "y": 2, "t": 1725460000000,
      "pointer_type": "mouse", "buttons": 1, "pressure": 0.5, "coalesced": 2
    }],
    "events": ["pointerdown", "pointermove", "pointerup"]
  },
  "pow_nonce": "48213",
  "browser": {
    "webdriver": false,
    "languages": ["en-US", "en"],
    "js_challenge_response": "hex-sha256"
  }
}
```

- `answer` is `antibot.SlideSubmit` / `ClickSubmit` / `RotateSubmit`
- `t` is epoch ms; must be monotonic; total duration must fit inside server-observed elapsed time
- `events` should include down → move → up order
- `pow_nonce` ≤ 64 chars when `pow` was present on issue (including probe PoW)

## PoW cost

WebCrypto SHA-256 runs ~50–200k hashes/s per worker. Expected work is `2^difficulty` hashes:

| difficulty | expected hashes | typical time (4 workers) |
|---|---|---|
| 10 (probe) | 1k | ~5 ms |
| 14 | 16k | ~30 ms |
| 18 | 262k | ~0.5 s |
| 22 | 4M | ~8 s |

Solving starts at `issue()` and overlaps with the user's interaction.

## Tests

```bash
node --test v2/antibot/client/antibot-client.test.mjs
```
