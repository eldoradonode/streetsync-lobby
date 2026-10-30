// Step 19: small pure helpers for pins, reactions and polls. No DOM, no network, so they are easy to test.
// The server enforces every rule again; these only make the page answer quickly.

export const REACTIONS = ['\u{1F44D}', '\u2764\uFE0F', '\u{1F602}', '\u{1F62E}', '\u{1F622}', '\u{1F64F}'];
export const POLL_LIMITS = { qMin: 3, qMax: 140, optMin: 2, optMax: 6, optLen: 60 };

const squeeze = s => String(s || '').replace(/[\u0000-\u001F\u007F]/g, ' ').split(/\s+/).filter(Boolean).join(' ');
const runes = s => Array.from(s).length;

// Returns { ok: true, q, opts } or { ok: false, err } with a message the person can read.
export function validatePoll(question, options) {
  const q = squeeze(question);
  if (runes(q) < POLL_LIMITS.qMin) return { ok: false, err: 'Write a question of at least ' + POLL_LIMITS.qMin + ' characters.' };
  if (runes(q) > POLL_LIMITS.qMax) return { ok: false, err: 'Keep the question under ' + POLL_LIMITS.qMax + ' characters.' };
  const opts = (options || []).map(squeeze).filter(Boolean);
  if (opts.length < POLL_LIMITS.optMin) return { ok: false, err: 'Add at least ' + POLL_LIMITS.optMin + ' options.' };
  if (opts.length > POLL_LIMITS.optMax) return { ok: false, err: 'Use at most ' + POLL_LIMITS.optMax + ' options.' };
  const seen = new Set();
  for (const o of opts) {
    if (runes(o) > POLL_LIMITS.optLen) return { ok: false, err: 'Keep each option under ' + POLL_LIMITS.optLen + ' characters.' };
    const k = o.toLowerCase();
    if (seen.has(k)) return { ok: false, err: 'Each option must be different.' };
    seen.add(k);
  }
  return { ok: true, q, opts };
}

// Whole-number percentages that add up to 100 when there is at least one vote (largest remainder).
export function percents(counts) {
  const total = (counts || []).reduce((a, b) => a + b, 0);
  if (!total) return (counts || []).map(() => 0);
  const raw = counts.map(n => (n * 100) / total);
  const out = raw.map(Math.floor);
  let left = 100 - out.reduce((a, b) => a + b, 0);
  raw.map((v, i) => [v - Math.floor(v), i]).sort((a, b) => b[0] - a[0] || a[1] - b[1]).forEach(([, i]) => { if (left > 0) { out[i]++; left--; } });
  return out;
}

// Applies one {"t":"rx"} frame to the reactions map. Only the sender's own frame changes "my" reaction.
export function applyReaction(map, frame, me) {
  const prev = map[frame.i];
  const mineNow = me && String(frame.h || '').toLowerCase() === String(me).toLowerCase();
  const y = mineNow ? (frame.y || '') : (prev ? prev.y : '');
  const c = frame.c || {};
  if (!Object.keys(c).length && !y) { delete map[frame.i]; return; }
  map[frame.i] = { c, y };
}

// A short line for the pinned banner.
export function pinSnippet(body, max = 90) {
  const r = Array.from(squeeze(body));
  return r.length <= max ? r.join('') : r.slice(0, max).join('') + '\u2026';
}
