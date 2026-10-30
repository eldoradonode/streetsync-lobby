const { JSDOM } = require('jsdom');
const fs = require('fs');
const html = fs.readFileSync(require('path').join(__dirname, 'build', 'page.html'), 'utf8');
let pass = 0, fail = 0;
const check = (name, cond, extra) => { if (cond) { pass++; console.log('PASS', name); } else { fail++; console.log('FAIL', name, extra || ''); } };

function boot(opts = {}) {
  const sockets = [];
  const dom = new JSDOM(html, {
    url: 'https://chat.example/', runScripts: 'dangerously', pretendToBeVisual: true,
    beforeParse(w) {
      try { Object.defineProperty(w, 'crypto', { value: require('crypto').webcrypto, configurable: true }); } catch (e) { /* none */ }
      const flush = () => { if (w.__lobbyFlush) w.__lobbyFlush(); };
      const origClick = w.HTMLElement.prototype.click;
      w.HTMLElement.prototype.click = function () { origClick.call(this); flush(); };
      const origDispatch = w.EventTarget.prototype.dispatchEvent;
      w.EventTarget.prototype.dispatchEvent = function (e) { const r = origDispatch.call(this, e); flush(); return r; };
      if (opts.wide) w.matchMedia = () => ({ matches: true, addEventListener() {}, removeEventListener() {} });
      if (opts.token !== false) w.localStorage.setItem('lobby.k', 'tok');
      if (opts.room) w.sessionStorage.setItem('lobby.r', opts.room);
      w.Element.prototype.scrollIntoView = function () {};
      const real = w.setTimeout.bind(w);
      w.setTimeout = (fn, ms, ...a) => (ms >= 60000 ? 0 : real(fn, ms, ...a));
      w.WebSocket = class {
        constructor() { this.readyState = 0; this.sent = []; this.closed = false; sockets.push(this); }
        send(d) { this.sent.push(JSON.parse(d)); }
        close() { this.readyState = 3; this.closed = true; }
        open() { this.readyState = 1; this.onopen && this.onopen(); flush(); }
        recv(o) { this.onmessage && this.onmessage({ data: JSON.stringify(o) }); flush(); }
        drop() { this.readyState = 3; this.onclose && this.onclose(); flush(); }
      };
    }
  });
  if (dom.window.__lobbyFlush) dom.window.__lobbyFlush();
  const w = dom.window, d = w.document;
  return { w, d, sockets, s: sockets[0], $: id => d.getElementById(id) };
}
const ROOMS = [
  { s: 'lobby', n: 'General Chat', c: 'General', g: ['Vibes'], u: 3, h: 1 },
  { s: 'elite-lounge', n: 'Elite Lounge', c: 'Members', g: ['Elite'], u: 0, h: 0, t: 3 },
  { s: 'x', n: '<img src=x onerror="window.pwned=1">', c: 'Test', g: ['<b>tag</b>'], u: 0, h: 0 },
];
const okf = (extra = {}) => ({ t: 'ok', h: 'Me', k: 'tok', p: 19, r: 0, x: 300, s: 1, g: false, ...extra });
const view = t => (!t.$('join').hidden ? 'join' : !t.$('blocked').hidden ? 'blocked' : !t.$('chat').hidden ? 'room' : !t.$('profile').hidden ? 'profile' : !t.$('hub').hidden ? 'hub' : 'none');


(async () => {
  const sub = (t, el) => el.dispatchEvent(new t.w.Event('submit', { cancelable: true }));
  const typeInto = (t, el, v) => { el.value = v; el.dispatchEvent(new t.w.Event('input', { bubbles: true })); };
  const signed = (opts = {}, extra = {}) => { const t = boot(opts); cur = t; t.s.open(); t.s.recv(okf({ c: 20, g: true, ...extra })); t.s.recv({ t: 'rooms', r: ROOMS }); return t; };
  const CHATS = [
    { h: 'ann', b: 'hello there', ts: 5000, m: false, u: 2 },
    { h: 'bob', b: 'see you later', ts: 4000, m: true, u: 0 },
    { h: 'cleo', b: 'match tonight?', ts: 3000, m: false, u: 0 },
    { h: 'zed', b: 'ok', ts: 2000, m: false, u: 0 },
  ];
  let cur = null;
  const li = n => cur.d.querySelectorAll('#chatList li')[n - 1];
  const inbox = (t, c = CHATS) => { t.$('tabInbox').click(); t.s.recv({ t: 'inbox', c, f: [], q: [], p: [], x: [] }); };

  // ---- two pane messages on a wide screen
  let t = signed({ wide: true });
  inbox(t);
  check('wide: the list is shown beside an empty conversation pane', !t.$('messages').hidden && !t.$('inbox').hidden && !t.$('threadEmpty').hidden && t.$('thread').hidden);
  check('wide: the empty pane says what to do', t.$('threadEmpty').textContent.includes('Select a conversation'));
  check('wide: the list is marked as the left pane', t.$('inbox').classList.contains('split') && t.$('messages').classList.contains('split'));
  li(1).querySelector('.chatrow').click();
  check('wide: choosing a conversation opens it beside the list', !t.$('thread').hidden && !t.$('inbox').hidden && t.$('threadEmpty').hidden);
  check('wide: it asks the server for that conversation', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'thread', h: 'ann' }));
  check('wide: opening a conversation clears its unread badge in the list at once', !li(1).querySelector('.badge'));
  check('wide: there is no back button, the list is already on screen', t.$('back').hidden);
  check('wide: the open conversation is highlighted', t.$('chatList').querySelector('li.sel') && t.$('chatList').querySelector('li.sel b').textContent === '@ann');
  t.s.recv({ t: 'thread', h: 'ann', s: 'friend', c: 0, m: [{ i: 1, f: 'ann', w: 'Me', b: 'hello there', ts: 5000 }] });
  li(2).querySelector('.chatrow').click();
  check('wide: switching conversation asks for the other one and moves the highlight', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'thread', h: 'bob' }) && t.$('chatList').querySelector('li.sel b').textContent === '@bob');
  const H16 = require('./helpers16');
  const E = await import('../ui/src/lib/e2ee.js');
  const me = await H16.handshake(t, 'Me');
  const bobId = await E.createIdentity('bob');
  t.s.recv({ t: 'thread', h: 'bob', s: 'friend', c: 0, m: [], ek: { pub: bobId.pub, fp: bobId.fp, ver: 1 }, mk: me });
  await H16.sleep(150);
  check('wide: the old conversation is gone from the right pane', !t.$('tlog').textContent.includes('hello there'));
  t.$('ttext').value = 'on my way'; sub(t, t.$('tsend'));
  await H16.sleep(150);
  const outDM = t.s.sent.pop();
  check('wide: an encrypted message is sent to the open conversation', outDM.t === 'dm' && outDM.h === 'bob' && !!outDM.e && outDM.b === undefined && !JSON.stringify(outDM).includes('on my way'), JSON.stringify(outDM));
  t.s.recv({ t: 'dm', i: 9, f: 'Me', w: 'bob', e: outDM.e, ts: 6000 });
  await H16.sleep(150);
  check('wide: it appears in the open conversation', t.$('tlog').textContent.includes('on my way'));
  check('wide: and the list moves it to the top with the new last message', li(1).querySelector('b').textContent === '@bob' && li(1).querySelector('small').textContent.includes('on my way'));
  t.s.recv({ t: 'dm', i: 10, f: 'cleo', w: 'Me', b: 'are you coming', ts: 7000 });
  check('wide: a message from someone else goes to the top of the list', li(1).querySelector('b').textContent === '@cleo');
  check('wide: and shows an unread badge, while the open conversation is unaffected', li(1).querySelector('.badge').textContent === '1' && !t.$('tlog').textContent.includes('are you coming'));
  t.s.recv({ t: 'dm', i: 11, f: 'bob', w: 'Me', b: 'great', ts: 8000 });
  check('wide: a message in the open conversation is not counted as unread', !li(1).querySelector('.badge') && li(1).querySelector('b').textContent === '@bob', [[...t.d.querySelectorAll('#chatList li')].map(l => l.querySelector('b').textContent + ':' + (l.querySelector('.badge') ? l.querySelector('.badge').textContent : '-'))].join(' '));
  check('wide: the open conversation reports it as read', t.s.sent.some(x => x.t === 'read' && x.h === 'bob'));
  check('wide: no duplicate rows in the list', t.d.querySelectorAll('#chatList li').length === 4);
  // search
  const many = Array.from({ length: 6 }, (_, i) => ({ h: 'user' + i, b: i === 3 ? 'pizza time' : 'msg ' + i, ts: 1000 + i, m: false, u: 0 }));
  t.s.recv({ t: 'inbox', c: many, f: [], q: [], p: [], x: [] });
  check('wide: with more than three conversations a search box appears', !!t.$('chatSearch'));
  typeInto(t, t.$('chatSearch'), 'pizza');
  check('search matches the last message', t.d.querySelectorAll('#chatList li').length === 1 && t.$('chatList').textContent.includes('user3'));
  typeInto(t, t.$('chatSearch'), 'USER1');
  check('search matches handles, ignoring case', t.d.querySelectorAll('#chatList li').length === 1 && t.$('chatList').textContent.includes('user1'));
  typeInto(t, t.$('chatSearch'), 'nothing like this');
  check('no match gives a message instead of an empty box', !!t.$('chatNoMatch') && t.$('chatNoMatch').textContent.includes('nothing like this'));
  typeInto(t, t.$('chatSearch'), '');
  check('clearing the search shows everything', t.d.querySelectorAll('#chatList li').length === 6);

  // a conversation opened from a room stays full width, with its own back button
  t = signed({ wide: true });
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' }); t.s.recv({ t: 'hist', m: [] });
  t.$('log').innerHTML === undefined;
  // narrow screens: the list first, then the conversation full screen
  t = signed({ wide: false });
  inbox(t);
  check('narrow: the list is shown alone, no empty pane', !t.$('inbox').hidden && t.$('threadEmpty').hidden && t.$('thread').hidden && !t.$('inbox').classList.contains('split'));
  li(1).querySelector('.chatrow').click();
  check('narrow: the conversation replaces the list', !t.$('thread').hidden && t.$('inbox').hidden);
  check('narrow: a back button is shown and returns to the list', !t.$('back').hidden);
  t.$('back').click(); t.s.recv({ t: 'inbox', c: CHATS, f: [], q: [], p: [], x: [] });
  check('narrow: back shows the list again', !t.$('inbox').hidden && t.$('thread').hidden);
  check('narrow: the tab bar is still the only navigation (no sidebar items leak in)', t.d.querySelectorAll('#tabs button:not(.sideonly)').length === 5);

  // friends only
  t = signed({ wide: false });
  inbox(t);
  li(1).querySelector('.chatrow').click();
  t.s.recv({ t: 'thread', h: 'ann', s: 'none', c: 1, m: [], cl: true });
  check('a friends only person shows why you cannot message them', !!t.$('closedNote') && t.$('closedNote').textContent.includes('only takes messages from friends'));
  check('and the box and Send are disabled', t.$('ttext').disabled && t.$('tsend').querySelector('button').disabled);
  t.s.sent.length = 0; t.$('ttext').value = 'hi'; sub(t, t.$('tsend'));
  check('nothing is sent', t.s.sent.length === 0);
  t.s.recv({ t: 'err', c: 'dm_closed' });
  check('a refusal from the server is explained', t.$('status').textContent.includes('only takes messages from friends'), t.$('status').textContent);
  t.s.recv({ t: 'thread', h: 'ann', s: 'friend', c: 0, m: [] });
  check('once friends the notice goes away and the box works', !t.$('closedNote') && !t.$('ttext').disabled);

  // ---- notifications
  t = signed();
  t.s.recv({ t: 'nc', c: 3 });
  check('the bell shows how many are unread', !t.$('bell').hidden && t.$('bellBadge').textContent === '3' && t.$('bell').getAttribute('aria-label').includes('3 unread'));
  t.s.recv({ t: 'nc', c: 0 });
  check('no badge when everything is read', !t.$('bellBadge'));
  t.s.recv({ t: 'nc', c: 120 });
  check('big counts are capped', t.$('bellBadge').textContent === '99+');
  t.s.recv({ t: 'nc', c: 5 });
  t.$('bell').click();
  check('the bell opens the list and asks for it', !t.$('notesPage').hidden && t.s.sent.some(x => x.t === 'nlist') && t.$('title').textContent === 'Notifications');
  check('before the list arrives it says loading', !!t.$('notesLoading'));
  const NOTES = [
    { i: 5, k: 'dm', a: 'ann', n: 3, ts: Date.now() - 60000, b: 'are you there?', u: 1 },
    { i: 4, k: 'tag', a: 'bob', n: 2, r: 'lobby', rn: 'General Chat', b: '<img src=x onerror="window.pwned=1"> hey @Me', ts: Date.now() - 600000, u: 1 },
    { i: 3, k: 'reply', a: 'cleo', n: 1, r: 'abuja', rn: 'Abuja', b: 'thanks', ts: Date.now() - 7200000, u: 1 },
    { i: 2, k: 'friend', a: 'dan', n: 1, b: 'request', ts: Date.now() - 86400000 * 2, u: 1 },
    { i: 1, k: 'tip', a: 'eve', n: 1, r: 'lobby', rn: 'General Chat', b: 'Pizza', m: 5, ts: Date.now() - 86400000 * 3 },
  ];
  t.s.recv({ t: 'notes', l: NOTES, c: 4 });
  const rows = () => [...t.d.querySelectorAll('#noteList li[data-id]')];
  check('every notification is listed', rows().length === 5 && !t.$('notesLoading') && !t.$('notesEmpty'));
  check('a grouped message shows its count', rows()[0].textContent.includes('@ann sent you 3 messages') && rows()[0].textContent.includes('are you there?'));
  check('a tag names the room and the repeat count', rows()[1].textContent.includes('@bob tagged you in General Chat (2)'));
  check('a reply says so', rows()[2].textContent.includes('@cleo replied to you in Abuja'));
  check('a friend request says so', rows()[3].textContent.includes('@dan sent you a friend request'));
  check('a gift shows what and how much', rows()[4].textContent.includes('@eve sent you a Pizza (5 credits) in General Chat'));
  check('html in a message stays text', !t.w.pwned && !rows()[1].querySelector('img') && rows()[1].textContent.includes('<img'));
  check('unread ones are marked and read ones are not', rows().slice(0, 4).every(r => r.classList.contains('unread') && r.querySelector('.udot')) && !rows()[4].classList.contains('unread'));
  check('the bell count follows the server count', t.$('bellBadge').textContent === '4');
  t.s.sent.length = 0;
  t.$('notesAll').click();
  check('Mark all read sends one request and clears everything', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'nread', all: true }) && !t.$('bellBadge') && !rows().some(r => r.classList.contains('unread')) && t.$('notesAll').disabled);
  t.s.recv({ t: 'notes', l: NOTES.map(n => ({ ...n, u: n.i === 5 || n.i === 3 ? 1 : 0 })), c: 2 });
  rows()[0].querySelector('.noterow').click();
  const sent = t.s.sent.slice(-2);
  check('opening a message notification marks it read and opens the conversation', JSON.stringify(sent[0]) === JSON.stringify({ t: 'nread', i: 5 }) && JSON.stringify(sent[1]) === JSON.stringify({ t: 'thread', h: 'ann' }) && !t.$('thread').hidden);
  check('the count drops by one', t.$('bellBadge').textContent === '1');
  t.$('tabHub').click();
  t.$('bell').click(); t.s.recv({ t: 'notes', l: NOTES.map(n => ({ ...n, u: n.i === 3 ? 1 : 0 })), c: 1 });
  t.s.sent.length = 0;
  rows()[1].querySelector('.noterow').click();
  check('a notification that is already read is not marked again', !t.s.sent.some(x => x.t === 'nread') && t.s.sent.some(x => x.t === 'join' && x.r === 'lobby'));
  t.s.recv({ t: 'left' });
  t = signed();
  t.$('bell').click(); t.s.recv({ t: 'notes', l: NOTES, c: 4 });
  rows()[3].querySelector('.noterow').click();
  check('a friend request opens Messages', !t.$('inbox').hidden);
  t.$('bell').click(); t.s.recv({ t: 'notes', l: NOTES, c: 4 });
  rows()[4].querySelector('.noterow').click();
  check('a gift opens the wallet', !t.$('walletPage').hidden);
  // live arrival
  t = signed();
  t.$('bell').click(); t.s.recv({ t: 'notes', l: [], c: 0 });
  check('an empty list explains what will show up', !!t.$('notesEmpty') && t.$('notesEmpty').textContent.includes('Messages, tags, replies, gifts'));
  t.s.recv({ t: 'nn', n: { i: 8, k: 'dm', a: 'ann', n: 1, b: 'hi', ts: Date.now(), u: 1 }, c: 1 });
  check('a new notification appears at once while the page is open', rows().length === 1 && t.$('bellBadge').textContent === '1' && !t.$('notesEmpty'));
  t.s.recv({ t: 'nn', n: { i: 8, k: 'dm', a: 'ann', n: 2, b: 'hi again', ts: Date.now(), u: 1 }, c: 1 });
  check('the same event updates its row instead of adding another', rows().length === 1 && rows()[0].textContent.includes('2 messages') && t.$('bellBadge').textContent === '1');
  t.s.recv({ t: 'nn', n: { i: 9, k: 'tip', a: 'bob', n: 1, b: 'Burger', m: 5, r: 'lobby', rn: 'General Chat', ts: Date.now(), u: 1 }, c: 2 });
  check('newest first', rows().length === 2 && rows()[0].dataset.id === '9');
  t.$('tabHub').click();
  t.s.recv({ t: 'nn', n: { i: 10, k: 'dm', a: 'cleo', n: 1, b: 'x', ts: 1, u: 1 }, c: 3 });
  check('elsewhere only the bell changes, nothing is fetched', t.$('bellBadge').textContent === '3');
  check('the bell is not shown inside a room or a conversation', (() => { t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' }); return t.$('bell').hidden; })());

  // ---- settings
  t = signed();
  t.s.recv({ t: 'pf', nd: true, nk: true, nt: false, nf: true, dp: 'everyone', so: true });
  t.$('tabSettings').click();
  check('Settings opens from the sidebar', !t.$('settingsPage').hidden && t.$('title').textContent === 'Settings');
  check('switches show the saved values', t.$('sw-nd').checked && t.$('sw-nk').checked && !t.$('sw-nt').checked && t.$('sw-nf').checked && !t.$('setFriendsOnly').checked && t.$('setShowOnline').checked);
  t.s.sent.length = 0;
  t.$('sw-nd').checked = false; t.$('sw-nd').dispatchEvent(new t.w.Event('change', { bubbles: true }));
  check('switching one off sends exactly that setting', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'set', k: 'nd', b: '0' }));
  t.$('setFriendsOnly').checked = true; t.$('setFriendsOnly').dispatchEvent(new t.w.Event('change', { bubbles: true }));
  check('friends only sends the privacy choice', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'set', k: 'dp', b: 'friends' }));
  t.$('setShowOnline').checked = false; t.$('setShowOnline').dispatchEvent(new t.w.Event('change', { bubbles: true }));
  check('hiding online status sends it', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'set', k: 'so', b: '0' }));
  t.s.recv({ t: 'pf', nd: false, nk: true, nt: true, nf: true, dp: 'friends', so: false });
  check('the server answer is what is shown', !t.$('sw-nd').checked && t.$('sw-nt').checked && t.$('setFriendsOnly').checked && !t.$('setShowOnline').checked);
  t.s.recv({ t: 'me', p: 19, r: 0, x: 300, s: 1, g: true, c: 20, bio: 'Abuja builder' });
  check('the bio shows and can be edited here', t.$('setBioText').textContent === 'Abuja builder' && t.$('setBioEdit').textContent === 'Edit bio');
  t.$('setBioEdit').click(); typeInto(t, t.$('setBio'), '  new   bio  '); sub(t, t.$('setBioForm'));
  check('saving sends the tidied bio', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'bio', b: 'new bio' }));
  t.$('setBioEdit') ;
  check('a password can be changed when the account has one', !!t.$('setPwForm') && !t.$('setSecure'));
  t.s.sent.length = 0;
  typeInto(t, t.$('setOldPw'), 'old password 1'); typeInto(t, t.$('setNewPw'), 'short'); sub(t, t.$('setPwForm'));
  check('a short new password is refused before sending', t.s.sent.length === 0 && t.$('status').textContent.includes('8 characters'));
  typeInto(t, t.$('setNewPw'), 'a new password 2'); sub(t, t.$('setPwForm'));
  check('the change is sent with the old and the new password', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'pwchange', o: 'old password 1', p: 'a new password 2' }));
  check('the boxes are cleared after sending', t.$('setOldPw').value === '' && t.$('setNewPw').value === '');
  t.s.recv({ t: 'pwchanged' });
  check('success is confirmed and says other devices were logged out', t.$('status').textContent.includes('other devices'));
  t.s.recv({ t: 'err', c: 'bad_login' });
  check('a wrong current password shows an error', t.$('status').textContent.length > 0);
  // a guest has no password to change
  t = signed({}, { g: false });
  t.$('tabSettings').click();
  check('a handle without a password is offered protection instead', !t.$('setPwForm') && !!t.$('setSecure'));
  t.$('setSecure').click();
  check('and it takes them to the password box', !t.$('profile').hidden && !t.$('secure').hidden);
  // offline
  t = signed(); t.$('tabSettings').click(); t.s.drop();
  t.$('sw-nk').checked = false; t.$('sw-nk').dispatchEvent(new t.w.Event('change', { bubbles: true }));
  check('a setting changed while offline is not pretended to be saved', t.$('status').textContent.includes('offline') || t.$('sw-nk').checked, t.$('status').textContent);

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
})().catch(e => { console.log('CRASH', e && e.stack ? e.stack.split('\n').slice(0, 4).join(' | ') : e); process.exit(1); });
