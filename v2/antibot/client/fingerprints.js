/**
 * Optional soft browser fingerprints (canvas / webgl / audio).
 * Never required — empty values are fine. Host may call collectSoftFingerprints()
 * and merge into browser signals before verify.
 */
export async function collectSoftFingerprints() {
  const out = { canvas_hash: "", webgl_hash: "", audio_hash: "" };
  try {
    if (typeof document !== "undefined") {
      const c = document.createElement("canvas");
      c.width = 64;
      c.height = 24;
      const ctx = c.getContext("2d");
      if (ctx) {
        ctx.textBaseline = "top";
        ctx.font = "14px Arial";
        ctx.fillStyle = "#f60";
        ctx.fillRect(0, 0, 64, 24);
        ctx.fillStyle = "#069";
        ctx.fillText(" antibot", 2, 2);
        out.canvas_hash = await sha256Hex(c.toDataURL()).catch(() => "");
      }
    }
  } catch {
    /* ignore */
  }
  try {
    const canvas = typeof document !== "undefined" ? document.createElement("canvas") : null;
    const gl = canvas && (canvas.getContext("webgl") || canvas.getContext("experimental-webgl"));
    if (gl) {
      const dbg = gl.getExtension("WEBGL_debug_renderer_info");
      const vendor = dbg ? gl.getParameter(dbg.UNMASKED_VENDOR_WEBGL) : "";
      const renderer = dbg ? gl.getParameter(dbg.UNMASKED_RENDERER_WEBGL) : "";
      out.webgl_hash = await sha256Hex(`${vendor}|${renderer}`).catch(() => "");
    }
  } catch {
    /* ignore */
  }
  // Audio fingerprint is intentionally omitted by default (privacy); hosts can add.
  return out;
}

async function sha256Hex(str) {
  const data = new TextEncoder().encode(str);
  const subtle = globalThis.crypto?.subtle;
  if (!subtle) return "";
  const buf = new Uint8Array(await subtle.digest("SHA-256", data));
  return Array.from(buf, (b) => b.toString(16).padStart(2, "0")).join("");
}

/**
 * Build a coarse keyboard accessibility trajectory (arrow keys → points).
 * Server still scores it; does not weaken geometry tolerance.
 * @param {{x:number,y:number}} target
 * @param {object} [opts]
 */
export function buildA11YTrajectory(target, opts = {}) {
  const startX = opts.startX ?? 0;
  const startY = opts.startY ?? target.y;
  const step = opts.step ?? 8;
  const t0 = Date.now();
  const points = [];
  const events = ["keydown", "keydown", "keyup"];
  let x = startX;
  let t = t0;
  points.push({ x, y: startY, t, pointer_type: "keyboard", is_trusted: true });
  while (x < target.x) {
    x = Math.min(target.x, x + step);
    t += 40 + Math.floor(Math.random() * 30);
    points.push({ x, y: startY + (Math.random() - 0.5) * 2, t, pointer_type: "keyboard", is_trusted: true });
  }
  points.push({ x: target.x, y: target.y, t: t + 50, pointer_type: "keyboard", is_trusted: true });
  return { points, events, piece_down: { x: 10, y: 10, t: t0 - 80 }, a11y: true };
}
