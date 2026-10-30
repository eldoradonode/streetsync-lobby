// Every handle gets the same illustrated avatar on every screen, with no image to download.
// Customising avatars with credits comes later. Until then, the handle decides.

// Name colors. The tier badge is the rank signal, so these only tell people apart.
export const PALETTE = ['#ff8a80', '#ffb74d', '#fff176', '#aed581', '#4dd0e1', '#64b5f6', '#b39ddb', '#f48fb1'];

function hash(s) {
  let h = 5381;
  const t = s.toLowerCase();
  for (let i = 0; i < t.length; i++) h = ((h << 5) + h + t.charCodeAt(i)) | 0;
  return Math.abs(h);
}

export const colorFor = handle => PALETTE[hash(handle) % PALETTE.length];

const SKIN = ['#f5d0b0', '#e8b48c', '#d49a6a', '#b87848', '#8d5a34', '#5e3a22'];
const HAIR = ['#1b1b24', '#3b2a20', '#6b4423', '#c28a3a', '#8a8a96', '#7a2f4f'];
const BG = ['#2a2f5c', '#3b2a5c', '#1f4a5c', '#5c2a3b', '#2a5c4a', '#4a3b1f', '#2f2f3f', '#5c4a2a'];
const SHIRT = ['#ff8a80', '#4dd0e1', '#ffb74d', '#64b5f6', '#aed581', '#b39ddb', '#f48fb1', '#fff176'];

// The looks a person can pick at sign up: eight in each of three sets (m, f and x). The server only stores the code, like "f3".
export const LOOKS = ['m', 'f', 'x'];
export const LOOK_COUNT = 8;
const PRESETS = {
  m: [[2, 0, 0, 0, 0, 0, 1], [4, 0, 4, 1, 2, 2, 3], [1, 2, 3, 0, 1, 1, 0], [5, 0, 1, 2, 0, 4, 7], [3, 1, 0, 1, 0, 3, 5], [0, 3, 0, 0, 1, 6, 2], [2, 4, 3, 2, 2, 7, 4], [4, 1, 1, 0, 1, 5, 6]],
  f: [[1, 1, 5, 2, 0, 1, 6], [4, 0, 2, 0, 0, 3, 5], [0, 3, 5, 0, 1, 0, 1], [5, 0, 1, 2, 0, 4, 7], [3, 5, 5, 1, 0, 2, 0], [2, 2, 2, 2, 1, 7, 3], [1, 0, 0, 0, 0, 5, 6], [4, 5, 1, 0, 0, 6, 2]],
  x: [[2, 0, 0, 0, 0, 0, 1], [3, 4, 4, 1, 2, 4, 5], [0, 5, 2, 2, 0, 1, 0], [5, 0, 1, 0, 1, 2, 3], [1, 2, 3, 2, 0, 3, 7], [4, 1, 5, 0, 0, 5, 4], [2, 3, 0, 1, 1, 6, 6], [3, 0, 2, 2, 0, 7, 2]],
};
export const validLook = code => /^[mfx][0-7]$/.test(code || '');

// hairStyle: 0 short, 1 curls, 2 bun, 3 cap, 4 none, 5 long
function fromPreset(code) {
  const [skin, hair, hairStyle, eyes, mouth, bg, shirt] = PRESETS[code[0]][+code[1]];
  return { bg: BG[bg], skin: SKIN[skin], hair: HAIR[hair], shirt: SHIRT[shirt], hairStyle, eyes, mouth };
}

// The look to draw: the one the person picked, or one made from the handle when they never picked.
export function avatarOf(handle, code) {
  if (validLook(code)) return fromPreset(code);
  const h = hash(handle);
  const pick = (list, shift) => list[((h >> shift) ^ (h >> (shift + 3))) % list.length];
  return {
    bg: pick(BG, 1),
    skin: pick(SKIN, 4),
    hair: pick(HAIR, 7),
    shirt: pick(SHIRT, 10),
    hairStyle: (h >> 13) % 5, // 0 short, 1 curls, 2 bun, 3 cap, 4 none
    eyes: (h >> 16) % 3, // 0 dots, 1 glasses, 2 happy
    mouth: (h >> 18) % 3, // 0 smile, 1 grin, 2 flat
  };
}

// The first letter, for places where an avatar is too much, like the room icons.
export const initial = s => (s || '?').trim().charAt(0).toUpperCase() || '?';
