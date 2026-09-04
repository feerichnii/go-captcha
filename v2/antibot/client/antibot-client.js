/**
 * antibot-client.js — browser companion for go-captcha/v2/antibot.
 *
 * Zero dependencies, ES module. Works in modern browsers and Node >= 18
 * (for tests). Three pieces:
 *
 *   1. TrajectoryTracker — records pointer/touch movement as {x, y, t} + events
 *   2. solvePoW          — finds a nonce with N leading zero bits (WebWorker when available)
 *   3. AntiBotClient     — glue: issue → track → solve → verify
 *
 * Wire format matches antibot.VerifyRequest on the server:
 *   { id, client_key?, answer, trajectory: {points, events}, pow_nonce }
 */

// ---------------------------------------------------------------------------
// Trajectory
// ---------------------------------------------------------------------------

const MOVE_EVENTS = ["pointermove", "mousemove", "touchmove"];
const DOWN_EVENTS = ["pointerdown", "mousedown", "touchstart"];
const UP_EVENTS = ["pointerup", "mouseup", "touchend", "pointercancel", "touchcancel"];

/**
 * Records a pointer trajectory relative to an element.
 *
 * Server caps: MaxTrajectoryPoints (default 2000) and MaxTrajectoryEvents (500).
 * We downsample moves to at most one per `minIntervalMs` and hard-cap the
 * arrays so a long drag never gets rejected as ErrInvalidRequest.
 */
export class TrajectoryTracker {
  /**
   * @param {HTMLElement} el          element to attach listeners to
   * @param {object}      [opts]
   * @param {number}      [opts.maxPoints=1500]
   * @param {number}      [opts.maxEvents=400]
   * @param {number}      [opts.minIntervalMs=8]  drop moves closer than this
   * @param {boolean}     [opts.relative=true]    coordinates relative to el
   */
  constructor(el, opts = {}) {
    this.el = el;
    this.maxPoints = opts.maxPoints ?? 1500;
    this.maxEvents = opts.maxEvents ?? 400;
    this.minIntervalMs = opts.minIntervalMs ?? 8;
    this.relative = opts.relative ?? true;
    this.reset();
    this._onEvent = this._onEvent.bind(this);
    this._attached = false;
  }

  reset() {
    this.points = [];
    this.events = [];
    this._lastMoveT = -Infinity;
  }

  start() {
    if (this._attached) return this;
    this.reset();
    const supportsPointer = typeof window !== "undefined" && "PointerEvent" in window;
    const names = supportsPointer
      ? ["pointerdown", "pointermove", "pointerup", "pointercancel"]
      : ["mousedown", "mousemove", "mouseup", "touchstart", "touchmove", "touchend", "touchcancel"];
    this._names = names;
    for (const n of names) this.el.addEventListener(n, this._onEvent, { passive: true });
    this._attached = true;
    return this;
  }

  stop() {
    if (!this._attached) return this;
    for (const n of this._names) this.el.removeEventListener(n, this._onEvent);
    this._attached = false;
    return this;
  }

  /** @returns {{points: {x:number,y:number,t:number}[], events: string[]}} */
  snapshot() {
    return { points: this.points.slice(), events: this.events.slice() };
  }

  _coords(e) {
    let x, y;
    if (e.touches && e.touches.length) {
      x = e.touches[0].clientX;
      y = e.touches[0].clientY;
    } else if (e.changedTouches && e.changedTouches.length) {
      x = e.changedTouches[0].clientX;
      y = e.changedTouches[0].clientY;
    } else {
      x = e.clientX;
      y = e.clientY;
    }
    if (this.relative && this.el.getBoundingClientRect) {
      const r = this.el.getBoundingClientRect();
      x -= r.left;
      y -= r.top;
    }
    return { x: Math.round(x * 100) / 100, y: Math.round(y * 100) / 100 };
  }

  _onEvent(e) {
    const t = Math.round(nowMs());
    const type = e.type;
    const isMove = MOVE_EVENTS.includes(type);

    if (isMove && t - this._lastMoveT < this.minIntervalMs) return;
    if (isMove) this._lastMoveT = t;

    if (this.events.length < this.maxEvents) this.events.push(type);
    if (this.points.length < this.maxPoints) {
      const { x, y } = this._coords(e);
      this.points.push({ x, y, t });
    }
  }
}

function nowMs() {
  if (typeof performance !== "undefined" && performance.now) return performance.timeOrigin + performance.now();
  return Date.now();
}

// ---------------------------------------------------------------------------
// Proof of work
// ---------------------------------------------------------------------------

/** Count leading zero bits of a byte array. */
export function leadingZeroBits(bytes) {
  let bits = 0;
  for (const b of bytes) {
    if (b === 0) {
      bits += 8;
      continue;
    }
    bits += Math.clz32(b) - 24;
    break;
  }
  return bits;
}

async function sha256(str) {
  const data = new TextEncoder().encode(str);
  const subtle = globalThis.crypto?.subtle;
  if (!subtle) throw new Error("antibot: WebCrypto not available (use HTTPS or a modern runtime)");
  return new Uint8Array(await subtle.digest("SHA-256", data));
}

/**
 * Check a candidate nonce (mirrors antibot.VerifyPoW on the server).
 * @returns {Promise<boolean>}
 */
export async function verifyPoW(salt, nonce, difficulty) {
  if (difficulty <= 0) return true;
  if (!salt || !nonce || nonce.length > 64) return false;
  const h = await sha256(`${salt}:${nonce}`);
  return leadingZeroBits(h) >= difficulty;
}

/**
 * Single-threaded solver. Yields to the event loop every `batch` hashes so the
 * UI stays responsive when no Worker is available.
 *
 * @param {string} salt
 * @param {number} difficulty leading zero bits (server sends 14–22)
 * @param {object} [opts]
 * @param {number} [opts.start=0]        starting nonce
 * @param {number} [opts.step=1]         stride (for sharding across workers)
 * @param {number} [opts.batch=256]      hashes between yields
 * @param {number} [opts.maxIterations]  give up after this many (default: 2^(difficulty+6))
 * @param {AbortSignal} [opts.signal]
 * @param {(n:number)=>void} [opts.onProgress]
 * @returns {Promise<string>} nonce (decimal string, <= 64 chars)
 */
export async function solvePoWInline(salt, difficulty, opts = {}) {
  if (difficulty <= 0) return "0";
  if (difficulty > 32) throw new Error(`antibot: difficulty ${difficulty} exceeds cap 32`);
  const start = opts.start ?? 0;
  const step = opts.step ?? 1;
  const batch = opts.batch ?? 256;
  const maxIter = opts.maxIterations ?? 2 ** (difficulty + 6);
  const enc = new TextEncoder();
  const subtle = globalThis.crypto?.subtle;
  if (!subtle) throw new Error("antibot: WebCrypto not available");

  let n = start;
  for (let i = 0; i < maxIter; i++) {
    if (opts.signal?.aborted) throw new Error("antibot: pow aborted");
    const h = new Uint8Array(await subtle.digest("SHA-256", enc.encode(`${salt}:${n}`)));
    if (leadingZeroBits(h) >= difficulty) return String(n);
    n += step;
    if (i % batch === batch - 1) {
      opts.onProgress?.(i + 1);
      await new Promise((r) => setTimeout(r, 0));
    }
  }
  throw new Error("antibot: pow search exhausted");
}

// Worker source is embedded so the module stays a single file.
const WORKER_SRC = `
function lzb(b){let n=0;for(const x of b){if(x===0){n+=8;continue}n+=Math.clz32(x)-24;break}return n}
self.onmessage=async(e)=>{
  const {salt,difficulty,start,step}=e.data;const enc=new TextEncoder();
  for(let n=start;;n+=step){
    const h=new Uint8Array(await crypto.subtle.digest('SHA-256',enc.encode(salt+':'+n)));
    if(lzb(h)>=difficulty){self.postMessage({nonce:String(n)});return}
  }
};`;

/**
 * Solve PoW using Web Workers (one per core, sharded by stride) with an
 * inline fallback. Resolves with the first nonce found.
 *
 * @param {string} salt
 * @param {number} difficulty
 * @param {object} [opts]
 * @param {number} [opts.workers]   default: navigator.hardwareConcurrency (max 4)
 * @param {number} [opts.timeoutMs=20000]
 * @param {AbortSignal} [opts.signal]
 * @returns {Promise<string>}
 */
export async function solvePoW(salt, difficulty, opts = {}) {
  if (difficulty <= 0) return "0";
  const canWorker =
    typeof Worker !== "undefined" && typeof Blob !== "undefined" && typeof URL !== "undefined" && URL.createObjectURL;
  if (!canWorker) return solvePoWInline(salt, difficulty, opts);

  const workers = Math.max(1, Math.min(opts.workers ?? navigator?.hardwareConcurrency ?? 2, 4));
  const timeoutMs = opts.timeoutMs ?? 20000;
  const url = URL.createObjectURL(new Blob([WORKER_SRC], { type: "text/javascript" }));
  const pool = [];

  const cleanup = () => {
    for (const w of pool) w.terminate();
    URL.revokeObjectURL(url);
  };

  try {
    return await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error("antibot: pow timeout")), timeoutMs);
      opts.signal?.addEventListener("abort", () => reject(new Error("antibot: pow aborted")), { once: true });
      for (let i = 0; i < workers; i++) {
        const w = new Worker(url);
        pool.push(w);
        w.onmessage = (e) => {
          clearTimeout(timer);
          resolve(e.data.nonce);
        };
        w.onerror = (e) => {
          clearTimeout(timer);
          reject(e.error || new Error("antibot: worker error"));
        };
        w.postMessage({ salt, difficulty, start: i, step: workers });
      }
    });
  } finally {
    cleanup();
  }
}

// ---------------------------------------------------------------------------
// Client glue
// ---------------------------------------------------------------------------

/**
 * High-level flow helper. Your server endpoints are expected to return:
 *
 *   POST issueUrl  → { id, expires_at, ttl_seconds, pow?: {salt, difficulty}, ...captcha public data / images }
 *   POST verifyUrl ← { id, answer, trajectory, pow_nonce }  → 2xx on success
 *
 * @example
 *   const ab = new AntiBotClient({ issueUrl: "/captcha/issue", verifyUrl: "/captcha/verify" });
 *   const ch = await ab.issue({ kind: "slide" });       // render ch.master/ch.tile, ch.dx/dy...
 *   const tracker = new TrajectoryTracker(sliderEl).start();
 *   ...user drags...
 *   tracker.stop();
 *   const ok = await ab.verify(ch, { x: 123, y: 80 }, tracker.snapshot());
 */
export class AntiBotClient {
  /**
   * @param {object} cfg
   * @param {string} cfg.issueUrl
   * @param {string} cfg.verifyUrl
   * @param {typeof fetch} [cfg.fetch]
   * @param {Record<string,string>} [cfg.headers]  e.g. CSRF token
   * @param {(pow:{salt:string,difficulty:number})=>void} [cfg.onPoWStart]
   * @param {()=>void} [cfg.onPoWDone]
   */
  constructor(cfg) {
    this.issueUrl = cfg.issueUrl;
    this.verifyUrl = cfg.verifyUrl;
    this.fetch = cfg.fetch ?? globalThis.fetch.bind(globalThis);
    this.headers = cfg.headers ?? {};
    this.onPoWStart = cfg.onPoWStart;
    this.onPoWDone = cfg.onPoWDone;
  }

  async _post(url, body) {
    const res = await this.fetch(url, {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", ...this.headers },
      body: JSON.stringify(body),
    });
    const text = await res.text();
    let data = null;
    try {
      data = text ? JSON.parse(text) : null;
    } catch {
      data = { raw: text };
    }
    return { ok: res.ok, status: res.status, data };
  }

  /**
   * Request a challenge. If the server attaches PoW, start solving it in the
   * background immediately so it overlaps with the user's interaction.
   */
  async issue(params = {}) {
    const { ok, status, data } = await this._post(this.issueUrl, params);
    if (!ok) throw new Error(`antibot: issue failed (${status})`);
    const ch = { ...data, _powPromise: null };
    if (data.pow && data.pow.difficulty > 0) {
      this.onPoWStart?.(data.pow);
      ch._powPromise = solvePoW(data.pow.salt, data.pow.difficulty).finally(() => this.onPoWDone?.());
      // Avoid unhandled rejection if verify() is never called.
      ch._powPromise.catch(() => {});
    }
    return ch;
  }

  /**
   * Submit the answer with trajectory (and PoW nonce when required).
   * @param {object} ch          object returned by issue()
   * @param {object} answer      antibot.ClickSubmit | SlideSubmit | RotateSubmit shape
   * @param {{points:any[],events:string[]}} trajectory
   * @returns {Promise<{ok:boolean,status:number,data:any}>}
   */
  async verify(ch, answer, trajectory) {
    const pow_nonce = ch._powPromise ? await ch._powPromise : undefined;
    return this._post(this.verifyUrl, {
      id: ch.id,
      answer,
      trajectory: {
        points: trajectory?.points ?? [],
        events: trajectory?.events ?? [],
      },
      pow_nonce,
    });
  }
}

export default AntiBotClient;
