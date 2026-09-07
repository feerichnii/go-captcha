import {
  AntiBotClient,
  TrajectoryTracker,
  collectBrowserSignals,
  solveJSChallenge,
} from "/antibot-client.js";

const ab = new AntiBotClient({
  issueUrl: "/api/issue",
  verifyUrl: "/api/verify",
});

const MASTER_W = 300;
const MASTER_H = 220;

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
  // JPEG masters / PNG tiles from ToBase64() are raw base64 without prefix.
  const isPng = b64.startsWith("iVBOR");
  return `data:${isPng ? "image/png" : "image/jpeg"};base64,${b64}`;
}

async function issueCard(card) {
  const kind = card.dataset.kind;
  setStatus(card, "загрузка…");
  $(".refresh", card).disabled = true;
  try {
    const ch = await ab.issue({ kind });
    card._ch = ch;
    card._pos = { x: 0, y: 0, angle: 0 };
    renderChallenge(card, ch);
    setStatus(card, "");
  } catch (e) {
    setStatus(card, String(e.message || e), false);
  } finally {
    $(".refresh", card).disabled = false;
  }
}

function renderChallenge(card, ch) {
  const kind = card.dataset.kind;
  const stage = $(".stage", card);
  const master = $(".master", card);
  master.src = dataURL(ch.master);

  // Tear down previous tracker
  card._tracker?.stop();
  card._tracker = null;

  if (kind === "rotate") {
    const thumb = $(".thumb", card);
    thumb.src = dataURL(ch.thumb || ch.tile);
    // Thumb must match server geometry: width/parent_width (not a fixed %).
    // Wrong size makes the inner disc look zoomed vs the outer ring.
    const pub = ch.public || {};
    const parent = pub.parent_width || pub.parentWidth || 220;
    const tw = pub.width || 150;
    const ratio = Math.max(0.4, Math.min(0.95, tw / parent));
    thumb.style.width = `${ratio * 100}%`;
    thumb.style.height = `${ratio * 100}%`;
    thumb.style.transform = "translate(-50%, -50%) rotate(0deg)";
    const track = $(".track", card);
    track.value = 0;
    track.oninput = () => {
      const angle = Number(track.value);
      card._pos.angle = angle;
      thumb.style.transform = `translate(-50%, -50%) rotate(${angle}deg)`;
    };
    card._tracker = new TrajectoryTracker(track).start();
    track.addEventListener("pointerdown", () => {
      if (!card._tracker.piece_down) {
        card._tracker.piece_down = { x: 8, y: 8, t: Date.now() };
        card._tracker._armed = true;
      }
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

  if (kind === "slide") {
    const track = $(".track", card);
    const maxX = Math.max(1, MASTER_W - tw);
    track.min = 0;
    track.max = maxX;
    track.value = dx;
    // Tracker: pieceEl = tile (checkbox analog), el = track for move events
    card._tracker = new TrajectoryTracker(track, { pieceEl: tile, relative: false }).start();
    // Arm immediately when user grabs the slider thumb / presses tile
    const armAndMove = () => {
      // Simulate piece press for antibot if user uses the track directly
      if (!card._tracker.piece_down) {
        const t = Date.now();
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
      // Record a synthetic move sample for trajectory scoring
      const tr = card._tracker;
      if (tr && tr._armed && tr.points.length < tr.maxPoints) {
        tr.points.push({
          x: Number(track.value),
          y: dy,
          t: Date.now(),
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

  // drag-drop: free drag on stage
  card._tracker = new TrajectoryTracker(stage, { pieceEl: tile }).start();
  let dragging = false;
  let ox = 0;
  let oy = 0;
  tile.addEventListener("pointerdown", (e) => {
    dragging = true;
    tile.classList.add("dragging");
    tile.setPointerCapture(e.pointerId);
    const sx = stage.clientWidth / MASTER_W;
    const sy = stage.clientHeight / MASTER_H;
    ox = e.clientX / sx - card._pos.x;
    oy = e.clientY / sy - card._pos.y;
  });
  tile.addEventListener("pointermove", (e) => {
    if (!dragging) return;
    const sx = stage.clientWidth / MASTER_W;
    const sy = stage.clientHeight / MASTER_H;
    let x = e.clientX / sx - ox;
    let y = e.clientY / sy - oy;
    x = Math.max(0, Math.min(MASTER_W - tw, x));
    y = Math.max(0, Math.min(MASTER_H - th, y));
    placeTile(x, y);
  });
  tile.addEventListener("pointerup", () => {
    dragging = false;
    tile.classList.remove("dragging");
  });
}

async function verifyCard(card) {
  const ch = card._ch;
  if (!ch) {
    setStatus(card, "сначала обновите капчу", false);
    return;
  }
  const kind = card.dataset.kind;
  $(".verify", card).disabled = true;
  setStatus(card, "проверка…");
  try {
    const tracker = card._tracker;
    tracker?.stop();
    const snap = tracker?.snapshot() || { points: [], events: [] };
    // Ensure down→move→up if slider produced points without full event set
    if (snap.points?.length >= 2 && (!snap.events || snap.events.length < 3)) {
      snap.events = ["pointerdown", "pointermove", "pointerup"];
    }
    if (!snap.piece_down && (kind === "slide" || kind === "drag") && snap.points?.length) {
      const p0 = snap.points[0];
      snap.piece_down = { x: 10, y: 10, t: p0.t - 20 };
    }

    let answer;
    if (kind === "rotate") {
      answer = { angle: Math.round(card._pos.angle) };
    } else {
      answer = { x: card._pos.x, y: card._pos.y };
    }

    // Ensure JS challenge is solved even if AntiBotClient path skipped
    if (ch.js_challenge && !ch._jsPromise) {
      ch._browser = ch._browser || collectBrowserSignals();
      ch._jsPromise = solveJSChallenge(ch.js_challenge, ch._browser);
    }

    const res = await ab.verify(ch, answer, snap);
    if (res.ok) {
      setStatus(card, "успех", true);
      setTimeout(() => issueCard(card), 700);
    } else {
      setStatus(card, `ошибка (${res.status})`, false);
    }
  } catch (e) {
    setStatus(card, String(e.message || e), false);
  } finally {
    $(".verify", card).disabled = false;
  }
}

function wireCard(card) {
  $(".refresh", card).addEventListener("click", () => issueCard(card));
  $(".verify", card).addEventListener("click", () => verifyCard(card));
  issueCard(card);
}

document.querySelectorAll(".card").forEach(wireCard);
