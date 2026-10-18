// All the chat logic: the connection, the saved login, rooms, messages and moderation.
// The screens only read `chat` and call the functions below.
import { tierOf, TIERS } from './rank.js';

const WS_URL = new URLSearchParams(location.search).get('server') ||
  ((location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/ws');

export const MESSAGES = {
  handle_taken: 'That handle is taken. Try another one.',
  bad_handle: 'Use 3 to 16 letters, numbers or underscores.',
  bad_login: 'Wrong handle or password.',
  too_many_attempts: 'Too many tries. Wait a few minutes.',
  session_expired: 'Your session ended. Pick a handle or log in again.',
  slow_down: 'Slow down a little.',
  too_long: 'That message is too long.',
  weak_password: 'Use at least 8 characters.',
  has_password: 'This handle already has a password.',
  server_busy: 'The server is busy. Try again in a moment.',
  reply_gone: 'That message is too old to reply to.',
  forbidden: 'You cannot do that.',
  no_such_user: 'There is no user with that handle.',
  needs_password: 'That person needs to protect their handle with a password first.',
  report_gone: 'That message is no longer there to report.',
  already_reported: 'You already reported that message.',
  self_report: 'You cannot report your own message.',
  bad_report: 'Pick spam, abuse or other.',
  not_muted: 'That person is not muted.',
  msg_gone: 'That message is already gone.',
  bad_action: 'That action does not exist.',
  bad_hello: 'Something went wrong. Reload the page.',
  bad_amount: 'Use a number from 1 to 100000.',
  bad_avatar: 'That avatar is not available.',
  blocked: 'You cannot message this person.',
  no_credits: 'You need 1 credit to message someone who is not your friend. Chat in rooms to earn credits.',
  self_dm: 'That is you.',
  empty: 'Write something first.',
  friend_cap: 'A friend list is full (100 friends).',
  already_friends: 'You are already friends, or a request is already open.',
  no_request: 'That request is gone.',
  too_many_requests: 'Too many open requests. Try again later.',
  bad_room_name: 'Room names use 3 to 30 letters, numbers and spaces.',
  bad_room_kind: 'Pick public or private.',
  bad_fee: 'The entry fee is 0 to 50 credits.',
  rooms_full: 'There are too many community rooms right now. Try again later.',
  room_name_taken: 'That room name is taken.',
  needs_invite: 'That room needs an invite code.',
  not_host: 'Only the host can do that.',
  bad_query: 'That search is too long.',
  room_muted: 'The host muted you in this room.',
  bad_bio: 'A bio can be up to 160 characters.',
  dm_closed: 'This person only takes messages from friends.',
  bad_category: 'Pick one of the categories.',
};

// Some error codes need a number or a different wording depending on where they come from.
function errText(m) {
  if (m.c === 'no_credits' && m.m) return 'You need ' + m.m + ' credits for that. Chat in rooms to earn more.';
  if (m.c === 'room_limit') return 'You can host ' + m.m + ' room' + (m.m === 1 ? '' : 's') + ' at a time. Close one first.';
  if (m.c === 'room_banned') return 'The host removed you from that room. Try again in ' + m.m + ' minute' + (m.m === 1 ? '' : 's') + '.';
  if (m.c === 'bad_room_password') return 'Use a password of 4 to 32 characters.';
  return MESSAGES[m.c] || 'Not done.';
}

// While the join panel is open, problems with getting in are shown inside it.
function panelError(m) {
  switch (m.c) {
    case 'needs_invite': return 'This room needs an invite code.';
    case 'bad_room_password': return 'Wrong password.';
    case 'too_many_attempts': return MESSAGES.too_many_attempts;
    case 'fee_changed': chat.joinPanel.f = m.m; return 'The price is now ' + m.m + ' credit' + (m.m === 1 ? '' : 's') + '. Tap again to accept it.';
    case 'no_credits': return 'You need ' + (m.m || chat.joinPanel.f) + ' credits to get in. Chat in rooms to earn more.';
    case 'room_banned': return errText(m);
    case 'no_such_room': return 'That room does not exist any more.';
  }
  return '';
}

export const chat = $state({
  view: 'join', // join, hub, inbox, profile, room, thread or blocked
  status: '',
  token: '',
  me: '',
  role: '', // mod or admin, empty for members
  points: 0,
  tier: 0,
  next: 300, // points for the next tier, 0 at the top
  streak: 0,
  hasPw: false,
  credits: 0,
  wallet: [], // latest credit movements, newest first
  rooms: [],
  curRoom: '',
  roomName: '',
  count: 0,
  items: [], // what is on screen in the open room: messages and notices
  replyTo: null,
  loginMode: false,
  joinErr: '',
  joinBusy: false,
  blockedMsg: '',
  reportCount: 0,
  reports: [],
  reportsOpen: false,
  secureOpen: false,
  secureErr: '',
  dmUnread: 0, // unread direct messages, shown on the Inbox tab
  requests: 0, // friend requests waiting for me
  chats: [],
  friends: [],
  incoming: [], // people who asked to be my friend
  outgoing: [], // people I asked
  blockedUsers: [],
  thread: { h: '', s: 'none', cost: 1, items: [], ready: false },
  threadBack: 'inbox', // where the back arrow of a conversation goes
  // rooms people create
  roomKind: '', // public or private, empty in official rooms
  roomOwner: '',
  roomFee: 0,
  roomPw: false,
  isHost: false,
  invite: '', // invite code, only the host gets it
  joinPanel: null, // {s, n, o, f, w, c, k, err} while someone is about to enter a room that costs credits or has a password
  found: [], // search results
  createOpen: false, // the create room dialog
  legal: '', // '', 'terms' or 'privacy': the open legal page
  createPending: false,
  tipSheet: null, // {i, h, sel, pending, err, key} while the tip picker is open
  avatars: {}, // the look each person picked, by handle in lower case
  myLook: '',
  userBack: 'hub', // where the back arrow of a profile goes
  userView: { h: '', ready: false },
  bio: '', // my bio
  wide: false, // a screen wide enough for the two pane messages layout
  notes: { list: [], unread: 0, loaded: false }, // notifications
  prefs: { nd: true, nk: true, nt: true, nf: true, dp: 'everyone', so: true }, // my settings
  myJoined: 0, // when I joined, in ms
  dir: [], // every public community room, even empty ones
  section: { admin: 'overview', mod: 'overview' }, // the open section of each staff area
  adm: { stats: null, users: [], q: '', staff: [], log: [], rooms: [], busy: false },
});

let ws = null, retry = 0, retryTimer = 0, pingTimer = 0, heard = 0, blockTimer = 0, flashTimer = 0, noteId = 0, unread = 0;
let loadedRoom = '', blocked = false, wantHandle = '', wantPass = '', wantLook = '', pendingView = '';

// Remember the look a person picked, so every screen draws them the same way.
function learn(handle, look) {
  if (handle && look) chat.avatars[handle.toLowerCase()] = look;
}
function learnAll(map) {
  for (const [h, v] of Object.entries(map || {})) learn(h, v);
}
const lastIds = {};

const lastFor = slug => (loadedRoom === slug ? lastIds[slug] || 0 : 0);
export const isMod = () => chat.role === 'mod' || chat.role === 'admin';
const joined = () => chat.view !== 'join';

// ---- small helpers ----

export { addNote as note };

export function flash(msg) {
  chat.status = msg;
  clearTimeout(flashTimer);
  flashTimer = setTimeout(() => { if (chat.status === msg) chat.status = ''; }, 4000);
}

function addNote(cls, text) {
  chat.items.push({ kind: 'note', key: 'n' + ++noteId, cls, text });
  trim();
}

function trim() {
  if (chat.items.length > 200) chat.items.splice(0, chat.items.length - 200);
}

export function send(o) {
  if (!ws || ws.readyState !== 1) { chat.status = 'Offline. Try again when you are connected.'; return false; }
  ws.send(JSON.stringify(o));
  return true;
}

// ---- progress ----

function updateMe(m) {
  chat.points = m.p;
  chat.tier = tierOf(m.r);
  chat.next = m.x || 0;
  chat.streak = m.s || 0;
  chat.hasPw = !!m.g;
  chat.credits = m.c || 0;
  chat.myLook = m.v || '';
  learn(chat.me, m.v);
  chat.role = m.o || '';
  chat.bio = m.bio || '';
  if (chat.hasPw) chat.secureOpen = false;
  // The pages are hidden from people without the role, and the server refuses their requests as well.
  if ((chat.view === 'admin' && chat.role !== 'admin') || (chat.view === 'mod' && !isMod())) chat.view = 'hub';
  if (!isMod()) { chat.reports = []; chat.reportsOpen = false; chat.reportCount = 0; }
}

// ---- messages ----

const mentionRe = () => new RegExp('(^|[^A-Za-z0-9_])@' + chat.me + '(?![A-Za-z0-9_])', 'i');

// True when a message tags me with @handle, or replies to one of my messages.
function mentionsMe(m) {
  if (!chat.me || m.h.toLowerCase() === chat.me.toLowerCase()) return false;
  if (m.qh && m.qh.toLowerCase() === chat.me.toLowerCase()) return true;
  return mentionRe().test(m.b);
}

function addMessage(m, live) {
  learn(m.h, m.av);
  if (m.i <= (lastIds[chat.curRoom] || 0)) return;
  lastIds[chat.curRoom] = m.i;
  const tagged = mentionsMe(m);
  chat.items.push({ kind: 'msg', key: 'm' + m.i, m, tagged });
  trim();
  if (live && tagged && document.visibilityState === 'hidden') {
    unread++;
    document.title = '(' + unread + ') STREETSYNC';
  }
}

export function sendMessage(text) {
  const s = text.trim();
  if (!s) return false;
  if (!ws || ws.readyState !== 1) { chat.status = 'Offline. Your message was not sent.'; probe(); return false; }
  ws.send(JSON.stringify(chat.replyTo ? { t: 'm', b: s, q: chat.replyTo.i } : { t: 'm', b: s }));
  clearReply();
  return true;
}

export function startReply(m) {
  chat.replyTo = { i: m.i, h: m.h, b: m.b };
  const box = document.getElementById('text');
  if (box) box.focus();
}
export function clearReply() { chat.replyTo = null; }

export function deleteOwn(id) { return send({ t: 'del', i: id }); }
export function modDelete(id) { return send({ t: 'mod', a: 'del', i: id }); }
export function modUser(a, handle, minutes) { return send({ t: 'mod', a, h: handle, m: minutes }); }
export function reportMessage(id, c) { return send({ t: 'rep', i: id, c }); }

// ---- rooms ----

export function chooseRoom(r) {
  if ((r.t || 0) > chat.tier && !isMod()) { flash('That room needs ' + TIERS[tierOf(r.t)] + ' or higher.'); return; }
  if (r.k && !r.y && (r.f > 0 || r.w || r.k === 'private')) {
    // Costs credits or needs a password: ask first, never charge on a single tap.
    chat.joinPanel = { s: r.s, n: r.n, o: r.o || '', f: r.f || 0, w: !!r.w, c: '', k: r.k, err: '' };
    return;
  }
  send({ t: 'join', r: r.s, l: lastFor(r.s) });
}

export function confirmJoin(password) {
  const p = chat.joinPanel;
  if (!p) return false;
  p.err = '';
  const o = { t: 'join', r: p.s, l: lastFor(p.s), f: p.f };
  if (p.c) o.c = p.c;
  if (password) o.p = password;
  return send(o);
}
export function cancelJoin() { chat.joinPanel = null; }

export function lookupCode(code) {
  const c = String(code).trim();
  if (c.length !== 8) { flash('Invite codes have 8 letters and numbers.'); return false; }
  return send({ t: 'rinfo', c });
}
export const findRooms = q => send({ t: 'find', n: String(q).trim() });

// ---- tips ----

const newKey = () =>
  typeof crypto !== 'undefined' && crypto.randomUUID ? crypto.randomUUID() : Math.random().toString(36).slice(2) + Date.now().toString(36);

// The key is made once when the sheet opens. A retry or a double tap sends the same key, and the server pays it only once.
export function openTip(m) {
  if (!chat.me || m.h.toLowerCase() === chat.me.toLowerCase()) return false;
  chat.tipSheet = { i: m.i, h: m.h, sel: 0, pending: false, err: '', key: newKey() };
  return true;
}
export const closeTip = () => { chat.tipSheet = null; };
export function pickTip(a) {
  const s = chat.tipSheet;
  if (s && !s.pending) { s.sel = a; s.err = ''; }
}
export function sendTip() {
  const s = chat.tipSheet;
  if (!s || s.pending || !s.sel) return false;
  if (chat.credits < s.sel) { s.err = 'You have ' + chat.credits + ' credits. This tip needs ' + s.sel + '.'; return false; }
  s.pending = true;
  s.err = '';
  if (send({ t: 'tip', i: s.i, amt: s.sel, c: s.key })) return true;
  s.pending = false;
  s.err = 'You are offline. Try again when you are back.';
  return false;
}

const TIP_ERRORS = {
  no_credits: 'You do not have enough credits for that tip.',
  self_tip: 'You cannot tip yourself.',
  blocked: 'You cannot tip this person.',
  msg_gone: 'That message is gone, so the tip was not sent.',
  slow_down: 'Slow down a little, then try again.',
  forbidden: 'That cannot be tipped.',
  no_room: 'Join a room to send tips.',
  bad_amount: 'That tip could not be sent.',
  bad_request: 'That tip could not be sent.',
  server_busy: 'Something went wrong. Trying again is safe: you will not be charged twice.',
};

export const openLegal = which => { chat.legal = which === 'privacy' ? 'privacy' : 'terms'; };
export const closeLegal = () => { chat.legal = ''; };

export function createRoom(f) {
  const n = f.name.trim().replace(/\s+/g, ' ');
  if (n.length < 3 || n.length > 30) { flash(MESSAGES.bad_room_name); return false; }
  const fee = Math.floor(Number(f.fee) || 0);
  if (fee < 0 || fee > 50) { flash(MESSAGES.bad_fee); return false; }
  if (f.pw && (f.pw.length < 4 || f.pw.length > 32)) { flash('Use a password of 4 to 32 characters.'); return false; }
  const o = { t: 'croom', n, k: f.kind, g: f.tags || '', f: fee };
  if (f.category) o.c = f.category;
  if (f.pw) o.p = f.pw;
  chat.createPending = true;
  return send(o);
}

export function saveRoomSettings({ fee, pw, kind }) {
  const o = { t: 'rset' };
  if (fee != null) o.f = fee;
  if (pw != null) o.pw = pw;
  if (kind) o.k = kind;
  return send(o);
}
export const closeRoom = () => send({ t: 'rclose' });
export const hostUser = (a, handle, minutes) => send({ t: 'hmod', a, h: handle, m: minutes });
export const inviteLink = () => location.origin + '/?room=' + chat.invite;

export function leaveRoom() { clearReply(); send({ t: 'leave' }); }

function leaveToHub() {
  chat.curRoom = '';
  chat.tipSheet = null;
  chat.roomKind = ''; chat.roomOwner = ''; chat.roomFee = 0; chat.roomPw = false; chat.isHost = false; chat.invite = '';
  sessionStorage.removeItem('lobby.r');
  clearReply();
  chat.view = 'hub';
  if (pendingView) {
    const v = pendingView;
    pendingView = '';
    if (v !== 'hub') showView(v);
  }
}

// Show one of the main screens and fetch what it needs.
const AREAS = { admin: () => chat.role === 'admin', mod: () => isMod() };

function showView(view) {
  if (AREAS[view] && !AREAS[view]()) view = 'hub'; // not for you; the server refuses the requests anyway
  chat.view = view;
  if (view === 'profile') { send({ t: 'wallet' }); send({ t: 'profile', h: chat.me }); } // fresh balance, history and join date
  if (view === 'wallet') send({ t: 'wallet' });
  if (view === 'notes') loadNotes();
  if (view === 'rooms' || view === 'hub') loadDir();
  if (AREAS[view]) loadSection(view);
  if (view === 'inbox') send({ t: 'inbox' }); // fresh chats and requests
}

// The sidebar and the bottom bar. They work from anywhere, also from inside a room: the room is left first.
export function go(view) {
  if (chat.view === 'join' || chat.view === 'blocked') return;
  if (chat.curRoom) {
    pendingView = view;
    clearReply();
    if (!send({ t: 'leave' })) leaveToHub(); // offline: leave on this side only
    return;
  }
  showView(view);
}

// ---- inbox, friends and direct messages ----

const sameHandle = (a, b) => !!a && !!b && a.toLowerCase() === b.toLowerCase();

// Open the profile of anyone: from a message, a conversation or a list.
export function openUser(handle) {
  const h = String(handle).trim().replace(/^@/, '');
  if (!/^[A-Za-z0-9_]{3,16}$/.test(h)) return false;
  if (chat.view !== 'user') chat.userBack = chat.view === 'join' ? 'hub' : chat.view;
  chat.userView = { h, ready: false };
  chat.view = 'user';
  return send({ t: 'profile', h });
}
export function closeUser() {
  const back = chat.userBack;
  if (back === 'room' && chat.curRoom) chat.view = 'room';
  else if (back === 'thread') chat.view = 'thread';
  else if (back === 'inbox') showView('inbox');
  else if (back === 'profile') showView('profile');
  else chat.view = 'hub';
}
export const setLook = code => send({ t: 'av', c: code });

export function openThread(handle) {
  const h = String(handle).trim().replace(/^@/, '');
  if (!/^[A-Za-z0-9_]{3,16}$/.test(h)) { flash(MESSAGES.bad_handle); return false; }
  if (sameHandle(h, chat.me)) { flash(MESSAGES.self_dm); return false; }
  if (chat.view !== 'thread') chat.threadBack = chat.view === 'room' ? 'room' : 'inbox';
  chat.thread = { h, s: 'none', cost: 1, items: [], ready: false, closed: false };
  const row = chat.chats.find(c => sameHandle(c.h, h));
  if (row) row.u = 0; // opening a conversation reads it; the list beside it should show that at once
  chat.view = 'thread';
  return send({ t: 'thread', h });
}

export function closeThread() {
  chat.view = chat.threadBack === 'room' && chat.curRoom ? 'room' : 'inbox';
  if (chat.view === 'inbox') send({ t: 'inbox' });
}

export function sendDM(text) {
  const b = text.trim();
  if (!b) return false;
  if (chat.thread.closed) { flash(MESSAGES.dm_closed); return false; }
  return send({ t: 'dm', h: chat.thread.h, b });
}

export const friendAct = (a, h) => send({ t: 'fr', a, h });
export const blockAct = (a, h) => send({ t: 'blk', a, h });
export const reportDM = (id, c) => send({ t: 'drep', i: id, c });

function refreshOpen(handle) {
  if (chat.view === 'user' && sameHandle(handle, chat.userView.h)) send({ t: 'profile', h: chat.userView.h });
  else if (chat.view === 'thread' && sameHandle(handle, chat.thread.h)) send({ t: 'thread', h: chat.thread.h });
  else if (chat.view === 'inbox') send({ t: 'inbox' });
}

// Keep the conversation list in step with what just arrived, so the list beside an open conversation is live.
function touchChat(other, m, mine, reading) {
  const prev = chat.chats.find(c => sameHandle(c.h, other));
  const before = prev ? prev.u || 0 : 0;
  const unread = reading ? 0 : mine ? before : before + 1;
  const row = { h: prev ? prev.h : other, b: m.b, ts: m.ts, m: mine, u: unread };
  chat.chats = [row, ...chat.chats.filter(c => !sameHandle(c.h, other))];
}

function onDM(m) {
  const mine = sameHandle(m.f, chat.me);
  const other = mine ? m.w : m.f;
  const reading = chat.view === 'thread' && sameHandle(other, chat.thread.h);
  if (chat.view !== 'inbox') touchChat(other, m, mine, reading);
  if (reading) {
    if (!chat.thread.items.some(x => x.i === m.i)) chat.thread.items.push(m);
    if (chat.thread.items.length > 200) chat.thread.items.splice(0, chat.thread.items.length - 200);
    if (!mine) send({ t: 'read', h: other });
    return;
  }
  if (mine) return;
  chat.dmUnread++;
  flash('New message from @' + m.f);
  if (chat.view === 'inbox') send({ t: 'inbox' });
}

const FRIEND_OK = {
  add: h => 'Friend request sent to @' + h + '.',
  accept: h => 'You are now friends with @' + h + '.',
  decline: () => 'Request declined.',
  cancel: () => 'Request cancelled.',
  remove: h => '@' + h + ' is no longer your friend.',
};
const FRIEND_EVENT = {
  request: h => '@' + h + ' sent you a friend request.',
  friends: h => '@' + h + ' and you are now friends.',
  removed: h => '@' + h + ' removed you as a friend.',
};

// ---- profile ----

export function openSecure() { chat.secureOpen = true; chat.secureErr = ''; }
export function closeSecure() { chat.secureOpen = false; }

export function setPassword(p) {
  if (p.length < 8) { chat.secureErr = MESSAGES.weak_password; return; }
  if (!ws || ws.readyState !== 1) { chat.secureErr = 'Offline. Try again when you are connected.'; return; }
  chat.secureErr = '';
  ws.send(JSON.stringify({ t: 'pw', p }));
}

export function openReports() {
  chat.reportsOpen = !chat.reportsOpen;
  if (chat.reportsOpen) send({ t: 'reports' });
}

export const modReport = o => send(o);

const DONE = { mute: 'Muted.', unmute: 'Unmuted.', kick: 'Removed for now.', promote: 'Moderator added.', demote: 'Moderator removed.', clearbio: 'Bio cleared.', close: 'Room closed.', del: 'Message deleted.' };

// ---- bio ----
export function saveBio(text) {
  const b = String(text).replace(/\s+/g, ' ').trim();
  if ([...b].length > 160) { flash(MESSAGES.bad_bio); return false; }
  return send({ t: 'bio', b });
}

// ---- notifications ----
export const loadNotes = () => send({ t: 'nlist' });

function onNote(m) {
  chat.notes.unread = m.c || 0;
  if (!chat.notes.loaded) return; // the list is fetched when the page opens
  const n = m.n;
  chat.notes.list = [n, ...chat.notes.list.filter(x => x.i !== n.i)].slice(0, 50);
}

export function markNoteRead(n) {
  if (!n.u) return;
  n.u = 0;
  chat.notes.unread = Math.max(0, chat.notes.unread - 1);
  send({ t: 'nread', i: n.i });
}

export function readAllNotes() {
  for (const n of chat.notes.list) n.u = 0;
  chat.notes.unread = 0;
  return send({ t: 'nread', all: true });
}

// Tap a notification: mark it read and go to what it is about.
export function openNote(n) {
  markNoteRead(n);
  if (n.k === 'dm') openThread(n.a);
  else if ((n.k === 'tag' || n.k === 'reply') && n.r) chooseRoom({ s: n.r, n: n.rn });
  else if (n.k === 'friend') (n.b === 'accepted' ? openUser(n.a) : go('inbox'));
  else if (n.k === 'tip') go('wallet');
}

// ---- settings ----
export function setPref(key, value) {
  const b = typeof value === 'boolean' ? (value ? '1' : '0') : value;
  const old = chat.prefs[key];
  chat.prefs[key] = value; // shown at once; the server's answer confirms it
  if (!send({ t: 'set', k: key, b })) { chat.prefs[key] = old; flash('You are offline. Try again when you are back.'); return false; }
  return true;
}

export function changePassword(current, next) {
  if (!current) { flash('Enter your current password.'); return false; }
  if (next.length < 8) { flash(MESSAGES.weak_password); return false; }
  return send({ t: 'pwchange', o: current, p: next });
}

export const setWide = v => { chat.wide = !!v; };

// On a wide screen the conversation list and the open conversation sit side by side.
// A conversation opened from a room stays full width, with its own back button.
export const isSplit = () => chat.wide && (chat.view === 'inbox' || (chat.view === 'thread' && chat.threadBack !== 'room'));

// ---- rooms directory ----
export const loadDir = () => send({ t: 'dir' });
export const ROOM_CATEGORIES = ['General', 'City', 'Campus', 'Sport', 'Music', 'Tech', 'Business', 'Creators', 'Entertainment', 'Lifestyle'];

// Every room a person can see: official rooms, the public community rooms (also empty ones) and their own.
export function allRooms() {
  const seen = new Set(chat.rooms.map(r => r.s));
  return [...chat.rooms, ...chat.dir.filter(r => !seen.has(r.s))];
}

// ---- staff areas (admin console and moderator dashboard) ----
// What the page shows is only a convenience. Every request below is checked again on the server.
const SECTION_LOAD = {
  overview: () => send({ t: 'astats' }),
  users: () => send({ t: 'ausers', h: chat.adm.q }),
  moderators: () => send({ t: 'astaff' }),
  rooms: () => send({ t: 'arooms' }),
  reports: () => send({ t: 'reports' }),
  log: () => send({ t: 'alog' }),
};
export function loadSection(area) {
  chat.adm.busy = true;
  const name = chat.section[area];
  if (name === 'moderators') send({ t: 'astats' });
  if (!(SECTION_LOAD[name] && SECTION_LOAD[name]())) chat.adm.busy = false;
}
export function setSection(area, name) {
  chat.section[area] = name;
  loadSection(area);
}
export const findUsers = q => { chat.adm.q = String(q).trim().replace(/^@/, ''); chat.adm.busy = true; return send({ t: 'ausers', h: chat.adm.q }); };
export const staffAct = (a, handle, minutes) => {
  const o = { t: 'mod', a, h: handle };
  if (minutes) o.m = minutes;
  return send(o);
};
export const closeRoomBySlug = slug => send({ t: 'rclose', r: slug });

// Admin only: hand out credits. The server refuses anyone else.
export function grantCredits(handle, amount) {
  const h = handle.trim().replace(/^@/, '');
  const n = Math.floor(Number(amount));
  if (!/^[A-Za-z0-9_]{3,16}$/.test(h)) { flash(MESSAGES.bad_handle); return false; }
  if (!(n >= 1 && n <= 100000)) { flash(MESSAGES.bad_amount); return false; }
  return send({ t: 'mod', a: 'grant', h, m: n });
}

export function logout() {
  if (ws && ws.readyState === 1) ws.send('{"t":"bye"}');
  clearTimeout(retryTimer);
  clearTimeout(blockTimer);
  stopPing();
  if (ws) { ws.onopen = ws.onmessage = ws.onclose = null; try { ws.close(); } catch (e) {} ws = null; }
  localStorage.removeItem('lobby.k');
  sessionStorage.removeItem('lobby.r');
  Object.assign(chat, {
    view: 'join', token: '', me: '', role: '', points: 0, tier: 0, next: 300, streak: 0, hasPw: false, credits: 0, wallet: [],
    rooms: [], curRoom: '', roomName: '', count: 0, items: [], replyTo: null, joinErr: '', joinBusy: false,
    reportCount: 0, reports: [], reportsOpen: false, status: '', secureOpen: false, secureErr: '',
    dmUnread: 0, requests: 0, chats: [], friends: [], incoming: [], outgoing: [], blockedUsers: [],
    thread: { h: '', s: 'none', cost: 1, items: [], ready: false }, threadBack: 'inbox',
    avatars: {}, myLook: '', userBack: 'hub', userView: { h: '', ready: false },
    notes: { list: [], unread: 0, loaded: false }, prefs: { nd: true, nk: true, nt: true, nf: true, dp: 'everyone', so: true },
    bio: '', myJoined: 0, dir: [], section: { admin: 'overview', mod: 'overview' },
    adm: { stats: null, users: [], q: '', staff: [], log: [], rooms: [], busy: false },
    roomKind: '', roomOwner: '', roomFee: 0, roomPw: false, isHost: false, invite: '', joinPanel: null, found: [], createOpen: false, createPending: false, tipSheet: null,
  });
  loadedRoom = '';
  for (const k of Object.keys(lastIds)) delete lastIds[k];
  blocked = false;
  document.title = 'STREETSYNC';
}

// ---- signing in ----

export function setMode(login) { chat.loginMode = login; chat.joinErr = ''; }

export function signIn(handle, password, look = '') {
  const h = handle.trim().replace(/^@/, '');
  if (!/^[A-Za-z0-9_]{3,16}$/.test(h)) { chat.joinErr = MESSAGES.bad_handle; return; }
  if (chat.loginMode && !password) { chat.joinErr = 'Enter your password.'; return; }
  if (!chat.loginMode && password.length < 8) { chat.joinErr = MESSAGES.weak_password; return; }
  wantHandle = h;
  wantPass = password;
  wantLook = chat.loginMode ? '' : look;
  chat.token = '';
  chat.joinErr = '';
  chat.joinBusy = true;
  connect();
}

function onFrame(m) {
  switch (m.t) {
    case 'ok':
      wantPass = ''; // not needed after sign in; later reconnects use the saved login
      chat.token = m.k;
      chat.me = m.h;
      localStorage.setItem('lobby.k', chat.token);
      retry = 0;
      chat.status = '';
      updateMe(m);
      if (chat.view === 'join') chat.view = chat.curRoom ? 'room' : 'hub';
      if (chat.tipSheet) chat.tipSheet.pending = false; // after a dropped connection the person can retry safely
      const code = new URLSearchParams(location.search).get('room'); // an invite link
      if (code) {
        history.replaceState(null, '', location.pathname);
        if (chat.view === 'hub') send({ t: 'rinfo', c: code });
      }
      if (chat.view === 'thread') send({ t: 'thread', h: chat.thread.h }); // after a dropped connection
      else if (chat.view === 'inbox') send({ t: 'inbox' });
      loadDir(); // every public room, also the empty ones
      startPing();
      break;
    case 'me': updateMe(m); break;
    case 'wallet': chat.credits = m.c || 0; chat.wallet = m.l || []; break;
    case 'ib': chat.dmUnread = m.u || 0; chat.requests = m.q || 0; break;
    case 'inbox':
      learnAll(m.av);
      chat.chats = m.c || []; chat.friends = m.f || []; chat.incoming = m.q || [];
      chat.outgoing = m.p || []; chat.blockedUsers = m.x || [];
      break;
    case 'thread':
      learnAll(m.av);
      if (sameHandle(m.h, chat.thread.h)) {
        chat.thread.h = m.h; chat.thread.s = m.s; chat.thread.cost = m.c || 0;
        chat.thread.items = m.m || []; chat.thread.ready = true; chat.thread.closed = !!m.cl;
      }
      break;
    case 'dm': learn(m.f, m.fa); onDM(m); break;
    case 'uprofile':
      learn(m.h, m.v);
      if (sameHandle(m.h, chat.me)) chat.myJoined = m.j || 0;
      if (sameHandle(m.h, chat.userView.h)) chat.userView = Object.assign({}, m, { ready: true });
      break;
    case 'found': chat.found = m.r || []; break;
    case 'dir': chat.dir = m.r || []; break;
    case 'nc': chat.notes.unread = m.c || 0; break;
    case 'nn': onNote(m); break;
    case 'notes': chat.notes.list = m.l || []; chat.notes.unread = m.c || 0; chat.notes.loaded = true; break;
    case 'pf': chat.prefs = { nd: !!m.nd, nk: !!m.nk, nt: !!m.nt, nf: !!m.nf, dp: m.dp === 'friends' ? 'friends' : 'everyone', so: !!m.so }; break;
    case 'pwchanged': flash('Password changed. Your other devices were logged out.'); break;
    case 'biook': flash('Bio saved.'); break;
    case 'astats': chat.adm.stats = m; chat.adm.busy = false; break;
    case 'ausers': chat.adm.users = m.u || []; chat.adm.q = m.q || ''; chat.adm.busy = false; break;
    case 'astaff': chat.adm.staff = m.s || []; chat.adm.busy = false; break;
    case 'alog': chat.adm.log = m.l || []; chat.adm.busy = false; break;
    case 'arooms': chat.adm.rooms = m.r || []; chat.adm.busy = false; break;
    case 'tipok':
      chat.tipSheet = null;
      flash(m.dup ? 'That tip was already sent.' : 'You sent @' + m.h + ' a ' + m.n + ' (' + m.a + ' credit' + (m.a === 1 ? '' : 's') + ').');
      break;
    case 'tipgot': flash('@' + m.f + ' sent you a ' + m.n + ' (' + m.a + ' credit' + (m.a === 1 ? '' : 's') + ').'); break;
    case 'rinfo':
      if (m.y) send({ t: 'join', r: m.s, l: lastFor(m.s), c: m.c, f: m.f || 0 }); // host or member: straight in
      else chat.joinPanel = { s: m.s, n: m.n, o: m.o || '', f: m.f || 0, w: !!m.w, c: m.c, k: m.k, err: '' };
      break;
    case 'croomok': chat.createPending = false; flash('Room created.'); send({ t: 'join', r: m.s }); break;
    case 'rsetok':
      chat.roomFee = m.f || 0; chat.roomPw = !!m.w; chat.roomKind = m.k || chat.roomKind;
      flash('Room settings saved.');
      break;
    case 'rmuted': addNote('sys', 'The host muted you in this room for ' + m.m + ' minute' + (m.m === 1 ? '' : 's') + '.'); break;
    case 'rkicked': leaveToHub(); flash('The host removed you from ' + m.n + ' for ' + m.m + ' minute' + (m.m === 1 ? '' : 's') + '.'); break;
    case 'rclosed':
      if (m.s === chat.curRoom) leaveToHub();
      if (chat.joinPanel && chat.joinPanel.s === m.s) chat.joinPanel = null;
      flash('The room ' + m.n + ' was closed.');
      break;
    case 'frok': flash((FRIEND_OK[m.a] || (() => 'Done.'))(m.h)); refreshOpen(m.h); break;
    case 'fr': flash((FRIEND_EVENT[m.a] || (() => 'Friends updated.'))(m.h)); refreshOpen(m.h); break;
    case 'blkok': flash((m.a === 'add' ? 'Blocked @' : 'Unblocked @') + m.h + '.'); refreshOpen(m.h); break;
    case 'rooms': chat.rooms = m.r || []; break;
    case 'hub':
      for (const u of m.r || []) {
        const r = chat.rooms.find(x => x.s === u.s);
        if (r) { r.u = u.u; r.h = u.h; }
      }
      break;
    case 'joined':
      chat.curRoom = m.r;
      chat.roomName = m.n;
      sessionStorage.setItem('lobby.r', chat.curRoom);
      if (loadedRoom !== chat.curRoom) {
        chat.items = [];
        loadedRoom = chat.curRoom;
        lastIds[chat.curRoom] = 0;
        clearReply();
      }
      chat.count = 0;
      chat.roomKind = m.k || ''; chat.roomOwner = m.o || ''; chat.roomFee = m.f || 0; chat.roomPw = !!m.w;
      chat.isHost = !!m.y; chat.invite = m.i || '';
      chat.joinPanel = null;
      if (chat.view !== 'thread') chat.view = 'room'; // a conversation opened from a room stays open after a reconnect
      break;
    case 'left': leaveToHub(); break;
    case 'hist': for (const x of m.m || []) addMessage(x, false); break;
    case 'm': addMessage(m, true); break;
    case 'n': chat.count = m.c; break;
    case 'up': addNote('up t' + tierOf(m.r), '@' + m.h + ' reached ' + TIERS[tierOf(m.r)]); break;
    case 'muted': addNote('sys', 'You were muted for ' + m.m + ' minute' + (m.m === 1 ? '' : 's') + '.'); break;
    case 'unmuted': addNote('up t1', 'You can chat again.'); break;
    case 'kicked': showBlocked(m.m || 1); break;
    case 'mod': addNote('mod', '@' + m.h + (m.a === 'kick' ? ' was removed for ' : ' was muted for ') + m.m + ' minute' + (m.m === 1 ? '' : 's') + '.'); break;
    case 'del': {
      const at = chat.items.findIndex(x => x.kind === 'msg' && x.m.i === m.i);
      if (at >= 0) chat.items.splice(at, 1);
      if (chat.replyTo && chat.replyTo.i === m.i) clearReply();
      break;
    }
    case 'repok': addNote('up t1', 'Thanks. A moderator will take a look.'); break;
    case 'modok':
      if (m.a === 'grant') flash('Sent ' + m.m + ' credits to @' + m.h + '.');
      else if (chat.view === 'admin' || chat.view === 'mod') flash(DONE[m.a] || 'Done.');
      else if (m.a !== 'dismiss') addNote('up t1', 'Done.');
      if (chat.view === 'admin' || chat.view === 'mod') loadSection(chat.view); // show the new state
      break;
    case 'rn':
      chat.reportCount = m.c;
      if (chat.reportsOpen || ((chat.view === 'admin' || chat.view === 'mod') && chat.section[chat.view] === 'reports')) send({ t: 'reports' });
      break;
    case 'reps': chat.reports = m.r || []; break;
    case 'pwok':
      chat.hasPw = true;
      chat.secureOpen = false;
      addNote('up t1', 'Your handle is now protected. Log in with it on any device.');
      break;
    case 'err': onError(m); break;
  }
}

function onError(m) {
  if (m.c === 'kicked') { showBlocked(m.m || 1); return; }
  if (chat.tipSheet && TIP_ERRORS[m.c]) {
    chat.tipSheet.pending = false;
    chat.tipSheet.err = TIP_ERRORS[m.c];
    return;
  }
  if (chat.joinPanel) {
    const text = panelError(m);
    if (text) { chat.joinPanel.err = text; return; }
  }
  if (m.c === 'tier_locked' && chat.createPending) {
    chat.createPending = false;
    flash('Creating a room needs ' + TIERS[tierOf(m.m)] + ' or higher.');
    return;
  }
  chat.createPending = false;
  if (m.c === 'muted' && chat.view === 'thread') { flash('You are muted for ' + m.m + ' more minute' + (m.m === 1 ? '' : 's') + '.'); return; }
  if (m.c === 'muted') { addNote('sys', 'You are muted for ' + m.m + ' more minute' + (m.m === 1 ? '' : 's') + '.'); return; }
  if (m.c === 'tier_locked' || m.c === 'no_such_room' || m.c === 'no_room') {
    flash(m.c === 'tier_locked' ? 'That room needs ' + TIERS[tierOf(m.m)] + ' or higher.' :
      m.c === 'no_room' ? 'Pick a room first.' : 'That room does not exist.');
    if (chat.view === 'room') leaveToHub(); // a saved room that no longer works
    return;
  }
  if (m.c === 'session_expired') {
    chat.token = '';
    localStorage.removeItem('lobby.k');
    chat.view = 'join';
    chat.joinBusy = false;
    chat.joinErr = MESSAGES.session_expired;
    return;
  }
  if (joined()) {
    if (m.c === 'reply_gone') clearReply();
    const text = errText(m);
    if (chat.secureOpen && (m.c === 'weak_password' || m.c === 'has_password' || m.c === 'server_busy')) chat.secureErr = text;
    else if (chat.view !== 'room') flash(text); // hub, inbox, profile and conversations have no chat log to write in
    else addNote('sys', text);
  } else {
    chat.view = 'join';
    chat.joinBusy = false;
    chat.joinErr = MESSAGES[m.c] || 'Could not join.';
  }
}

// ---- the connection ----

function connect() {
  clearTimeout(retryTimer);
  if (ws) { ws.onopen = ws.onmessage = ws.onclose = null; try { ws.close(); } catch (e) {} }
  chat.status = retry ? 'Reconnecting...' : 'Connecting...';
  const sock = new WebSocket(WS_URL);
  ws = sock;
  sock.onopen = () => {
    heard = Date.now();
    const hello = chat.token
      ? Object.assign({ t: 'hi', k: chat.token }, chat.curRoom ? { r: chat.curRoom, l: lastFor(chat.curRoom) } : {})
      : chat.loginMode ? { t: 'login', h: wantHandle, p: wantPass }
      : Object.assign({ t: 'hi', h: wantHandle, p: wantPass }, wantLook ? { v: wantLook } : {});
    sock.send(JSON.stringify(hello));
  };
  sock.onmessage = ev => {
    if (sock !== ws) return;
    heard = Date.now();
    let m;
    try { m = JSON.parse(ev.data); } catch (e) { return; }
    onFrame(m);
  };
  sock.onclose = () => {
    if (sock !== ws) return;
    stopPing();
    if (blocked) return;
    if (chat.token) scheduleRetry();
    else if (!joined() && !chat.joinErr) { chat.joinBusy = false; chat.joinErr = 'Could not connect. Try again.'; }
  };
}

function scheduleRetry() {
  clearTimeout(retryTimer);
  const wait = Math.min(8000, 500 * 2 ** Math.min(retry, 4));
  retry++;
  chat.status = 'Offline. Retrying in ' + Math.max(1, Math.round(wait / 1000)) + 's';
  retryTimer = setTimeout(connect, wait);
}

// A phone can lose the network without telling the page, and background tabs freeze timers.
// So the connection is also checked the moment the page comes back, and dropped if the server is silent.
function probe() {
  if (!chat.token || blocked) return;
  if (!ws || ws.readyState > 1) { retry = 0; connect(); return; }
  if (ws.readyState !== 1) return;
  const sock = ws, sentAt = Date.now();
  sock.send('{"t":"p"}');
  setTimeout(() => {
    if (sock === ws && heard < sentAt && sock.readyState <= 1) { retry = 0; sock.close(); connect(); }
  }, 4000);
}

function startPing() {
  stopPing();
  pingTimer = setInterval(() => {
    if (!ws || ws.readyState !== 1) return;
    if (Date.now() - heard > 45000) { retry = 0; ws.close(); connect(); return; }
    ws.send('{"t":"p"}');
  }, 15000);
}
function stopPing() { clearInterval(pingTimer); }

function showBlocked(minutes) {
  blocked = true;
  clearTimeout(retryTimer);
  stopPing();
  chat.view = 'blocked';
  chat.status = '';
  chat.blockedMsg = 'You can come back in about ' + minutes + ' minute' + (minutes === 1 ? '' : 's') + '. The page will reconnect by itself.';
  clearTimeout(blockTimer);
  blockTimer = setTimeout(() => {
    blocked = false;
    retry = 0;
    chat.view = chat.curRoom ? 'room' : 'hub';
    connect();
  }, minutes * 60000 + 1500);
}

// ---- start up ----

export function init() {
  chat.token = localStorage.getItem('lobby.k') || '';
  chat.curRoom = sessionStorage.getItem('lobby.r') || '';
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') { unread = 0; document.title = 'STREETSYNC'; probe(); }
  });
  window.addEventListener('pageshow', probe);
  window.addEventListener('focus', probe);
  window.addEventListener('online', probe);
  if (chat.token) {
    chat.view = chat.curRoom ? 'room' : 'hub';
    connect();
  }
}
