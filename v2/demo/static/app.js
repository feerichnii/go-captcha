import {
  AntiBotClient,
  TrajectoryTracker,
  collectBrowserSignals,
  solveJSChallenge,
  solvePoW,
} from "/antibot-client.js";

const ab = new AntiBotClient({
  issueUrl: "/api/issue",
  verifyUrl: "/api/verify",
  capabilities: { protocol: 2, pow: ["sha256-v1"] },
});

const MASTER_W = 300;
const MASTER_H = 220;

function nowMs() {
  if (typeof performance !== "undefined" && performance.now) {
    return performance.timeOrigin + performance.now();
  }
  return Date.now();
}

function ensureTrajectory(snap, kind, tileW, tileH) {
  const out = {
    points: Array.isArray(snap?.points) ? snap.points.slice() : [],
    events: Array.isArray(snap?.events) ? snap.events.slice() : [],
    coalesced_total: snap?.coalesced_total || 0,
  };
  if (snap?.piece_down) out.piece_down = { ...snap.piece_down };

  if (out.points.length < 1 && (kind === "slide" || kind === "rotate")) {
    return out;
  }

  const isDown = (e) => /^(pointerdown|mousedown|touchstart)$/i.test(e);
  const isMove = (e) => /^(pointermove|mousemove|touchmove)$/i.test(e);
  const isUp = (e) => /^(pointerup|mouseup|touchend|pointercancel|touchcancel)$/i.test(e);
  let seenDown = false;
  let seenMove = false;
  let seenUp = false;
  for (const e of out.events) {
    if (isDown(e)) seenDown = true;
    else if (isMove(e) && seenDown) seenMove = true;
    else if (isUp(e) && seenDown) seenUp = true;
  }
  if (!(seenDown && seenMove && seenUp)) {
    out.events = ["pointerdown", "pointermove", "pointerup"];
  }

  const tw = Math.max(2, Number(tileW) || 40);
  const th = Math.max(2, Number(tileH) || 40);
  const cx = Math.min(tw, Math.max(1, tw / 2));
  const cy = Math.min(th, Math.max(1, th / 2));
  const p0 = out.points[0];
  const dwellPad = 40;
  if (!out.piece_down && p0) {
    out.piece_down = { x: cx, y: cy, t: p0.t - dwellPad };
  } else if (out.piece_down && p0) {
    if (!(out.piece_down.t > 0) || out.piece_down.t > p0.t - 16) {
      out.piece_down = {
        x: Math.min(tw, Math.max(0, out.piece_down.x ?? cx)),
        y: Math.min(th, Math.max(0, out.piece_down.y ?? cy)),
        t: p0.t - dwellPad,
      };
    }
  }
  return out;
}

function $(sel, root = document) {
  return root.querySelector(sel);
}

function setStatus(card, msg, ok) {
  const el = $(".status", card);
  el.textContent = msg || "";
  el.className = "status" + (ok === true ? " ok" : ok === false ? " err" : "");
}

function dataURL(b64) {
  if (!b64) return "";
  if (b64.startsWith("data:")) return b64;
  const isPng = b64.startsWith("iVBOR");
  return `data:${isPng ? "image/png" : "image/jpeg"};base64,${b64}`;
}

function setVerifyUI(card, { busy, done, label } = {}) {
  const row = $(".verify-row", card);
  const box = $(".verify-box", card);
  const spin = $(".verify-spin", card);
  const text = $(".verify-label", card);
  if (label && text) text.textContent = label;
  if (busy) {
    row.disabled = true;
    row.classList.add("busy");
    row.classList.remove("done");
    if (spin) spin.hidden = false;
    if (box) box.classList.remove("checked");
  } else if (done) {
    row.disabled = true;
    row.classList.remove("busy");
    row.classList.add("done");
    if (spin) spin.hidden = true;
    if (box) box.classList.add("checked");
    if (text) text.textContent = "Проверено";
  } else {
    row.disabled = false;
    row.classList.remove("busy", "done");
    if (spin) spin.hidden = true;
    if (box) box.classList.remove("checked");
    if (text) text.textContent = label || "Проверить решение";
  }
}

function pickKind() {
  return Math.random() < 0.5 ? "rotate" : "slide";
}

function applyKindUI(card, kind) {
  card.dataset.kind = kind;
  const badge = $(".badge", card);
  const hint = $(".hint", card);
  const thumb = $(".thumb", card);
  const tile = $(".tile", card);
  if (badge) badge.textContent = kind.toUpperCase();
  if (hint) {
    hint.textContent =
      kind === "rotate"
        ? "Совмести круги ползунком, затем проверь решение"
        : "Двигай плитку до прорези, затем проверь решение";
  }
  if (thumb) thumb.hidden = kind !== "rotate";
  if (tile) tile.hidden = kind !== "slide";
  // Drop prior track listeners when switching kinds / re-issuing.
  const oldTrack = $(".track", card);
  if (oldTrack) {
    const track = oldTrack.cloneNode(true);
    oldTrack.replaceWith(track);
  }
}

async function issueCard(card) {
  if (card._lockedUntil && Date.now() < card._lockedUntil) {
    const left = Math.max(0, card._lockedUntil - Date.now());
    setStatus(card, `заблокировано, подождите ${Math.ceil(left / 1000)}с`, false);
    return;
  }
  const kind = pickKind();
  applyKindUI(card, kind);
  setStatus(card, "загрузка…");
  setVerifyUI(card, { busy: false, done: false });
  $(".refresh", card).disabled = true;
  $(".verify-row", card).disabled = true;
  try {
    const ch = await ab.issue({ kind });
    card._ch = ch;
    card._pos = { x: 0, y: 0, angle: 0 };
    card._verified = false;
    renderChallenge(card, ch);
    setStatus(card, "");
    setVerifyUI(card, { busy: false, done: false });
  } catch (e) {
    const retry = Number(e.retry_after_ms || e.data?.retry_after_ms || 0);
    if (retry > 0) {
      card._lockedUntil = Date.now() + retry;
      setStatus(card, `заблокировано (${Math.ceil(retry / 1000)}с)`, false);
    } else {
      setStatus(card, String(e.message || e), false);
    }
  } finally {
    $(".refresh", card).disabled = false;
  }
}

function renderChallenge(card, ch) {
  const kind = card.dataset.kind;
  const stage = $(".stage", card);
  const master = $(".master", card);
  master.src = dataURL(ch.master);

  card._tracker?.stop();
  card._tracker = null;

  if (kind === "rotate") {
    const thumb = $(".thumb", card);
    thumb.src = dataURL(ch.thumb || ch.tile);
    const pub = ch.public || {};
    const parent = pub.parent_width || pub.parentWidth || 220;
    const tw = pub.width || 150;
    const th = pub.height || tw;
    const ratio = Math.max(0.4, Math.min(0.95, tw / parent));
    thumb.style.width = `${ratio * 100}%`;
    thumb.style.height = `${ratio * 100}%`;
    thumb.style.transform = "translate(-50%, -50%) rotate(0deg)";
    const track = $(".track", card);
    track.value = 0;
    card._tracker = new TrajectoryTracker(track, { relative: false }).start();
    const armRotate = () => {
      if (!card._tracker.piece_down) {
        const t = Math.round(nowMs());
        card._tracker.piece_down = { x: tw / 2, y: th / 2, t };
        card._tracker._armed = true;
        if (card._tracker.events.length < card._tracker.maxEvents) {
          card._tracker.events.push("pointerdown");
        }
      }
    };
    track.addEventListener("pointerdown", armRotate);
    track.oninput = () => {
      armRotate();
      const angle = Number(track.value);
      card._pos.angle = angle;
      thumb.style.transform = `translate(-50%, -50%) rotate(${angle}deg)`;
      const tr = card._tracker;
      if (tr && tr._armed && tr.points.length < tr.maxPoints) {
        tr.points.push({
          x: angle,
          y: th / 2,
          t: Math.round(nowMs()),
          pointer_type: "mouse",
          buttons: 1,
          pressure: 0.5,
        });
        if (tr.events.length < tr.maxEvents) tr.events.push("pointermove");
      }
    };
    track.addEventListener("pointerup", () => {
      const tr = card._tracker;
      if (tr && tr.events.length < tr.maxEvents) tr.events.push("pointerup");
    });
    return;
  }

  const tile = $(".tile", card);
  tile.src = dataURL(ch.tile);
  const pub = ch.public || {};
  const tw = pub.width || 60;
  const th = pub.height || 60;
  const dx = pub.dx ?? pub.tile_x ?? 0;
  const dy = pub.dy ?? pub.tile_y ?? 0;

  const placeTile = (x, y) => {
    card._pos.x = Math.round(x);
    card._pos.y = Math.round(y);
    const sx = stage.clientWidth / MASTER_W;
    const sy = stage.clientHeight / MASTER_H;
    tile.style.width = `${tw * sx}px`;
    tile.style.height = `${th * sy}px`;
    tile.style.left = `${x * sx}px`;
    tile.style.top = `${y * sy}px`;
  };

  placeTile(dx, dy);

  const track = $(".track", card);
  const maxX = Math.max(1, MASTER_W - tw);
  track.min = 0;
  track.max = maxX;
  track.value = dx;
  card._tracker = new TrajectoryTracker(track, { pieceEl: tile, relative: false }).start();
  const armAndMove = () => {
    if (!card._tracker.piece_down) {
      const t = Math.round(nowMs());
      card._tracker.piece_down = { x: tw / 2, y: th / 2, t };
      card._tracker._armed = true;
      card._tracker.events.push("pointerdown");
    }
  };
  track.addEventListener("pointerdown", armAndMove);
  tile.style.pointerEvents = "auto";
  tile.style.cursor = "grab";
  track.oninput = () => {
    armAndMove();
    placeTile(Number(track.value), dy);
    const tr = card._tracker;
    if (tr && tr._armed && tr.points.length < tr.maxPoints) {
      tr.points.push({
        x: Number(track.value),
        y: dy,
        t: Math.round(nowMs()),
        pointer_type: "mouse",
        buttons: 1,
        pressure: 0.5,
      });
      if (tr.events.length < tr.maxEvents) tr.events.push("pointermove");
    }
  };
  track.addEventListener("pointerup", () => {
    const tr = card._tracker;
    if (tr && tr.events.length < tr.maxEvents) tr.events.push("pointerup");
  });
}

async function verifyCard(card) {
  const ch = card._ch;
  if (!ch || card._verified) {
    if (!ch) setStatus(card, "сначала дождитесь капчи", false);
    return;
  }
  const kind = card.dataset.kind;
  const tracker = card._tracker;
  const raw = tracker?.snapshot() || { points: [], events: [] };
  const pub = ch.public || {};
  const tileW = pub.width || (kind === "rotate" ? 150 : 60);
  const tileH = pub.height || tileW;
  const snap = ensureTrajectory(raw, kind, tileW, tileH);
  let answer;
  if (kind === "rotate") {
    answer = { angle: Math.round(card._pos.angle) };
  } else {
    answer = { x: card._pos.x, y: card._pos.y };
  }

  card._ch = null;
  setVerifyUI(card, { busy: true, label: "Проверка…" });
  setStatus(card, "");

  try {
    tracker?.stop();

    // Hidden tech gates on verify click: ensure JS/PoW from Issue are solved.
    if (ch.js_challenge && !ch._jsPromise) {
      ch._browser = ch._browser || collectBrowserSignals();
      ch._jsPromise = solveJSChallenge(ch.js_challenge, ch._browser);
    }
    if (ch.pow && ch.pow.difficulty > 0 && !ch._powPromise) {
      ch._powPromise = solvePoW(ch.pow);
    }

    const res = await ab.verify(ch, answer, snap);
    if (res.ok) {
      card._verified = true;
      setVerifyUI(card, { done: true });
      setStatus(card, "успех", true);
      setTimeout(() => issueCard(card), 900);
    } else {
      const retry = Number(res.data?.retry_after_ms || 0);
      const errName = res.data?.error_code || res.data?.error || "";
      setVerifyUI(card, { busy: false, done: false });
      if (retry > 0) {
        card._lockedUntil = Date.now() + retry;
        setStatus(card, `${errName || "ошибка"} — пауза ${Math.ceil(retry / 1000)}с`, false);
        setTimeout(() => issueCard(card), retry + 50);
      } else {
        setStatus(card, `ошибка (${errName || res.status})`, false);
        setTimeout(() => issueCard(card), 900);
      }
    }
  } catch (e) {
    setVerifyUI(card, { busy: false, done: false });
    const retry = Number(e.retry_after_ms || e.data?.retry_after_ms || 0);
    if (retry > 0) {
      card._lockedUntil = Date.now() + retry;
      setStatus(card, String(e.message || e), false);
      setTimeout(() => issueCard(card), retry + 50);
    } else {
      setStatus(card, String(e.message || e), false);
      setTimeout(() => issueCard(card), 900);
    }
  }
}

function wireCard(card) {
  $(".refresh", card).addEventListener("click", () => issueCard(card));
  $(".verify-row", card).addEventListener("click", () => verifyCard(card));
  issueCard(card);
}

document.querySelectorAll(".card").forEach(wireCard);
