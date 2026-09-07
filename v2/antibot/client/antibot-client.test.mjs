// Node >= 18 test: node --test v2/antibot/client/
import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  leadingZeroBits,
  verifyPoW,
  solvePoWInline,
  solvePoW,
  powPreimage,
  TrajectoryTracker,
  AntiBotClient,
} from "./antibot-client.js";

test("leadingZeroBits matches Go implementation", () => {
  assert.equal(leadingZeroBits([0, 0, 0x0f]), 20);
  assert.equal(leadingZeroBits([0x80]), 0);
  assert.equal(leadingZeroBits([0x01]), 7);
  assert.equal(leadingZeroBits([0, 0]), 16);
});

test("solvePoWInline produces a nonce the server accepts", async () => {
  const pow = {
    challenge_id: "cid",
    bind: "sess",
    salt: "0123456789abcdef0123456789abcdef",
    difficulty: 10,
  };
  const nonce = await solvePoWInline(pow);
  assert.ok(nonce.length <= 64);
  assert.equal(await verifyPoW(pow, nonce, 10), true);
  const h = createHash("sha256").update(powPreimage(pow, nonce)).digest();
  assert.ok(leadingZeroBits(h) >= 10);
});

test("solvePoW falls back inline outside a browser", async () => {
  const pow = { challenge_id: "c", bind: "b", salt: "salt", difficulty: 6 };
  const nonce = await solvePoW(pow);
  assert.equal(await verifyPoW(pow, nonce, 6), true);
});

test("verifyPoW rejects bad or oversized nonces", async () => {
  const pow = { challenge_id: "c", bind: "b", salt: "s" };
  assert.equal(await verifyPoW(pow, "", 4), false);
  assert.equal(await verifyPoW(pow, "x".repeat(65), 1), false);
  assert.equal(await verifyPoW(pow, "anything", 0), true);
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

test("solveJSChallenge matches ExpectedJSResponse contract", async () => {
  const { solveJSChallenge, workloadDigest } = await import("./antibot-client.js");
  const nonce = "00112233445566778899aabbccddeeff";
  const id = "cccccccccccccccccccccccccccccccc";
  const token = "dddddddddddddddddddddddddddddddd";
  const ch = {
    nonce,
    challenge_id: id,
    token,
    seed: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
    workload: "probe",
    probe: "languages.length",
  };
  const hex = await solveJSChallenge(ch, { languages: ["en", "fr"] });
  const work = await workloadDigest(ch);
  const h = createHash("sha256").update(`${nonce}|${id}|${token}|${work}|2`).digest("hex");
  assert.equal(hex, h);
});

test("AntiBotClient issue→verify solves PoW and posts expected shape", async () => {
  const calls = [];
  const pow = { salt: "abcdef", difficulty: 8, challenge_id: "c1", bind: "deadbeef", kind: "sha256" };
  const fakeFetch = async (url, init) => {
    const body = JSON.parse(init.body);
    calls.push({ url, body });
    if (url === "/issue") {
      return {
        ok: true,
        status: 200,
        text: async () =>
          JSON.stringify({
            id: "c1",
            pow,
            js_challenge: {
              nonce: "aa".repeat(16),
              challenge_id: "c1",
              token: "bb".repeat(16),
              seed: "cc".repeat(16),
              workload: "probe",
              probe: "languages.length",
            },
          }),
      };
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
  assert.equal(await verifyPoW(pow, v.pow_nonce, 8), true);
  assert.ok(v.browser);
  assert.ok(typeof v.browser.js_challenge_response === "string" && v.browser.js_challenge_response.length === 64);
});

test("TrajectoryTracker records pointer meta when present", () => {
  const listeners = {};
  const el = {
    addEventListener: (n, fn) => (listeners[n] = fn),
    removeEventListener: (n) => delete listeners[n],
    getBoundingClientRect: () => ({ left: 0, top: 0 }),
  };
  const tr = new TrajectoryTracker(el, { maxPoints: 10, maxEvents: 10, minIntervalMs: 0 }).start();
  listeners.mousedown({
    type: "mousedown",
    clientX: 1,
    clientY: 2,
    pointerType: "mouse",
    pointerId: 1,
    buttons: 1,
    pressure: 0.5,
    isPrimary: true,
    getCoalescedEvents: () => [{}, {}],
  });
  tr.stop();
  const p = tr.snapshot().points[0];
  assert.equal(p.pointer_type, "mouse");
  assert.equal(p.buttons, 1);
  assert.equal(p.coalesced, 2);
});

test("TrajectoryTracker pieceEl requires press before drag", () => {
  const trackListeners = {};
  const pieceListeners = {};
  const track = {
    addEventListener: (n, fn) => (trackListeners[n] = fn),
    removeEventListener: (n) => delete trackListeners[n],
    getBoundingClientRect: () => ({ left: 0, top: 0 }),
  };
  const piece = {
    addEventListener: (n, fn) => (pieceListeners[n] = fn),
    removeEventListener: (n) => delete pieceListeners[n],
    getBoundingClientRect: () => ({ left: 50, top: 40 }),
  };
  const tr = new TrajectoryTracker(track, { pieceEl: piece, minIntervalMs: 0 }).start();
  trackListeners.mousemove({ type: "mousemove", clientX: 60, clientY: 50 });
  assert.equal(tr.snapshot().points.length, 0);
  assert.equal(tr.snapshot().piece_down, undefined);

  pieceListeners.mousedown({
    type: "mousedown",
    clientX: 60,
    clientY: 50,
    pointerType: "mouse",
    buttons: 1,
    pressure: 0.5,
  });
  trackListeners.mousemove({ type: "mousemove", clientX: 80, clientY: 50 });
  trackListeners.mouseup({ type: "mouseup", clientX: 90, clientY: 50 });
  tr.stop();
  const snap = tr.snapshot();
  assert.ok(snap.piece_down);
  assert.deepEqual({ x: snap.piece_down.x, y: snap.piece_down.y }, { x: 10, y: 10 });
  assert.ok(snap.points.length >= 2);
  assert.ok(snap.events.includes("mousedown"));
});
