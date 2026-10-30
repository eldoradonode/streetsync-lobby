// Step 20: small pure helpers for the member list and the directory order. No DOM, no network, easy to test.
// The server decides who is a member and who is online; these only keep what it sent in order.

export const STATUS = {
  on: { label: 'Online', mark: '\u25CF' }, // filled circle
  off: { label: 'Offline', mark: '\u25CB' }, // empty circle
  hid: { label: 'Status hidden', mark: '\u25D0' }, // half circle: the person keeps their online state private
};

const key = h => String(h || '').toLowerCase();

// Applies one {"t":"mp"} frame to the list. d removes the person, otherwise they are added or updated.
// Returns how many members the list gained (1), lost (-1) or neither (0).
export function applyMember(list, f) {
  const i = list.findIndex(x => key(x.h) === key(f.h));
  if (f.d) {
    if (i < 0) return 0;
    list.splice(i, 1);
    return -1;
  }
  if (!f.s) return 0;
  if (i >= 0) {
    list[i] = { ...list[i], s: f.s, a: f.a || list[i].a, o: f.o || list[i].o || '', x: f.x || list[i].x || false };
    return 0;
  }
  list.push({ h: f.h, a: f.a || '', s: f.s, o: f.o || '', x: !!f.x });
  return 1;
}

// Adds a page of the list without repeating anyone already there.
export function mergeMembers(list, page) {
  for (const m of page || []) {
    if (!list.some(x => key(x.h) === key(m.h))) list.push(m);
  }
}

const byName = (a, b) => (b.x ? 1 : 0) - (a.x ? 1 : 0) || key(a.h).localeCompare(key(b.h));

// Online first, then people who keep their state private, then offline. The host first within each.
export function groupMembers(list) {
  const g = { on: [], hid: [], off: [] };
  for (const m of list) (g[m.s] || g.off).push(m);
  for (const k of Object.keys(g)) g[k].sort(byName);
  return g;
}

export const SORTS = [
  { id: 'active', label: 'Most active' },
  { id: 'recent', label: 'Recently active' },
  { id: 'new', label: 'Newest' },
];

// Rooms for the directory in the order the server sent them. Official rooms first (they are not ranked), then the
// community rooms exactly as the server ordered them, then my own private rooms the directory does not list.
export function directoryOrder(rooms, dir) {
  const official = rooms.filter(r => !r.k);
  if (!dir.length) return [...official, ...rooms.filter(r => r.k)];
  const inDir = new Set(dir.map(r => r.s));
  return [...official, ...dir, ...rooms.filter(r => r.k && !inDir.has(r.s))];
}
