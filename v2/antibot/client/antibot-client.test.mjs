// Node >= 18 test: node --test v2/antibot/client/
import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { leadingZeroBits, verifyPoW, solvePoWInline, solvePoW, TrajectoryTracker, AntiBotClient } from "./antibot-client.js";

test("leadingZeroBits matches Go implementation", () => {
  assert.equal(leadingZeroBits([0, 0, 0x0f]), 20);
  assert.equal(leadingZeroBits([0x80]), 0);
  assert.equal(leadingZeroBits([0x01]), 7);
  assert.equal(leadingZeroBits([0, 0]), 16);
});

test("solvePoWInline produces a nonce the server accepts", async () => {
  const salt = "0123456789abcdef0123456789abcdef";
  const nonce = await solvePoWInline(salt, 10);
  assert.ok(nonce.length <= 64);
  assert.equal(await verifyPoW(salt, nonce, 10), true);
  // Cross-check with node:crypto exactly like Go: sha256(salt + ":" + nonce)
  const h = createHash("sha256").update(`${salt}:${nonce}`).digest();
  assert.ok(leadingZeroBits(h) >= 10);
});

test("solvePoW falls back inline outside a browser", async () => {
  const nonce = await solvePoW("salt", 6);
  assert.equal(await verifyPoW("salt", nonce, 6), true);
});

test("verifyPoW rejects bad or oversized nonces", async () => {
  assert.equal(await verifyPoW("s", "", 4), false);
  assert.equal(await verifyPoW("s", "x".repeat(65), 1), false);
  assert.equal(await verifyPoW("s", "anything", 0), true);
});

test("TrajectoryTracker downsamples moves and caps arrays", () => {
  const listeners = {};
  const el = {
    addEventListener: (n, fn) => (listeners[n] = fn),
    removeEventListener: (n) => delete listeners[n],
    getBoundingClientRect: () => ({ left: 10, top: 20 }),
  };
  const tr = new TrajectoryTracker(el, { maxPoints: 5, maxEvents: 3, minIntervalMs: 0 }).start();
  listeners.mousedown({ type: "mousedown", clientX: 15, clientY: 25 });
  for (let i = 0; i < 10; i++) listeners.mousemove({ type: "mousemove", clientX: 20 + i, clientY: 25 });
  listeners.mouseup({ type: "mouseup", clientX: 30, clientY: 25 });
  tr.stop();
  const snap = tr.snapshot();
  assert.equal(snap.points.length, 5);
  assert.equal(snap.events.length, 3);
  assert.deepEqual({ x: snap.points[0].x, y: snap.points[0].y }, { x: 5, y: 5 });
  assert.ok(Object.keys(listeners).length === 0, "listeners removed on stop");
});

test("AntiBotClient issue→verify solves PoW and posts expected shape", async () => {
  const calls = [];
  const salt = "abcdef";
  const fakeFetch = async (url, init) => {
    const body = JSON.parse(init.body);
    calls.push({ url, body });
    if (url === "/issue") {
      return { ok: true, status: 200, text: async () => JSON.stringify({ id: "c1", pow: { salt, difficulty: 8 } }) };
    }
    return { ok: true, status: 200, text: async () => "{}" };
  };
  const ab = new AntiBotClient({ issueUrl: "/issue", verifyUrl: "/verify", fetch: fakeFetch });
  const ch = await ab.issue({ kind: "slide" });
  const res = await ab.verify(ch, { x: 1, y: 2 }, { points: [{ x: 0, y: 0, t: 1 }], events: ["pointerdown"] });
  assert.equal(res.ok, true);
  const v = calls[1].body;
  assert.equal(v.id, "c1");
  assert.deepEqual(v.answer, { x: 1, y: 2 });
  assert.equal(v.trajectory.points.length, 1);
  assert.equal(await verifyPoW(salt, v.pow_nonce, 8), true);
});
