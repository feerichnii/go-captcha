# antibot-client.js

Browser companion for [`v2/antibot`](../). Single ES module, no dependencies.

| Export | Purpose |
|---|---|
| `TrajectoryTracker` | records `{x, y, t}` + pointer events on an element, downsampled and capped to server limits |
| `solvePoW(salt, difficulty)` | finds a nonce with `difficulty` leading zero bits of `sha256(salt + ":" + nonce)`; Web Workers (up to 4) with inline fallback |
| `solvePoWInline` / `verifyPoW` / `leadingZeroBits` | building blocks, mirror the Go side exactly |
| `AntiBotClient` | `issue()` → starts PoW in background if required → `verify()` posts `{id, answer, trajectory, pow_nonce}` |

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
// render ch.master / ch.tile, place the tile at ch.public.dx / ch.public.dy

const tracker = new TrajectoryTracker(sliderEl).start();
sliderEl.addEventListener("pointerup", async () => {
  tracker.stop();
  const res = await ab.verify(ch, { x: tileX, y: tileY }, tracker.snapshot());
  if (res.ok) { /* proceed */ } else { /* show generic error, re-issue */ }
}, { once: true });
</script>
```

The server handlers this expects are in [`example_http_test.go`](../example_http_test.go).

## Wire contract

Verify body:

```json
{
  "id": "32-hex",
  "answer": { "x": 123, "y": 80 },
  "trajectory": { "points": [{ "x": 1.5, "y": 2, "t": 1725460000000 }], "events": ["pointerdown", "pointermove", "pointerup"] },
  "pow_nonce": "48213"
}
```

- `answer` is `antibot.SlideSubmit` / `ClickSubmit` (`{"points":[{"x":..,"y":..}]}`, ordered) / `RotateSubmit` (`{"angle":..}`)
- `t` is epoch ms; only differences matter, and the total must fit inside server-observed elapsed time
- `pow_nonce` ≤ 64 chars, decimal string from the solver

## PoW cost

WebCrypto SHA-256 runs ~50–200k hashes/s per worker. Expected work is `2^difficulty` hashes:

| difficulty | expected hashes | typical time (4 workers) |
|---|---|---|
| 14 | 16k | ~30 ms |
| 18 | 262k | ~0.5 s |
| 22 | 4M | ~8 s |

Solving starts at `issue()` and overlaps with the user's interaction, so it is rarely visible below ~18.

## Tests

```bash
node --test v2/antibot/client/antibot-client.test.mjs
```
