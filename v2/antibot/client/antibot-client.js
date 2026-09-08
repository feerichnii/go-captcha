/**
 * antibot-client.js — browser companion for go-captcha/v2/antibot.
 *
 * Zero dependencies, ES module. Works in modern browsers and Node >= 18
 * (for tests). Pieces:
 *
 *   1. TrajectoryTracker — PointerEvent fields + coalesced events + down/move/up
 *   2. collectBrowserSignals — webdriver / headless / hardware hints
 *   3. solveJSChallenge — DOM/JS probe response for IssueResponse.js_challenge
 *   4. solvePoW — leading-zero-bits SHA-256 (WebWorker when available)
 *   5. AntiBotClient — glue: issue → track → solve → verify
 *
 * Wire format matches antibot.VerifyRequest on the server:
 *   { id, answer, trajectory: {points, events}, pow_nonce, browser }
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
   * @param {HTMLElement} el          track / master element for move/up
   * @param {object}      [opts]
   * @param {HTMLElement} [opts.pieceEl]  movable tile/knob — press here first (checkbox analog)
   * @param {number}      [opts.maxPoints=1500]
   * @param {number}      [opts.maxEvents=400]
   * @param {number}      [opts.minIntervalMs=8]  drop moves closer than this
   * @param {boolean}     [opts.relative=true]    coordinates relative to el (moves)
   */
  constructor(el, opts = {}) {
    this.el = el;
    this.pieceEl = opts.pieceEl || null;
    this.maxPoints = opts.maxPoints ?? 1500;
    this.maxEvents = opts.maxEvents ?? 400;
    this.minIntervalMs = opts.minIntervalMs ?? 8;
    this.relative = opts.relative ?? true;
    this.reset();
    this._onEvent = this._onEvent.bind(this);
    this._onPieceDown = this._onPieceDown.bind(this);
    this._attached = false;
  }

  reset() {
    this.points = [];
    this.events = [];
    this.piece_down = null;
    this._armed = !this.pieceEl; // without pieceEl, behave as before
    this._lastMoveT = -Infinity;
    this._coalescedTotal = 0;
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
    if (this.pieceEl) {
      this._pieceNames = supportsPointer ? ["pointerdown"] : ["mousedown", "touchstart"];
      for (const n of this._pieceNames) this.pieceEl.addEventListener(n, this._onPieceDown, { passive: true });
    } else {
      this._pieceNames = [];
    }
    this._attached = true;
    return this;
  }

  stop() {
    if (!this._attached) return this;
    for (const n of this._names) this.el.removeEventListener(n, this._onEvent);
    if (this.pieceEl) {
      for (const n of this._pieceNames) this.pieceEl.removeEventListener(n, this._onPieceDown);
    }
    this._attached = false;
    return this;
  }

  /** @returns {{points: object[], events: string[], piece_down?: object, coalesced_total: number}} */
  snapshot() {
    const out = {
      points: this.points.slice(),
      events: this.events.slice(),
      coalesced_total: this._coalescedTotal,
    };
    if (this.piece_down) out.piece_down = { ...this.piece_down };
    return out;
  }

  _coordsOn(el, e) {
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
    if (this.relative && el.getBoundingClientRect) {
      const r = el.getBoundingClientRect();
      x -= r.left;
      y -= r.top;
    }
    return { x: Math.round(x * 100) / 100, y: Math.round(y * 100) / 100 };
  }

  _coords(e) {
    return this._coordsOn(this.el, e);
  }

  _pointerMeta(e) {
    const coalesced =
      typeof e.getCoalescedEvents === "function" ? e.getCoalescedEvents().length : 0;
    this._coalescedTotal += coalesced;
    const meta = {
      pointer_type: e.pointerType || (e.touches ? "touch" : "mouse"),
      pointer_id: typeof e.pointerId === "number" ? e.pointerId : 1,
      buttons: typeof e.buttons === "number" ? e.buttons : undefined,
      pressure: typeof e.pressure === "number" ? e.pressure : undefined,
      tilt_x: typeof e.tiltX === "number" ? e.tiltX : undefined,
      tilt_y: typeof e.tiltY === "number" ? e.tiltY : undefined,
      width: typeof e.width === "number" ? e.width : undefined,
      height: typeof e.height === "number" ? e.height : undefined,
      coalesced,
    };
    if (typeof e.isPrimary === "boolean") meta.is_primary = e.isPrimary;
    if (typeof e.isTrusted === "boolean") meta.is_trusted = e.isTrusted;
    return meta;
  }

  _onPieceDown(e) {
    const t = Math.round(nowMs());
    const { x, y } = this._coordsOn(this.pieceEl, e);
    this.piece_down = { x, y, t };
    this._armed = true;
    if (this.events.length < this.maxEvents) this.events.push(e.type);
    if (this.points.length < this.maxPoints) {
      // Also record a track-relative point so down→move→up stays consistent.
      const track = this._coords(e);
      this.points.push({ ...track, t, ...this._pointerMeta(e) });
    }
  }

  _onEvent(e) {
    const type = e.type;
    const isDown = DOWN_EVENTS.includes(type);
    const isMove = MOVE_EVENTS.includes(type);

    // When pieceEl is set, ignore track downs — arming happens on the piece.
    if (this.pieceEl && isDown) return;
    if (!this._armed) return;

    const t = Math.round(nowMs());
    if (isMove && t - this._lastMoveT < this.minIntervalMs) return;
    if (isMove) this._lastMoveT = t;

    if (this.events.length < this.maxEvents) this.events.push(type);
    if (this.points.length < this.maxPoints) {
      const { x, y } = this._coords(e);
      this.points.push({ x, y, t, ...this._pointerMeta(e) });
    }
  }
}

function nowMs() {
  if (typeof performance !== "undefined" && performance.now) return performance.timeOrigin + performance.now();
  return Date.now();
}

// ---------------------------------------------------------------------------
// Browser signals + JS challenge
// ---------------------------------------------------------------------------

/**
 * Collect untrusted environment hints for the server risk engine.
 * @returns {object} antibot.BrowserSignals shape
 */
export function collectBrowserSignals() {
  const nav = typeof navigator !== "undefined" ? navigator : {};
  const win = typeof window !== "undefined" ? window : {};
  const hints = [];
  const ua = String(nav.userAgent || "").toLowerCase();
  for (const h of ["headless", "phantomjs", "selenium", "webdriver", "puppeteer", "playwright"]) {
    if (ua.includes(h)) hints.push(`ua.${h}`);
  }
  if (nav.webdriver) hints.push("navigator.webdriver");
  if (win.chrome && !win.chrome.runtime) hints.push("chrome.runtime_missing");
  if (typeof win.outerWidth === "number" && win.outerWidth === 0 && win.outerHeight === 0) {
    hints.push("outer_zero");
  }
  const secCH = typeof document !== "undefined" ? "" : ""; // filled by host from headers if needed
  return {
    webdriver: !!nav.webdriver,
    headless_hints: hints,
    languages: Array.from(nav.languages || []),
    platform: String(nav.platform || ""),
    hardware_concurrency: Number(nav.hardwareConcurrency) || 0,
    device_memory: Number(nav.deviceMemory) || 0,
    outer_zero: !!(win.outerWidth === 0 && win.outerHeight === 0),
    plugin_count: nav.plugins ? nav.plugins.length : 0,
    max_touch_points: Number(nav.maxTouchPoints) || 0,
    sec_ch_ua: secCH,
  };
}

async function sha256Hex(str) {
  const data = new TextEncoder().encode(str);
  const subtle = globalThis.crypto?.subtle;
  if (!subtle) throw new Error("antibot: WebCrypto not available (use HTTPS or a modern runtime)");
  const buf = new Uint8Array(await subtle.digest("SHA-256", data));
  return Array.from(buf, (b) => b.toString(16).padStart(2, "0")).join("");
}

/**
 * Deterministic workload digest (mirrors antibot.WorkloadDigest).
 * @param {{workload?:string,seed?:string,token?:string,loop_count?:number}} ch
 */
export async function workloadDigest(ch) {
  const w = ch.workload || "probe";
  if (w === "loop") {
    let h = ch.seed || "";
    const n = ch.loop_count > 0 ? ch.loop_count : 32;
    for (let i = 0; i < n; i++) h = await sha256Hex(h);
    return h;
  }
  if (w === "mix") {
    const full = await sha256Hex(`${ch.token || ""}:${ch.seed || ""}`);
    return full.slice(0, 16);
  }
  return "probe";
}

/**
 * Solve IssueResponse.js_challenge:
 * SHA-256(nonce|challengeID|token|workDigest|probeValue)
 * @param {{nonce:string,challenge_id?:string,token?:string,seed?:string,workload?:string,probe?:string,loop_count?:number}} ch
 * @param {object} [signals]
 */
export async function solveJSChallenge(ch, signals = {}) {
  if (!ch?.nonce) return "";
  const challengeID = ch.challenge_id || ch.challengeId || "";
  const token = ch.token || "";
  let probeValue = "0";
  switch (ch.probe) {
    case "platform":
      probeValue = String(signals.platform || (typeof navigator !== "undefined" ? navigator.platform : "") || "");
      break;
    case "hw":
      probeValue = String(
        signals.hardware_concurrency ||
          (typeof navigator !== "undefined" ? navigator.hardwareConcurrency : 0) ||
          0
      );
      break;
    default: {
      const langs =
        signals.languages ||
        (typeof navigator !== "undefined" ? Array.from(navigator.languages || []) : []);
      probeValue = String(langs.length);
      break;
    }
  }
  const work = await workloadDigest(ch);
  return sha256Hex(`${ch.nonce}|${challengeID}|${token}|${work}|${probeValue}`);
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
 * Bound PoW preimage (mirrors antibot.PoWPreimage).
 * @param {{challenge_id?:string,challengeId?:string,bind?:string,salt:string}} pow
 * @param {string} nonce
 */
export function powPreimage(pow, nonce) {
  const id = pow.challenge_id || pow.challengeId || "";
  const bind = pow.bind || "";
  return `${id}:${bind}:${pow.salt}:${nonce}`;
}

/**
 * Check a candidate nonce (mirrors antibot.VerifyPoW on the server).
 * @param {{challenge_id?:string,bind?:string,salt:string}|string} powOrSalt
 * @param {string} nonceOrBind
 * @param {number|string} difficultyOrSalt
 * @param {number} [maybeDiff]
 * @returns {Promise<boolean>}
 */
export async function verifyPoW(powOrSalt, nonceOrBind, difficultyOrSalt, maybeDiff) {
  let difficulty;
  let nonce;
  if (typeof powOrSalt === "string") {
    nonce = String(nonceOrBind ?? "");
    difficulty = Number(difficultyOrSalt) || 0;
    if (difficulty <= 0) return true;
    if (!powOrSalt || !nonce || nonce.length > 64) return false;
    const h = await sha256(`${powOrSalt}:${nonce}`);
    return leadingZeroBits(h) >= difficulty;
  }
  nonce = String(nonceOrBind ?? "");
  difficulty = Number(difficultyOrSalt) || 0;
  if (difficulty <= 0) return true;
  if (!powOrSalt?.salt || !nonce || nonce.length > 64) return false;
  if (powOrSalt.kind === "stretch") {
    const dig = await stretchDigest(powOrSalt, nonce);
    const bytes = hexToBytes(dig);
    return leadingZeroBits(bytes) >= difficulty;
  }
  const h = await sha256(powPreimage(powOrSalt, nonce));
  return leadingZeroBits(h) >= difficulty;
}

function hexToBytes(hex) {
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  return out;
}

/** Mirrors antibot.StretchDigest (smaller loops for browser). */
export async function stretchDigest(pow, nonce) {
  let mb = pow.memory_mb || 8;
  if (mb > 32) mb = 32;
  if (mb < 1) mb = 1;
  let rounds = pow.rounds || 2;
  if (rounds < 1) rounds = 1;
  const size = mb * 1024 * 1024;
  const buf = new Uint8Array(size);
  let seed = await sha256(powPreimage(pow, nonce));
  buf.set(seed.subarray(0, Math.min(32, size)));
  const subtle = globalThis.crypto.subtle;
  for (let r = 0; r < rounds; r++) {
    let block = seed.slice();
    for (let i = 0; i < size; i += 32) {
      const view = new DataView(block.buffer, block.byteOffset, 4);
      view.setUint32(0, (i ^ r) >>> 0, true);
      block = new Uint8Array(await subtle.digest("SHA-256", block));
      const end = Math.min(i + 32, size);
      for (let j = i; j < end; j++) buf[j] ^= block[(j - i) % 32];
    }
    seed = new Uint8Array(await subtle.digest("SHA-256", buf.subarray(size - 32)));
  }
  const tail = new Uint8Array(35);
  tail.set(seed, 0);
  tail[32] = buf[0];
  tail[33] = buf[(size / 2) | 0];
  tail[34] = buf[size - 1];
  const sum = new Uint8Array(await subtle.digest("SHA-256", tail));
  return Array.from(sum, (b) => b.toString(16).padStart(2, "0")).join("");
}

/**
 * Single-threaded solver. Yields to the event loop every `batch` hashes so the
 * UI stays responsive when no Worker is available.
 *
 * @param {{challenge_id?:string,bind?:string,salt:string,difficulty:number}|string} powOrSalt
 * @param {number} [difficulty] when first arg is salt string (legacy)
 * @param {object} [opts]
 * @returns {Promise<string>} nonce (decimal string, <= 64 chars)
 */
export async function solvePoWInline(powOrSalt, difficulty, opts = {}) {
  let pow;
  if (typeof powOrSalt === "string") {
    pow = { salt: powOrSalt, challenge_id: opts.challenge_id || "", bind: opts.bind || "" };
  } else {
    pow = powOrSalt || {};
    opts = difficulty && typeof difficulty === "object" ? difficulty : opts;
    difficulty = pow.difficulty ?? 0;
  }
  if (difficulty <= 0) return "0";
  if (difficulty > 32) throw new Error(`antibot: difficulty ${difficulty} exceeds cap 32`);
  const start = opts.start ?? 0;
  const step = opts.step ?? 1;
  const batch = opts.batch ?? 64;
  const maxIter = opts.maxIterations ?? 2 ** (difficulty + 6);
  const subtle = globalThis.crypto?.subtle;
  if (!subtle) throw new Error("antibot: WebCrypto not available");

  let n = start;
  for (let i = 0; i < maxIter; i++) {
    if (opts.signal?.aborted) throw new Error("antibot: pow aborted");
    const nonce = String(n);
    let ok = false;
    if (pow.kind === "stretch") {
      const dig = await stretchDigest(pow, nonce);
      ok = leadingZeroBits(hexToBytes(dig)) >= difficulty;
    } else {
      const h = await sha256(powPreimage(pow, nonce));
      ok = leadingZeroBits(h) >= difficulty;
    }
    if (ok) return nonce;
    n += step;
    if (i % batch === batch - 1) {
      opts.onProgress?.(i + 1);
      await new Promise((r) => setTimeout(r, 0));
    }
  }
  throw new Error("antibot: pow search exhausted");
}

const WORKER_SRC = `
function lzb(b){let n=0;for(const x of b){if(x===0){n+=8;continue}n+=Math.clz32(x)-24;break}return n}
self.onmessage=async(e)=>{
  const {prefix,difficulty,start,step}=e.data;const enc=new TextEncoder();
  for(let n=start;;n+=step){
    const h=new Uint8Array(await crypto.subtle.digest('SHA-256',enc.encode(prefix+n)));
    if(lzb(h)>=difficulty){self.postMessage({nonce:String(n)});return}
  }
};`;

/**
 * Solve PoW using Web Workers (one per core, sharded by stride) with an
 * inline fallback. Resolves with the first nonce found.
 * @param {{challenge_id?:string,bind?:string,salt:string,difficulty:number}|string} powOrSalt
 * @param {number} [difficulty]
 * @param {object} [opts]
 */
export async function solvePoW(powOrSalt, difficulty, opts = {}) {
  let pow;
  if (typeof powOrSalt === "string") {
    pow = { salt: powOrSalt, challenge_id: opts.challenge_id || "", bind: opts.bind || "", difficulty };
  } else {
    pow = { ...powOrSalt };
    if (difficulty && typeof difficulty === "object") opts = difficulty;
    else if (typeof difficulty === "number") pow.difficulty = difficulty;
  }
  const diff = pow.difficulty ?? 0;
  if (diff <= 0) return "0";
  // Stretch PoW needs memory-hard digest; the SHA-256 worker pool cannot solve it.
  if (pow.kind === "stretch") return solvePoWInline(pow, diff, opts);
  const canWorker =
    typeof Worker !== "undefined" && typeof Blob !== "undefined" && typeof URL !== "undefined" && URL.createObjectURL;
  if (!canWorker) return solvePoWInline(pow, diff, opts);

  const workers = Math.max(1, Math.min(opts.workers ?? navigator?.hardwareConcurrency ?? 2, 4));
  const timeoutMs = opts.timeoutMs ?? 20000;
  const url = URL.createObjectURL(new Blob([WORKER_SRC], { type: "text/javascript" }));
  const pool = [];
  const prefix = `${pow.challenge_id || pow.challengeId || ""}:${pow.bind || ""}:${pow.salt}:`;

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
        w.postMessage({ prefix, difficulty: diff, start: i, step: workers });
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
 *   POST issueUrl  → { id, expires_at, ttl_seconds, pow?, js_challenge?, ... }
 *   POST verifyUrl ← { id, answer, trajectory, pow_nonce, browser }  → 2xx on success
 *
 * Challenge objects from issue() carry local lifecycle flags:
 *   _verifyInFlight — single-flight guard
 *   _terminal — set after success / bad_geometry / locked / not_found / low_score
 */

const DEFAULT_CAPABILITIES = Object.freeze({
  protocol: 2,
  pow: Object.freeze(["sha256-v1"]),
});

/** Terminal API error_code values — challenge must not be re-verified. */
const TERMINAL_ERROR_CODES = new Set([
  "bad_geometry",
  "bad_answer", // legacy demo wording
  "locked",
  "not_found",
  "low_score",
]);

const PRECHECK_TERMINAL_CODES = new Set([
  "locked",
  "rate_limited",
  "precheck_expired",
  "unsupported_client",
]);

export class AntiBotClient {
  /**
   * @param {object} cfg
   * @param {string} cfg.issueUrl
   * @param {string} cfg.verifyUrl
   * @param {string} [cfg.precheckIssueUrl]
   * @param {string} [cfg.precheckVerifyUrl]
   * @param {typeof fetch} [cfg.fetch]
   * @param {Record<string,string>} [cfg.headers]
   * @param {(pow:{salt:string,difficulty:number})=>void} [cfg.onPoWStart]
   * @param {()=>void} [cfg.onPoWDone]
   * @param {{protocol?:number,pow?:string[]}} [cfg.capabilities]
   */
  constructor(cfg) {
    this.issueUrl = cfg.issueUrl;
    this.verifyUrl = cfg.verifyUrl;
    this.precheckIssueUrl = cfg.precheckIssueUrl ?? "";
    this.precheckVerifyUrl = cfg.precheckVerifyUrl ?? "";
    this.fetch = cfg.fetch ?? globalThis.fetch.bind(globalThis);
    this.headers = cfg.headers ?? {};
    this.onPoWStart = cfg.onPoWStart;
    this.onPoWDone = cfg.onPoWDone;
    this.capabilities = cfg.capabilities ?? DEFAULT_CAPABILITIES;
    this._precheckInFlight = false;
    this._precheckTerminal = null;
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

  _throwHTTP(status, data, fallback) {
    const code = data?.error_code || data?.error;
    const err = new Error(
      code
        ? `antibot: ${code}${data.retry_after_ms ? ` (retry_after_ms=${data.retry_after_ms})` : ""}`
        : `antibot: ${fallback} (${status})`
    );
    err.status = status;
    err.data = data;
    err.error_code = code;
    err.retry_after_ms = data?.retry_after_ms;
    throw err;
  }

  /**
   * Stage-1 checkbox precheck: mint → solve JS/PoW → verify.
   * When precheckVerify returns status "challenge", that interactive challenge is returned.
   * @param {object} [opts]
   * @param {string} [opts.kind] slide|rotate for merged verify→issue
   * @param {object} [opts.interaction] PrecheckInteraction fields
   * @param {object} [opts.browser]
   */
  async runPrecheck(opts = {}) {
    if (!this.precheckIssueUrl || !this.precheckVerifyUrl) {
      const err = new Error("antibot: precheck URLs not configured");
      err.error_code = "invalid_request";
      throw err;
    }
    if (this._precheckTerminal) {
      const err = new Error(`antibot: precheck already ${this._precheckTerminal}`);
      err.error_code = "already_terminal";
      throw err;
    }
    if (this._precheckInFlight) {
      const err = new Error("antibot: precheck already in flight");
      err.error_code = "in_flight";
      throw err;
    }
    this._precheckInFlight = true;
    try {
      const browser = opts.browser || collectBrowserSignals();
      const caps = opts.capabilities ?? this.capabilities;
      const { ok, status, data } = await this._post(this.precheckIssueUrl, {
        capabilities: caps,
        browser,
      });
      if (!ok) {
        const code = data?.error_code || data?.error || "";
        if (PRECHECK_TERMINAL_CODES.has(code)) this._precheckTerminal = code;
        this._throwHTTP(status, data, "precheck issue failed");
      }
      if (data.pow?.kind === "stretch") {
        const err = new Error("antibot: stretch PoW is not supported by this client");
        err.error_code = "unsupported_pow";
        throw err;
      }
      let pow_nonce;
      let js_response;
      if (data.js_challenge) {
        js_response = await solveJSChallenge(data.js_challenge, browser);
      }
      if (data.pow && data.pow.difficulty > 0) {
        this.onPoWStart?.(data.pow);
        try {
          pow_nonce = await solvePoW(data.pow);
        } finally {
          this.onPoWDone?.();
        }
      }
      const ver = await this._post(this.precheckVerifyUrl, {
        kind: opts.kind || "slide",
        precheck_id: data.precheck_id,
        js_response,
        pow_nonce,
        browser: { ...browser, js_challenge_response: js_response },
        interaction: opts.interaction || {},
        capabilities: caps,
      });
      if (!ver.ok) {
        const code = ver.data?.error_code || ver.data?.error || "";
        if (PRECHECK_TERMINAL_CODES.has(code)) this._precheckTerminal = code;
        this._throwHTTP(ver.status, ver.data, "precheck verify failed");
      }
      this._precheckTerminal = null; // allow refresh / new flow after success path
      if (ver.data?.status === "challenge" && ver.data?.id) {
        const ch = {
          ...ver.data,
          _powPromise: null,
          _browser: browser,
          _verifyInFlight: false,
          _terminal: null,
        };
        if (ver.data.js_challenge) {
          ch._jsPromise = solveJSChallenge(ver.data.js_challenge, browser);
          ch._jsPromise.catch(() => {});
        }
        if (ver.data.pow && ver.data.pow.difficulty > 0) {
          this.onPoWStart?.(ver.data.pow);
          ch._powPromise = solvePoW(ver.data.pow).finally(() => this.onPoWDone?.());
          ch._powPromise.catch(() => {});
        }
        return ch;
      }
      return ver.data;
    } finally {
      this._precheckInFlight = false;
    }
  }

  /** Reset checkbox precheck single-flight / terminal (e.g. after cooldown). */
  resetPrecheck() {
    this._precheckInFlight = false;
    this._precheckTerminal = null;
  }

  /**
   * Request a challenge. If the server attaches PoW, start solving it in the
   * background immediately so it overlaps with the user's interaction.
   * Advertises capabilities so the server never issues an unsupported PoW algo.
   */
  async issue(params = {}) {
    const body = {
      ...params,
      capabilities: params.capabilities ?? this.capabilities,
    };
    const { ok, status, data } = await this._post(this.issueUrl, body);
    if (!ok) {
      this._throwHTTP(status, data, "issue failed");
    }
    // Refuse to solve stretch in the browser until stretch-v2 is redesigned.
    if (data.pow?.kind === "stretch") {
      const err = new Error("antibot: stretch PoW is not supported by this client");
      err.error_code = "unsupported_pow";
      throw err;
    }
    const ch = {
      ...data,
      _powPromise: null,
      _browser: collectBrowserSignals(),
      _verifyInFlight: false,
      _terminal: null,
    };
    if (data.js_challenge) {
      ch._jsPromise = solveJSChallenge(data.js_challenge, ch._browser);
      ch._jsPromise.catch(() => {});
    }
    if (data.pow && data.pow.difficulty > 0) {
      this.onPoWStart?.(data.pow);
      ch._powPromise = solvePoW(data.pow).finally(() => this.onPoWDone?.());
      ch._powPromise.catch(() => {});
    }
    return ch;
  }

  /**
   * Submit the answer with trajectory, browser signals and PoW nonce.
   * Single-flight: concurrent/repeat verify of the same challenge is rejected locally.
   * @param {object} ch          object returned by issue()
   * @param {object} answer      antibot.SlideSubmit | RotateSubmit shape
   * @param {{points:any[],events:string[],coalesced_total?:number}} trajectory
   */
  async verify(ch, answer, trajectory) {
    if (!ch || !ch.id) {
      const err = new Error("antibot: missing challenge");
      err.error_code = "invalid_request";
      throw err;
    }
    if (ch._terminal) {
      const err = new Error(`antibot: challenge already ${ch._terminal}`);
      err.error_code = "already_terminal";
      err.terminal = ch._terminal;
      throw err;
    }
    if (ch._verifyInFlight) {
      const err = new Error("antibot: verify already in flight");
      err.error_code = "in_flight";
      throw err;
    }
    ch._verifyInFlight = true;
    try {
      const pow_nonce = ch._powPromise ? await ch._powPromise : undefined;
      const browser = { ...(ch._browser || collectBrowserSignals()) };
      if (trajectory?.coalesced_total != null) browser.coalesced_total = trajectory.coalesced_total;
      if (ch._jsPromise) browser.js_challenge_response = await ch._jsPromise;
      const res = await this._post(this.verifyUrl, {
        id: ch.id,
        answer,
        trajectory: {
          points: trajectory?.points ?? [],
          events: trajectory?.events ?? [],
          piece_down: trajectory?.piece_down,
        },
        pow_nonce,
        browser,
      });
      const code = res.data?.error_code || res.data?.error || "";
      if (res.ok) {
        ch._terminal = "success";
      } else if (TERMINAL_ERROR_CODES.has(code)) {
        ch._terminal = code === "bad_answer" ? "bad_geometry" : code;
      }
      return res;
    } finally {
      ch._verifyInFlight = false;
    }
  }
}

export default AntiBotClient;
