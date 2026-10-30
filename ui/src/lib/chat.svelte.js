// All the chat logic: the connection, the saved login, rooms, messages and moderation.
// The screens only read `chat` and call the functions below.
import { tierOf, TIERS } from './rank.js';
import * as E from './e2ee.js';
import { applyReaction, validatePoll, REACTIONS } from './engage.js';
import { applyMember, mergeMembers, directoryOrder } from './rooms20.js';

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
  same_password: 'Choose a password that is different from the current one.',
  has_password: 'This handle already has a password.',
  server_busy: 'The server is busy. Try again in a moment.',
  reply_gone: 'That message is too old to reply to.',
  forbidden: 'You cannot do that.',
  no_such_user: 'There is no user with that handle.',
  needs_password: 'That person needs to protect their handle with a password first.',
  report_gone: 'That message is no longer there to report.',
  already_reported: 'You already reported that message.',
  self_report: 'You cannot report your own message.',
  bad_report: 'Pick a reason first.',
  not_muted: 'That person is not muted.',
  msg_gone: 'That message is already gone.',
  bad_emoji: 'That reaction is not available.',
  bad_poll: 'Check the question and options and try again.',
  poll_open: 'Close the running poll before starting another.',
  fav_full: 'You can keep 50 favourite rooms. Remove one first.',
  poll_failed: 'The poll could not be saved. This is a server fault, not something you did. Tell the room admin.',
  poll_closed: 'That poll is closed.',
  no_poll: 'That poll is no longer open here.',
  no_room: 'Open a room first.',
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
  e2ee_required: 'Messages must be encrypted. Reload the page and try again.',
  bad_envelope: 'That message could not be sent. Reload the page and try again.',
  no_own_key: 'Your encryption key is not set up yet. Open Settings, Encrypted messages.',
  peer_no_key: 'That person has not set up encrypted messages yet. They need to open STREETSYNC once.',
  key_changed: 'Their encryption key changed. Check the safety code before you send anything.',
  replay: 'That message was already sent.',
  admin_no_wallet: 'Admin accounts do not use credits.',
  bad_key: 'That key was refused.',
  needs_disclosure: 'Pick the messages to report first.',
  bad_disclosure: 'That message could not be verified, so it was not reported.',
  bad_category: 'Pick one of the categories.',
  staff_shield: 'Admins and moderators cannot be tipped, messaged or sent friend requests this way.',
  staff_report: 'Staff are reported through "Report staff misconduct", which only admins can read.',
  not_staff: 'That person is not staff. Use the normal report instead.',
  bad_details: 'Add a few words about what happened (up to 300 characters).',
  bad_description: 'A description can be up to 200 characters.',
  staff_no_fee: 'Staff accounts cannot charge an entry fee.',
  owner_cannot_leave: 'You host this room. Close it instead of leaving.',
  not_member: 'You have not joined that room.',
  not_banned: 'That account is not banned.',
  already_banned: 'That account is already banned.',
  bad_password: 'That password is not right.',
  bad_confirm: 'Type the confirmation exactly as shown.',
  delete_expired: 'That confirmation ran out. Start again.',
  admin_cannot_delete: 'An admin account cannot be deleted here. Ask another admin to remove the admin role first.',
  banned: 'This account was permanently banned.',
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
  pin: null, // the pinned message of the open room, or null (step 19)
  poll: null, // the latest poll of the open room, or null
  rx: {}, // reactions by message id: { c: { emoji: count }, y: my emoji }
  pollForm: false,
  favSet: [], // slugs of my favourite rooms, newest first (step 20)
  favRooms: [], // those rooms, as the server describes them for me
  recentRooms: [], // rooms I entered lately, newest first
  dirSort: 'active', // directory order: active, recent or new
  members: { open: false, busy: false, err: '', list: [], on: 0, tot: 0, more: false, next: 0 },
  pollBusy: false, // a poll request is on its way: the button waits, a second tap is ignored
  pollErr: '', // why the last poll request failed, shown in the form
  pickFor: 0, // id of the message whose reaction picker is open
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
  thread: { h: '', s: 'none', cost: 1, items: [], ready: false, sec: { state: 'loading' } },
  threadBack: 'inbox', // where the back arrow of a conversation goes
  // end-to-end encryption. state: init, unsupported, registering, ready, mismatch (the server has another key than this
  // device), other_device (the server has a key and this device has none), error
  e2e: { state: 'init', fp: '', persistent: null, err: '', busy: false },
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
  photos: {}, // profile photo version by handle in lower case, for people who have one
  roomNotices: (() => { try { return localStorage.getItem('ss.roomNotices') !== '0'; } catch (e) { return true; } })(), // show "@x joined the room" lines, kept on this device
  inboxReady: false, // the first answer for the conversation list has arrived
  photoOn: null, // null unknown, true when the server can store photos
  myLook: '',
  userBack: 'hub', // where the back arrow of a profile goes
  userView: { h: '', ready: false },
  bio: '', // my bio
  wide: false, // a screen wide enough for the two pane messages layout
  notes: { list: [], unread: 0, loaded: false }, // notifications
  prefs: { nd: true, nk: true, nt: true, nf: true, dp: 'everyone', so: true }, // my settings
  myJoined: 0, // when I joined, in ms
  presence: {}, // lower case handle to true or false, for the person whose conversation is open
  roomJoined: false, // the server confirmed I am joined to the room I am in
  joining: '', // slug of a join that was asked for and not answered yet
  dir: [], // every public community room, even empty ones
  section: { admin: 'overview', mod: 'overview' }, // the open section of each staff area
  adm: { stats: null, users: [], q: '', staff: [], log: [], rooms: [], busy: false },
  roomsCat: '', // a category picked on the home page, opened on the Rooms page
  ufound: { q: '', r: [], busy: false }, // people search
  complaints: [], // admins only: open complaints about staff
  complaintCount: 0,
  manage: null, // the room management page: {s, ...} filled by the server
  del: { stage: 0, k: '', err: '', busy: false }, // account deletion: 0 closed, 1 asking, 2 final confirmation
  reportSheet: null, // {kind: 'user'|'room'|'message'|'staff', h, s, i, c, d, err, pending}
});

let ws = null, retry = 0, retryTimer = 0, pingTimer = 0, heard = 0, blockTimer = 0, flashTimer = 0, noteId = 0, unread = 0;
let loadedRoom = '', blocked = false, wantHandle = '', wantPass = '', wantLook = '', pendingView = '';

// Remember the look a person picked, so every screen draws them the same way.
// A look arrives as a code like "f3", or "f3~7" when the person has a profile photo (7 is its version).
// Lookups from the database are the truth. Looks copied into old messages can be older, so they only add a photo, never remove one.
function learn(handle, look, strong = false) {
  if (!handle) return;
  const k = handle.toLowerCase();
  const [code, ver] = String(look || '').split('~');
  if (code) chat.avatars[k] = code;
  if (ver) { if (strong || !chat.photos[k] || +ver > +chat.photos[k]) chat.photos[k] = ver; }
  else if (strong) delete chat.photos[k];
}
function learnAll(map) {
  for (const [h, v] of Object.entries(map || {})) learn(h, v, true);
}

const PHOTO_ERRORS = {
  photo_bad: 'That file is not a usable JPEG, PNG or WebP picture.',
  photo_too_big: 'That picture is too large. Try a smaller one.',
  photo_dimensions: 'That picture is too big in pixels. Try a smaller one.',
  photo_small: 'That picture is too small. Use one at least 48 pixels wide and tall.',
  photo_webp_here: 'Choose a JPEG, PNG or WebP picture.',
  slow_down: 'Wait a few seconds and try again.',
  photo_storage: 'Could not save the picture. Try again.',
  photos_off: 'Profile photos are not switched on yet.',
  not_signed_in: 'Sign in again to change your photo.',
};
export async function loadPhotoStatus() {
  try { const r = await fetch('/api/photo/status', { cache: 'no-store' }); chat.photoOn = !!(await r.json()).on; } catch (e) { chat.photoOn = false; }
}
async function photoCall(method, body) {
  try {
    const r = await fetch('/api/photo', { method, body, headers: { Authorization: 'Bearer ' + chat.token } });
    const j = await r.json().catch(() => ({}));
    if (r.ok) return { ok: true, look: j.look || '' };
    return { ok: false, err: PHOTO_ERRORS[j.e] || 'Could not change your photo. Try again.' };
  } catch (e) { return { ok: false, err: 'Offline. Try again when you are connected.' }; }
}
export async function uploadPhoto(blob) {
  const res = await photoCall('POST', blob);
  if (res.ok) { learn(chat.me, res.look, true); chat.myLook = String(res.look).split('~')[0]; }
  return res;
}
export async function removePhoto() {
  const res = await photoCall('DELETE');
  if (res.ok) learn(chat.me, chat.myLook, true);
  return res;
}
const lastIds = {};

const lastFor = slug => (loadedRoom === slug ? lastIds[slug] || 0 : 0);
export const isAdmin = () => chat.role === 'admin'; // admins have no wallet: the page hides every credit control for them
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
  chat.myLook = String(m.v || '').split('~')[0];
  if (chat.me) learn(chat.me, m.v || chat.myLook, true);
  learn(chat.me, m.v, true);
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

function resetEngage() {
  chat.pin = null; chat.poll = null; chat.rx = {}; chat.pollForm = false; chat.pickFor = 0;
  resetMembers();
  endPollRequest('');
}

// Step 19 actions. The server decides who may do what; these only send the request.
export const canManageRoom = () => !!chat.curRoom && (chat.isHost || isMod());
export const pinMessage = id => send({ t: 'pin', i: id });
export const unpinMessage = () => send({ t: 'unpin' });
export function reactTo(id, emoji) {
  if (!REACTIONS.includes(emoji) && emoji !== '') return false;
  chat.pickFor = 0;
  return send({ t: 'rx', i: id, em: emoji });
}
const POLL_ERRORS = ['forbidden', 'bad_poll', 'poll_open', 'poll_failed', 'server_busy', 'no_room', 'slow_down', 'muted', 'room_muted'];
let pollWatch = 0;
function endPollRequest(err) {
  clearTimeout(pollWatch);
  chat.pollBusy = false;
  chat.pollErr = err;
}
// One request at a time: a second tap while one is on its way does nothing. The button comes back when the server
// answers, with the poll (success) or the reason (failure), or after 10 seconds without any answer.
export function createPoll(question, options) {
  if (chat.pollBusy) return '';
  const v = validatePoll(question, options);
  if (!v.ok) { chat.pollErr = v.err; return v.err; }
  chat.pollErr = '';
  chat.pollBusy = true;
  if (!send({ t: 'pcreate', b: v.q, os: v.opts })) { endPollRequest('Offline. Try again when you are connected.'); return chat.pollErr; }
  clearTimeout(pollWatch);
  pollWatch = setTimeout(() => { if (chat.pollBusy) endPollRequest('No answer from the server. Check your connection and try again.'); }, 10000);
  return '';
}
export const votePoll = opt => chat.poll && send({ t: 'pvote', pid: chat.poll.id, opt });
export const closePoll = () => chat.poll && send({ t: 'pclose', pid: chat.poll.id });

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
  if (chat.joining === r.s) return; // already asked, waiting for the server
  if ((r.t || 0) > chat.tier && !isMod()) { flash('That room needs ' + TIERS[tierOf(r.t)] + ' or higher.'); return; }
  if (r.k && !r.y && (r.f > 0 || r.w || r.k === 'private')) {
    // Costs credits or needs a password: ask first, never charge on a single tap.
    chat.joinPanel = { s: r.s, n: r.n, o: r.o || '', f: r.f || 0, w: !!r.w, c: '', k: r.k, err: '' };
    return;
  }
  markJoining(r.s);
  send({ t: 'join', r: r.s, l: lastFor(r.s) });
}

// A join was asked for. Repeated taps are ignored until the server answers, and a lost answer clears itself.
let joiningTimer;
function markJoining(slug) {
  chat.joining = slug;
  clearTimeout(joiningTimer);
  joiningTimer = setTimeout(() => { if (chat.joining === slug) { chat.joining = ''; flash('Could not join. Try again.'); } }, 10000);
}
function clearJoining() { chat.joining = ''; clearTimeout(joiningTimer); }

export function confirmJoin(password) {
  const p = chat.joinPanel;
  if (!p) return false;
  p.err = '';
  const o = { t: 'join', r: p.s, l: lastFor(p.s), f: p.f };
  if (p.c) o.c = p.c;
  if (password) o.p = password;
  markJoining(p.s);
  return send(o);
}
export function cancelJoin() { chat.joinPanel = null; }

export function lookupCode(code) {
  const c = String(code).trim();
  if (c.length !== 8) { flash('Invite codes have 8 letters and numbers.'); return false; }
  return send({ t: 'rinfo', c });
}
// Search. The same words are cleaned the way the server cleans them (no @, lower case), because phones capitalise the first
// letter and the server answers with the cleaned words: an answer is only used when it matches what is on screen now.
const normUser = q => String(q).trim().replace(/^@/, '').toLowerCase();
let findPending = 0; // find requests sent and not answered yet. Only the last answer is shown.
let lastUserSearch = 0, retryTimerS, liveTimer;
export const searchUsers = q => {
  const n = normUser(q);
  if (n.length < 2) { chat.ufound = { q: '', r: [], busy: false }; return false; }
  chat.ufound = { q: n, r: [], busy: true };
  lastUserSearch = Date.now();
  return send({ t: 'usearch', n });
};
let lastFindQ = '';
export const findRooms = q => { lastFindQ = String(q).trim(); const ok = send({ t: 'find', n: lastFindQ, sort: chat.dirSort }); if (ok) findPending++; return ok; };
// Search as the person types: wait for a pause of 250 ms, then ask. A new keystroke cancels the wait.
export function liveSearch(raw, now = false) {
  clearTimeout(liveTimer);
  const q = String(raw).trim();
  const run = () => {
    if (q.length >= 2) findRooms(q); else chat.found = [];
    const gap = now ? 0 : 300 - (Date.now() - lastUserSearch); // the server allows one people search per 250 ms; Enter asks at once and a refusal is retried
    clearTimeout(retryTimerS);
    if (gap > 0) retryTimerS = setTimeout(() => searchUsers(q), gap); else searchUsers(q);
  };
  if (now) run(); else liveTimer = setTimeout(run, 250);
}

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
  const d = String(f.description || '').trim().replace(/\s+/g, ' ');
  if (d.length > 200) { flash(MESSAGES.bad_description); return false; }
  const o = { t: 'croom', n, k: f.kind, g: f.tags || '', f: fee, d };
  if (f.category) o.c = f.category;
  if (f.pw) o.p = f.pw;
  chat.createPending = true;
  return send(o);
}

export function saveRoomSettings({ fee, pw, kind, description, room }) {
  const o = { t: 'rset' };
  if (room) o.r = room;
  if (description != null) o.d = String(description).trim().replace(/\s+/g, ' ').slice(0, 200);
  if (fee != null) o.f = fee;
  if (pw != null) o.pw = pw;
  if (kind) o.k = kind;
  return send(o);
}
export const closeRoom = () => send({ t: 'rclose' });
export const hostUser = (a, handle, minutes, room) => send(room ? { t: 'hmod', a, h: handle, m: minutes, r: room } : { t: 'hmod', a, h: handle, m: minutes });
export const leaveMembership = slug => send({ t: 'rleave', r: slug });
export function openManage(slug) { chat.manage = { s: slug, ready: false }; chat.view = 'manage'; return send({ t: 'rmgr', r: slug }); }
export const refreshManage = () => chat.manage && send({ t: 'rmgr', r: chat.manage.s });
export function closeManage() { chat.manage = null; showView('rooms'); }
export const banUser = (handle, reason) => send({ t: 'mod', a: 'ban', h: handle, b: reason || '' });
export const unbanUser = handle => send({ t: 'mod', a: 'unban', h: handle });
export const loadComplaints = () => send({ t: 'acomp' });
export const closeComplaint = (id, a, note) => send({ t: 'acompdone', id, a, b: note || '' });

// reports and complaints
export function openReport(o) { chat.reportSheet = Object.assign({ c: '', d: '', err: '', pending: false }, o); return true; }
export const closeReport = () => { chat.reportSheet = null; };
export function submitReport() {
  const r = chat.reportSheet;
  if (!r || r.pending) return false;
  if (!r.c) { r.err = MESSAGES.bad_report; return false; }
  if (r.kind === 'dm') return submitDMReport();
  if (r.d.length > (r.kind === 'staff' ? 500 : 300)) { r.err = MESSAGES.bad_details; return false; }
  const o = r.kind === 'staff' ? { t: 'scomp', h: r.h, i: r.i || 0, c: r.c, b: r.d }
    : r.kind === 'user' ? { t: 'urep', h: r.h, c: r.c, b: r.d }
    : r.kind === 'room' ? { t: 'rrep', r: r.s, c: r.c, b: r.d }
    : { t: 'rep', i: r.i, c: r.c };
  r.pending = true; r.err = '';
  if (send(o)) return true;
  r.pending = false; r.err = 'You are offline.';
  return false;
}

// account deletion, two steps
export const deleteStart = () => { chat.del = { stage: 1, k: '', err: '', busy: false }; };
export const deleteCancel = () => { chat.del = { stage: 0, k: '', err: '', busy: false }; };
export function deleteVerify(password, handle) {
  chat.del.busy = true; chat.del.err = '';
  return send(chat.hasPw ? { t: 'delreq', p: password } : { t: 'delreq', h: handle });
}
export function deleteFinal(word) {
  chat.del.busy = true; chat.del.err = '';
  return send({ t: 'delgo', k: chat.del.k, c: word });
}
export const inviteLink = () => location.origin + '/?room=' + chat.invite;

export function leaveRoom() { clearReply(); send({ t: 'leave' }); } // the back arrow: still a member, notice only after a grace period
const setJoined = (slug, on) => { for (const list of [chat.rooms, chat.dir, chat.found]) for (const r of list) if (r.s === slug) r.j = on; };
// The Leave room button. A community room you joined ends your membership; an official room just takes you out, at once.
export function leaveCurrentRoom() {
  const slug = chat.curRoom;
  if (!slug) return;
  const r = allRooms().find(x => x.s === slug);
  clearReply();
  if (r && r.k && r.j && r.o !== chat.me) { leaveMembership(slug); return; }
  send({ t: 'leave', a: 'x' });
}

function leaveToHub() {
  resetEngage();
  chat.curRoom = '';
  chat.tipSheet = null;
  chat.roomKind = ''; chat.roomOwner = ''; chat.roomFee = 0; chat.roomPw = false; chat.isHost = false; chat.invite = ''; chat.roomJoined = false;
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
  if (view === 'rooms' || view === 'hub') { loadDir(); loadMyRooms(); }
  if (AREAS[view]) loadSection(view);
  if (view === 'inbox') send({ t: 'inbox' }); // fresh chats and requests
}

// The sidebar and the bottom bar. They work from anywhere, also from inside a room: the room is left first.
export function go(view) {
  if (chat.view === 'join' || chat.view === 'blocked') return;
  if (view === 'wallet' && isAdmin()) view = 'hub';
  if (view !== 'manage') chat.manage = null;
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

// ---- direct message receipts ----
// 1 sent, 2 delivered, 3 read. Only ids and numbers travel; a status never goes down.
const visible = () => document.visibilityState === 'visible';
const askThread = h => send(visible() ? { t: 'thread', h } : { t: 'thread', h, nr: true }); // a hidden page never marks anything read
const seenSt = new Map(); // receipts that arrived before their message was on screen
const seenUpto = {};
const stFor = (m, other) => Math.max(m.st || 1, seenSt.get(m.i) || 0, (seenUpto[String(other).toLowerCase()] || 0) >= m.i ? 3 : 0);
function onReceipt(m) {
  const key = String(m.h).toLowerCase();
  if (m.s === 3) seenUpto[key] = Math.max(seenUpto[key] || 0, m.u || 0);
  else for (const id of m.ids || []) seenSt.set(id, Math.max(seenSt.get(id) || 0, m.s));
  if (seenSt.size > 500) seenSt.clear();
  if (!sameHandle(m.h, chat.thread.h)) return;
  for (const x of chat.thread.items) {
    if (!sameHandle(x.f, chat.me)) continue;
    const to = m.s === 3 ? x.i <= (m.u || 0) : (m.ids || []).includes(x.i);
    if (to && (x.st || 1) < m.s) x.st = m.s;
  }
}
function ackFrom(other, msgs) {
  const ids = (msgs || []).filter(x => !sameHandle(x.f, chat.me) && (x.st || 1) === 1).map(x => x.i).slice(-100);
  if (ids.length) send({ t: 'dack', h: other, ids });
}

export function openThread(handle) {
  const h = String(handle).trim().replace(/^@/, '');
  if (!/^[A-Za-z0-9_]{3,16}$/.test(h)) { flash(MESSAGES.bad_handle); return false; }
  if (sameHandle(h, chat.me)) { flash(MESSAGES.self_dm); return false; }
  if (chat.view !== 'thread') chat.threadBack = chat.view === 'room' ? 'room' : 'inbox';
  chat.thread = { h, s: 'none', cost: 1, items: [], ready: false, closed: false, sec: { state: 'loading' } };
  const row = chat.chats.find(c => sameHandle(c.h, h));
  if (row) row.u = 0; // opening a conversation reads it; the list beside it should show that at once
  chat.view = 'thread';
  return askThread(h);
}

export function closeThread() {
  send({ t: 'unwatch' });
  chat.view = chat.threadBack === 'room' && chat.curRoom ? 'room' : 'inbox';
  if (chat.view === 'inbox') send({ t: 'inbox' });
}

export function sendDM(text) {
  const b = text.trim();
  if (!b) return false;
  if (chat.thread.closed) { flash(MESSAGES.dm_closed); return false; }
  const why = sendBlocker();
  if (why) { flash(why); return false; }
  if (!ws || ws.readyState !== 1) { chat.status = 'Offline. Try again when you are connected.'; return false; }
  encryptAndSend(chat.thread.h, b);
  return true;
}

// Why a message can not be sent right now, or '' when it can. The text is never sent unencrypted.
function sendBlocker() {
  if (chat.e2e.state === 'unsupported') return 'This browser cannot encrypt messages, so direct messages are off here.';
  if (chat.e2e.state !== 'ready') return chat.e2e.state === 'mismatch' || chat.e2e.state === 'other_device'
    ? 'Your encryption key is on another device. Open Settings, Encrypted messages, to use this device.' : 'Encryption is still starting. Try again in a moment.';
  const sec = chat.thread.sec;
  if (sec.state === 'changed') return '@' + chat.thread.h + "'s encryption key changed. Check the safety code and accept it first.";
  if (sec.state === 'peer_none') return '@' + chat.thread.h + ' has not set up encrypted messages yet.';
  if (sec.state !== 'ok') return 'Still checking encryption. Try again in a moment.';
  return '';
}

async function encryptAndSend(peer, text) {
  try {
    const mine = await E.loadIdentity(chat.me);
    const served = chat.thread.sec.served;
    if (!mine || !served) throw new Error('no_key');
    const env = await E.encryptMessage(chat.me, peer, mine, served, text);
    send({ t: 'dm', h: peer, e: env });
  } catch (e) {
    flash('That message could not be encrypted, so it was not sent.');
  }
}

export const friendAct = (a, h) => send({ t: 'fr', a, h });
export const blockAct = (a, h) => send({ t: 'blk', a, h });
// Reporting a direct message: the person ticks the messages they want to share. Only those are revealed, each with
// its own key, and the server checks that the key really opens the stored message. Nothing else is sent.
export function openDMReport(id) {
  if (!chat.thread.items.some(x => x.i === id)) return false;
  return openReport({ kind: 'dm', h: chat.thread.h, ids: [id] });
}
export function toggleDMReportItem(id) {
  const r = chat.reportSheet;
  if (!r || r.kind !== 'dm') return;
  r.err = '';
  if (r.ids.includes(id)) r.ids = r.ids.filter(x => x !== id);
  else if (r.ids.length < 5) r.ids = [...r.ids, id];
  else r.err = 'You can share up to 5 messages in one report.';
}
async function submitDMReport() {
  const r = chat.reportSheet;
  if (!r.ids.length) { r.err = 'Tick at least one message to report.'; return false; }
  r.pending = true; r.err = '';
  try {
    const keys = await E.pinnedKeys(chat.me, r.h);
    const served = chat.thread.sec.served;
    if (served) keys[served.fp] = served.pub;
    const it = [];
    for (const id of r.ids) {
      const m = chat.thread.items.find(x => x.i === id);
      if (!m || mine(m)) throw new Error('gone');
      if (m.e) it.push({ i: id, k: await E.revealKey(chat.me, r.h, m.e, m.f, m.w, keys) });
      else it.push({ i: id, k: '' });
    }
    if (!send({ t: 'drep', c: r.c, it })) throw new Error('offline');
  } catch (e) {
    r.pending = false;
    r.err = e.message === 'offline' ? 'You are offline.' : 'Could not prepare those messages for the report.';
  }
  return true;
}
const mine = m => sameHandle(m.f, chat.me);

// ---- encryption keys ----
async function initE2EE() {
  if (!E.supported()) { chat.e2e.state = 'unsupported'; return; }
  try {
    chat.e2e.persistent = await E.keysArePersistent();
    send({ t: 'ekeyget', h: chat.me }); // the key the server has on file for me
  } catch (e) { chat.e2e.state = 'error'; }
}

async function syncMyKey(served) {
  const id = await E.loadIdentity(chat.me);
  if (!served.fp) {
    // Nothing registered yet: first set up on this account. A key is made here and only its public half is sent.
    const mineKey = id || await E.createIdentity(chat.me);
    chat.e2e.state = 'registering';
    send({ t: 'ekey', pub: mineKey.pub });
    return;
  }
  chat.e2e.fp = served.fp;
  if (id && id.fp === served.fp) { chat.e2e.state = 'ready'; if (chat.view === 'thread') askThread(chat.thread.h); return; }
  chat.e2e.state = id ? 'mismatch' : 'other_device';
}

// Make a new key on this device and replace the one the server has. Needs the account password. Messages made for
// the old key stay readable only on the device that holds it.
export async function useThisDevice(password) {
  if (chat.e2e.busy) return false;
  if (chat.hasPw && !password) { chat.e2e.err = 'Enter your password to continue.'; return false; }
  chat.e2e.busy = true; chat.e2e.err = '';
  try {
    const id = await E.createIdentity(chat.me);
    if (!send({ t: 'ekey', pub: id.pub, o: password || '' })) throw new Error('offline');
  } catch (e) { chat.e2e.busy = false; chat.e2e.err = 'Could not make a key. Are you online?'; return false; }
  return true;
}

export async function checkPeerKey(m) {
  // Decide how far to trust the contact's key, then decrypt the thread.
  const peer = m.h;
  const served = m.ek && m.ek.fp ? m.ek : null;
  const sec = { state: 'loading', served: null, verified: false, code: '', peerFp: served ? served.fp : '', myFp: chat.e2e.fp };
  if (chat.e2e.state === 'unsupported') sec.state = 'unsupported';
  else if (!served) sec.state = 'peer_none';
  else {
    const c = await E.peerCheck(chat.me, peer, served);
    if (c.state === 'new') { await E.pinPeer(chat.me, peer, served, false); sec.state = 'ok'; sec.served = served; }
    else if (c.state === 'ok') { sec.state = 'ok'; sec.served = served; sec.verified = c.verified; }
    else if (c.state === 'changed') { sec.state = 'changed'; sec.oldFp = c.oldFp; sec.newKey = served; }
    const myFp = chat.e2e.fp || (m.mk && m.mk.fp) || '';
    sec.myFp = myFp;
    if (served && myFp) sec.code = await E.safetyCode(myFp, served.fp);
  }
  return sec;
}

async function decryptItems(peer, items, sec) {
  const keys = await E.pinnedKeys(chat.me, peer);
  if (sec.served) keys[sec.served.fp] = sec.served.pub;
  const out = [];
  for (const it of items) {
    if (!it.e) { out.push(Object.assign({}, it, { legacy: true })); continue; }
    let b = '', err = '';
    try { b = await E.openMessage(chat.me, peer, it.e, it.f, it.w, keys); }
    catch (e) { err = e.message; }
    out.push(Object.assign({}, it, { b, err }));
  }
  return out;
}

async function applyThread(m) {
  const sec = await checkPeerKey(m);
  const items = await decryptItems(m.h, m.m || [], sec);
  if (!sameHandle(m.h, chat.thread.h)) return;
  chat.thread.sec = sec;
  chat.thread.items = items;
  chat.thread.ready = true;
}

// Accept a contact's new key after seeing the safety code. Verification starts over for the new key.
export async function acceptPeerKey() {
  const sec = chat.thread.sec;
  if (sec.state !== 'changed' || !sec.newKey) return false;
  await E.pinPeer(chat.me, chat.thread.h, sec.newKey, false);
  askThread(chat.thread.h);
  return true;
}
export async function verifyPeer() {
  if (chat.thread.sec.state !== 'ok') return false;
  await E.markVerified(chat.me, chat.thread.h);
  chat.thread.sec.verified = true;
  return true;
}

async function previewChats(list) {
  const out = [];
  for (const c of list) {
    if (!c.e) { out.push(c); continue; }
    let b = 'Encrypted message';
    try {
      const keys = await E.pinnedKeys(chat.me, c.h);
      const from = c.m ? chat.me : c.h, to = c.m ? c.h : chat.me;
      b = await E.openMessage(chat.me, c.h, c.e, from, to, keys);
    } catch (e) { /* keep the placeholder */ }
    out.push(Object.assign({}, c, { b }));
  }
  chat.chats = out;
}

function refreshOpen(handle) {
  if (chat.view === 'user' && sameHandle(handle, chat.userView.h)) send({ t: 'profile', h: chat.userView.h });
  else if (chat.view === 'thread' && sameHandle(handle, chat.thread.h)) askThread(chat.thread.h);
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

async function onDM(m) {
  const mineMsg = sameHandle(m.f, chat.me);
  const other = mineMsg ? m.w : m.f;
  const reading = chat.view === 'thread' && sameHandle(other, chat.thread.h);
  if (m.e) {
    // Decrypt first. Only keys this person already accepted are used, so a swapped key never shows up as text.
    const sec = reading ? chat.thread.sec : { served: null };
    const keys = await E.pinnedKeys(chat.me, other);
    if (sec.served) keys[sec.served.fp] = sec.served.pub;
    try { m.b = await E.openMessage(chat.me, other, m.e, m.f, m.w, keys); m.err = ''; }
    catch (e) { m.b = ''; m.err = e.message; }
  } else m.legacy = true;
  if (mineMsg) m.st = stFor(m, other);
  const preview = m.b || 'Encrypted message';
  touchChat(other, Object.assign({}, m, { b: preview }), mineMsg, reading); // the list is updated here, no second trip to the server
  if (reading) {
    if (!chat.thread.items.some(x => x.i === m.i)) chat.thread.items.push(m);
    if (chat.thread.items.length > 200) chat.thread.items.splice(0, chat.thread.items.length - 200);
    if (!mineMsg) send(visible() ? { t: 'read', h: other } : { t: 'dack', h: other, ids: [m.i] });
    return;
  }
  if (mineMsg) return;
  send({ t: 'dack', h: other, ids: [m.i] }); // this device has it
  chat.dmUnread++;
  if (chat.prefs.nd) flash('New message from @' + m.f); // the pop up follows the Direct messages setting; the unread badge always counts
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

const DONE = { mute: 'Muted.', unmute: 'Unmuted.', kick: 'Removed for now.', promote: 'Moderator added.', demote: 'Moderator removed.', clearbio: 'Bio cleared.', close: 'Room closed.', del: 'Message deleted.', ban: 'Banned for good.', unban: 'Unbanned.', remove: 'Removed from the room.', complaint: 'Complaint settled.' };

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
export function setRoomNotices(on) {
  chat.roomNotices = !!on;
  try { localStorage.setItem('ss.roomNotices', on ? '1' : '0'); } catch (e) { /* private mode: it lasts until the page closes */ }
}

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
export const loadDir = () => send({ t: 'dir', sort: chat.dirSort });
export const ROOM_CATEGORIES = ['General', 'City', 'Campus', 'Sport', 'Music', 'Tech', 'Business', 'Creators', 'Entertainment', 'Lifestyle'];

// Every room a person can see: official rooms, the public community rooms (also empty ones) and their own.
// Official rooms first, then the community rooms in the order the server ranked them (see sortRooms in the server).
export function allRooms() {
  return directoryOrder(chat.rooms, chat.dir);
}

// ---- favourites, recent rooms, directory order, member list (step 20) ----

export const isFavourite = slug => chat.favSet.includes(slug);
export const toggleFavourite = slug => send({ t: 'fav', r: slug, fav: !isFavourite(slug) });
export const loadMyRooms = () => send({ t: 'myrooms' });
export function setDirSort(mode) {
  chat.dirSort = mode;
  loadDir();
  if (lastFindQ.length >= 2) findRooms(lastFindQ);
}

function resetMembers() {
  chat.members = { open: false, busy: false, err: '', list: [], on: 0, tot: 0, more: false, next: 0 };
}
export function openMembers() {
  if (!chat.curRoom) return;
  chat.members = { ...chat.members, open: true, busy: true, err: '', list: [], more: false, next: 0 };
  send({ t: 'members', off: 0 });
}
export function closeMembers() {
  if (chat.members.open) send({ t: 'mclose' });
  resetMembers();
}
export function moreMembers() {
  if (chat.members.busy || !chat.members.more) return;
  chat.members.busy = true;
  send({ t: 'members', off: chat.members.next });
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
  complaints: () => send({ t: 'acomp' }),
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
    thread: { h: '', s: 'none', cost: 1, items: [], ready: false, sec: { state: 'loading' } }, threadBack: 'inbox',
    e2e: { state: 'init', fp: '', persistent: null, err: '', busy: false },
    avatars: {}, photos: {}, myLook: '', userBack: 'hub', userView: { h: '', ready: false },
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
      if (chat.view === 'thread') askThread(chat.thread.h); // after a dropped connection
      else if (chat.view === 'inbox') send({ t: 'inbox' });
      loadDir(); // every public room, also the empty ones
      initE2EE();
      startPing();
      break;
    case 'me': updateMe(m); break;
    case 'wallet': chat.credits = m.c || 0; chat.wallet = m.l || []; break;
    case 'ib': chat.dmUnread = m.u || 0; chat.requests = m.q || 0; break;
    case 'inbox':
      learnAll(m.av);
      chat.inboxReady = true;
      chat.chats = m.c || []; chat.friends = m.f || []; chat.incoming = m.q || [];
      previewChats(chat.chats);
      chat.outgoing = m.p || []; chat.blockedUsers = m.x || [];
      break;
    case 'thread':
      learnAll(m.av);
      if (sameHandle(m.h, chat.thread.h)) {
        chat.thread.h = m.h; chat.thread.s = m.s; chat.thread.cost = m.c || 0; chat.thread.closed = !!m.cl;
        // Plain legacy rows show at once. Encrypted rows show as soon as they are decrypted (applyThread).
        chat.thread.items = (m.m || []).filter(x => !x.e).map(x => Object.assign({}, x, { legacy: true }));
        chat.thread.ready = true;
        if (!visible()) ackFrom(m.h, m.m); // opened while hidden: received, not read
        applyThread(m);
      }
      break;
    case 'ekeyis': if (sameHandle(m.h, chat.me)) syncMyKey(m); break;
    case 'ekeyok':
      chat.e2e.state = 'ready'; chat.e2e.fp = m.fp; chat.e2e.busy = false; chat.e2e.err = '';
      if (chat.view === 'thread') askThread(chat.thread.h);
      break;
    case 'dm': learn(m.f, m.fa); onDM(m); break;
    case 'rcpt': onReceipt(m); break;
    case 'uprofile':
      learn(m.h, m.v);
      if (sameHandle(m.h, chat.me)) chat.myJoined = m.j || 0;
      if (sameHandle(m.h, chat.userView.h)) chat.userView = Object.assign({}, m, { ready: true });
      break;
    case 'found': if (findPending > 0) findPending--; if (findPending === 0) chat.found = m.r || []; break; // an older answer never replaces a newer search
    case 'ufound': if (normUser(m.q) === chat.ufound.q) chat.ufound = { q: m.q, r: m.r || [], busy: false }; for (const u of m.r || []) learn(u.h, u.v); break;
    case 'scompok': if (chat.reportSheet) chat.reportSheet = null; flash('Sent to the admins. Moderators cannot see it.'); break;
    case 'comps': chat.complaints = m.r || []; break;
    case 'cn': chat.complaintCount = m.c || 0; if (chat.view === 'admin' && chat.section.admin === 'complaints') send({ t: 'acomp' }); break;
    case 'rmgrinfo': if (chat.manage && chat.manage.s === m.s) chat.manage = Object.assign({}, m, { ready: true }); break;
    case 'rleft': setJoined(m.s, false); if (chat.curRoom === m.s || !chat.curRoom) chat.roomJoined = false; flash('You left ' + m.n + '.'); break;
    case 'rs': if (chat.curRoom && chat.roomNotices) addNote('sys', '@' + m.h + (m.a === 'j' ? ' joined the room' : ' left the room')); break;
    case 'delready': chat.del.k = m.k; chat.del.stage = 2; chat.del.busy = false; chat.del.err = ''; break;
    case 'delok': case 'deleted':
      chat.token = ''; localStorage.removeItem('lobby.k'); sessionStorage.removeItem('lobby.r');
      chat.del = { stage: 0, k: '', err: '', busy: false };
      chat.view = 'join'; chat.joinBusy = false; chat.joinErr = 'Your account was deleted.'; chat.me = '';
      break;
    case 'dir': chat.dir = m.r || []; break;
    case 'mr': chat.favSet = m.fs || []; chat.favRooms = m.f || []; chat.recentRooms = m.v || []; break;
    case 'ml': { // a page of the member list
      if (m.r !== chat.curRoom || !chat.members.open) break;
      const g = chat.members;
      if (m.off === 0) { g.list = []; g.tot = m.tot || 0; }
      mergeMembers(g.list, m.m);
      g.on = Math.max(m.on || 0, g.list.filter(x => x.s === 'on').length);
      g.more = !!m.more; g.next = (m.off || 0) + (m.m || []).length; g.busy = false; g.err = '';
      break;
    }
    case 'mp': { // someone's status changed, or they came or went
      const g = chat.members;
      if (!g.open) break;
      g.tot = Math.max(0, g.tot + applyMember(g.list, m));
      g.on = g.list.filter(x => x.s === 'on').length;
      break;
    }
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
        resetEngage();
        loadedRoom = chat.curRoom;
        lastIds[chat.curRoom] = 0;
        clearReply();
      }
      chat.count = 0;
      chat.roomKind = m.k || ''; chat.roomOwner = m.o || ''; chat.roomFee = m.f || 0; chat.roomPw = !!m.w;
      chat.isHost = !!m.y; chat.invite = m.i || '';
      clearJoining();
      chat.roomJoined = !!m.j; // the server's own answer, not a guess
      setJoined(m.r, !!m.j);
      chat.joinPanel = null;
      if (chat.view !== 'thread') chat.view = 'room'; // a conversation opened from a room stays open after a reconnect
      break;
    case 'left': leaveToHub(); break;
    case 'pres': chat.presence[String(m.h).toLowerCase()] = m.o === '1'; break;
    case 'hist': for (const x of m.m || []) addMessage(x, false); break;
    case 'x19': // pin, poll and the reactions of the messages just loaded, in one frame
      if (m.r !== chat.curRoom) break;
      chat.pin = m.pin || null;
      chat.poll = m.poll || null;
      for (const [id, v] of Object.entries(m.rx || {})) chat.rx[id] = v;
      break;
    case 'pin': chat.pin = m.p || null; break;
    case 'poll': { // the answer to my request, or a change someone else made
      const mine = chat.pollBusy;
      chat.poll = m.p || null;
      endPollRequest('');
      if (mine || (m.p && !m.p.c)) chat.pollForm = false; // typed text stays when an unrelated poll frame arrives
      break;
    }
    case 'rx': applyReaction(chat.rx, m, chat.me); break;
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
      delete chat.rx[m.i];
      if (chat.replyTo && chat.replyTo.i === m.i) clearReply();
      break;
    }
    case 'repok': if (chat.reportSheet) chat.reportSheet = null; addNote('up t1', 'Thanks. A moderator will take a look.'); if (chat.view !== 'room') flash('Thanks. A moderator will take a look.'); break;
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
  if (chat.pollBusy && POLL_ERRORS.includes(m.c)) { endPollRequest(errText(m)); return; } // the answer to a poll request: shown in the form
  if (chat.joining) clearJoining(); // a refused join: the button goes back to Join and the error below says why
  if (chat.e2e.busy) { chat.e2e.busy = false; chat.e2e.err = errText(m); return; }
  if (m.c === 'slow_down' && chat.ufound.busy) { // the person is typing faster than the server allows: ask again, show nothing
    const q = chat.ufound.q;
    clearTimeout(retryTimerS);
    retryTimerS = setTimeout(() => { if (chat.ufound.q === q) searchUsers(q); }, 300);
    return;
  }
  if (m.c === 'key_changed' && chat.view === 'thread') { askThread(chat.thread.h); flash(MESSAGES.key_changed); return; }
  if (m.c === 'kicked') { showBlocked(m.m || 1); return; }
  if (m.c === 'banned' && joined()) {
    chat.token = ''; localStorage.removeItem('lobby.k');
    chat.blockedMsg = 'Your account was permanently banned.'; chat.view = 'blocked';
    return;
  }
  if (chat.reportSheet && chat.reportSheet.pending) {
    chat.reportSheet.pending = false;
    chat.reportSheet.err = errText(m);
    return;
  }
  if (chat.del.busy) { chat.del.busy = false; chat.del.err = errText(m); if (m.c === 'delete_expired') chat.del.stage = 1; return; }
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
    findPending = 0; // answers to requests of the old connection never come
    const hello = chat.token
      ? Object.assign({ t: 'hi', k: chat.token }, chat.curRoom ? { r: chat.curRoom, l: lastFor(chat.curRoom) } : {})
      : chat.loginMode ? { t: 'login', h: wantHandle, p: wantPass }
      : Object.assign({ t: 'hi', h: wantHandle, p: wantPass }, wantLook ? { v: wantLook } : {});
    sock.send(JSON.stringify(hello));
    if (document.visibilityState === 'hidden') sock.send('{"t":"bg"}'); // reconnected while in the background
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
    chat.inboxReady = false; // the list is asked for again after reconnecting
    chat.presence = {}; // unknown until the connection is back: never show a stale green dot
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
    // Tell the server when the page goes to the background: a phone that locks its screen cuts the connection
    // without the person leaving, and the server keeps them in the room longer for that.
    if (document.visibilityState === 'hidden') send({ t: 'bg' });
    else send({ t: 'fg' });
    if (document.visibilityState === 'visible') {
      unread = 0; document.title = 'STREETSYNC'; probe();
      if (chat.view === 'thread' && chat.thread.h && chat.thread.ready) send({ t: 'read', h: chat.thread.h }); // the conversation is on screen now
    }
  });
  window.addEventListener('pageshow', probe);
  window.addEventListener('focus', probe);
  window.addEventListener('online', probe);
  if (chat.token) {
    chat.view = chat.curRoom ? 'room' : 'hub';
    connect();
  }
}
