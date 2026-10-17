const { JSDOM } = require('jsdom');
const fs = require('fs');
const html = fs.readFileSync(require('path').join(__dirname, '..', 'web', 'index.html'), 'utf8');
let pass = 0, fail = 0;
const check = (name, cond, extra) => { if (cond) { pass++; console.log('PASS', name); } else { fail++; console.log('FAIL', name, extra || ''); } };

function boot(opts = {}) {
  const sockets = [];
  const dom = new JSDOM(html, {
    url: 'https://chat.example/', runScripts: 'dangerously', pretendToBeVisual: true,
    beforeParse(w) {
      const flush = () => { if (w.__lobbyFlush) w.__lobbyFlush(); };
      const origClick = w.HTMLElement.prototype.click;
      w.HTMLElement.prototype.click = function () { origClick.call(this); flush(); };
      const origDispatch = w.EventTarget.prototype.dispatchEvent;
      w.EventTarget.prototype.dispatchEvent = function (e) { const r = origDispatch.call(this, e); flush(); return r; };
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



const btn = (root, label) => [...root.querySelectorAll('button')].find(b => b.textContent.trim() === label);
const last = t => t.s.sent[t.s.sent.length - 1];
const sentOf = (t, type) => t.s.sent.filter(x => x.t === type);
const submitForm = (t, id) => t.$(id).dispatchEvent(new t.w.Event('submit', { cancelable: true }));

const fills = el => [...el.querySelectorAll('[fill]')].map(n => n.getAttribute('fill')).join(',');
const msgRow = (t, id) => t.d.querySelector('#log .m[data-id="' + id + '"]');
const moreBtn = el => el.querySelector('button[aria-label^="More actions"]');

function inRoom(extra = {}) {
  const t = boot();
  t.s.open(); t.s.recv(okf({ c: 20, h: 'Mike', ...extra })); t.s.recv({ t: 'rooms', r: ROOMS });
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' });
  t.s.recv({ t: 'hist', m: [
    { i: 1, h: 'Ann', b: 'hello', ts: 1, r: 0, av: 'f3' },
    { i: 2, h: 'Bob', b: 'hi there', ts: 2, r: 0, av: 'f3' },
    { i: 3, h: 'Cat', b: 'no look picked', ts: 3, r: 0 },
  ] });
  return t;
}

(async () => {
  // ---- the landing page and sign up
  let t = boot({ token: false });
  const picker = () => t.d.querySelector('#landing .lookpicker');
  check('the sign in page has a hero with the promise and the points', t.$('landing').textContent.includes('Real people.') && t.$('landing').querySelectorAll('.points li').length === 4);
  check('and a preview of a room, marked as an example', t.$('landing').querySelectorAll('.preview .pv-msg').length === 3 && t.$('landing').querySelector('.pv-tip').textContent.includes('Burger') && t.$('landing').querySelector('.pv-head small').textContent === 'Example');
  check('and a footer line', t.$('landing').querySelector('.legal').textContent.includes('Free to join'));
  check('the app header is hidden on this page', t.$('shell').classList.contains('joining') && t.$('tabs').hidden);
  check('there is an avatar picker with eight looks to choose from', !!picker() && picker().querySelectorAll('.lookbtn').length === 8);
  check('the neutral set is chosen first, with its first look selected', picker().querySelector('.lookbtn.sel').dataset.look === 'x0');
  const setBtn = label => [...picker().querySelectorAll('.sets button')].find(b => b.textContent === label);
  setBtn('Female').click();
  check('choosing Female shows the female set and selects its first look', picker().querySelector('.lookbtn.sel').dataset.look === 'f0' && [...picker().querySelectorAll('.lookbtn')].every(b => b.dataset.look[0] === 'f'));
  setBtn('Male').click();
  check('choosing Male shows the male set', [...picker().querySelectorAll('.lookbtn')].every(b => b.dataset.look[0] === 'm') && picker().querySelector('.lookbtn.sel').dataset.look === 'm0');
  const looks = [...picker().querySelectorAll('.lookbtn')].map(b => fills(b));
  check('the eight male looks are all different', new Set(looks).size === 8);
  setBtn('Female').click();
  t.d.querySelector('#landing .lookpicker [data-look="f3"]').click();
  check('picking a look selects it', t.d.querySelector('#landing .lookpicker [data-look="f3"]').classList.contains('sel') && t.d.querySelector('#landing .lookpicker [data-look="f3"]').getAttribute('aria-checked') === 'true');
  t.$('handle').value = 'Ada'; t.$('pass').value = 'secret pass'; t.$('enter').click(); t.sockets[0].open();
  check('the pick is sent with the handle and password', JSON.stringify(t.sockets[0].sent[0]) === JSON.stringify({ t: 'hi', h: 'Ada', p: 'secret pass', v: 'f3' }), JSON.stringify(t.sockets[0].sent[0]));
  t = boot({ token: false });
  t.$('toggle').click();
  check('logging in has no picker', !t.d.querySelector('#landing .lookpicker'));
  t.$('handle').value = 'Ada'; t.$('pass').value = 'secret pass'; t.$('enter').click(); t.sockets[0].open();
  check('and sends no look', JSON.stringify(t.sockets[0].sent[0]) === JSON.stringify({ t: 'login', h: 'Ada', p: 'secret pass' }), JSON.stringify(t.sockets[0].sent[0]));

  // ---- everyone sees the same picture
  t = inRoom();
  const av = id => msgRow(t, id).parentElement.querySelector('.av');
  check('two people with the same look are drawn the same', fills(av(1)) === fills(av(2)) && fills(av(1)) !== fills(av(3)));
  t.s.recv({ t: 'm', i: 4, h: 'Cat', b: 'now with a look', ts: 4, r: 0, av: 'm2' });
  check('a look that arrives later changes how that person is drawn everywhere', fills(av(3)) === fills(av(4)) && fills(av(3)) !== '');
  const before = fills(av(1));
  t.s.recv({ t: 'dm', i: 9, f: 'Ann', w: 'Mike', b: 'psst', ts: 5, fa: 'm5' });
  check('a direct message can update a look too', fills(av(1)) !== before);
  t.s.recv({ t: 'inbox', c: [], f: ['Ann'], q: [], p: [], x: [], av: { Ann: 'x6' } });
  t.s.recv({ t: 'thread', h: 'Zed', s: 'none', c: 1, m: [], av: { Zed: 'f1' } });
  check('conversation and inbox frames carry looks as well', fills(av(1)) !== before);

  // ---- changing my own look
  t.s.recv({ t: 'left' });
  t = boot(); t.s.open(); t.s.recv(okf({ c: 20, h: 'Mike', v: 'x1' })); t.s.recv({ t: 'rooms', r: ROOMS });
  t.$('tabProfile').click();
  check('the profile avatar is my look', !!t.$('profile').querySelector('.herocard .av'));
  t.$('lookToggle').click();
  const mine = () => t.$('profile').querySelector('.lookpicker');
  check('Change avatar opens the picker at my look', !!mine() && mine().querySelector('.lookbtn.sel').dataset.look === 'x1');
  mine().querySelector('[data-look="x4"]').click();
  check('picking sends the new look', JSON.stringify(t.s.sent[t.s.sent.length - 1]) === JSON.stringify({ t: 'av', c: 'x4' }), JSON.stringify(t.s.sent[t.s.sent.length - 1]));
  t.s.recv({ t: 'me', p: 19, r: 0, x: 300, s: 1, g: false, c: 20, v: 'x4' });
  check('the answer moves the selection', mine().querySelector('[data-look="x4"]').classList.contains('sel'));
  t.$('lookToggle').click();
  check('Done closes it', !mine());

  // ---- the sidebar works from inside a room
  t = inRoom();
  t.$('tabInbox').click();
  check('Messages from inside a room leaves the room first', JSON.stringify(t.s.sent.filter(x => x.t === 'leave')) === JSON.stringify([{ t: 'leave' }]) && !t.$('chat').hidden);
  t.s.recv({ t: 'left' }); t.s.recv({ t: 'rooms', r: ROOMS });
  check('and then shows Messages and asks for it', !t.$('inbox').hidden && t.$('chat').hidden && t.s.sent.some(x => x.t === 'inbox'));
  t.$('tabHub').click();
  check('Home works from Messages', !t.$('hub').hidden);
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' }); t.s.recv({ t: 'hist', m: [] });
  t.$('tabProfile').click(); t.s.recv({ t: 'left' });
  check('Profile from inside a room shows the profile and asks for the wallet', !t.$('profile').hidden && t.s.sent.some(x => x.t === 'wallet'));
  t.$('tabHub').click();
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' }); t.s.recv({ t: 'hist', m: [{ i: 1, h: 'Matt', b: 'hey', ts: 1, r: 0 }] });
  moreBtn(msgRow(t, 1)).click(); btn(msgRow(t, 1), 'Message').click();
  check('a conversation opened from a room is open', !t.$('thread').hidden);
  t.$('tabHub').click(); t.s.recv({ t: 'left' });
  check('Home from a conversation that came from a room leaves the room too', !t.$('hub').hidden && t.s.sent.filter(x => x.t === 'leave').length === 3);
  const t2 = inRoom();
  t2.s.drop();
  t2.$('tabInbox').click();
  check('offline, the sidebar still moves on', !t2.$('inbox').hidden);

  // ---- a person's profile
  t = inRoom();
  t.d.querySelector('#log .m[data-id="1"]').closest('.row').querySelector('.avbtn').click();
  check('tapping an avatar asks for that person\'s profile', JSON.stringify(t.s.sent[t.s.sent.length - 1]) === JSON.stringify({ t: 'profile', h: 'Ann' }) && !t.$('userProfile').hidden && t.$('chat').hidden, JSON.stringify(t.s.sent[t.s.sent.length - 1]));
  check('while loading it says so and the header names them', t.$('userProfile').textContent.includes('Loading') && t.$('title').textContent === '@Ann' && !t.$('back').hidden);
  const prof = (extra = {}) => t.s.recv({ t: 'uprofile', h: 'Ann', v: 'f3', r: 1, p: 450, j: Date.UTC(2026, 9, 5), on: true, s: 'none', ...extra });
  prof();
  check('the profile shows name, status, tier, points and the month they joined', t.$('userName').textContent === '@Ann' && t.$('userState').textContent.includes('Online now') && t.$('userTier').textContent === 'Regular' && t.$('userPoints').textContent === '450' && t.$('userJoined').textContent.includes('2026'));
  check('a stranger can be messaged, added or blocked', !!t.$('userMessage') && !!t.$('userFriend') && !!t.$('userBlock') && !t.$('userRemove') && !t.$('userUnblock'));
  t.$('userMessage').click();
  check('Message opens the conversation', JSON.stringify(t.s.sent[t.s.sent.length - 1]) === JSON.stringify({ t: 'thread', h: 'Ann' }) && !t.$('thread').hidden);
  t.$('back').click(); t.$('back').click();
  check('back from the conversation and then from the profile returns to the room', !t.$('chat').hidden || !t.$('inbox').hidden);
  t = inRoom();
  t.d.querySelector('#log .m[data-id="1"] .who').click(); prof();
  t.$('userFriend').click();
  check('Add friend sends the request', JSON.stringify(t.s.sent[t.s.sent.length - 1]) === JSON.stringify({ t: 'fr', a: 'add', h: 'Ann' }));
  const n1 = t.s.sent.filter(x => x.t === 'profile').length;
  t.s.recv({ t: 'frok', a: 'add', h: 'Ann', s: 'sent' });
  check('and the profile reloads to show the new state', t.s.sent.filter(x => x.t === 'profile').length === n1 + 1);
  prof({ s: 'sent' });
  check('a sent request can be cancelled', !!t.$('userCancel') && !t.$('userFriend'));
  t.$('userCancel').click();
  check('Cancel sends the right frame', JSON.stringify(t.s.sent[t.s.sent.length - 1]) === JSON.stringify({ t: 'fr', a: 'cancel', h: 'Ann' }));
  prof({ s: 'got' });
  check('a request to me can be accepted or declined', !!t.$('userAccept') && !!t.$('userDecline'));
  t.$('userAccept').click();
  check('Accept sends the right frame', JSON.stringify(t.s.sent[t.s.sent.length - 1]) === JSON.stringify({ t: 'fr', a: 'accept', h: 'Ann' }));
  t.$('userDecline').click();
  check('Decline sends the right frame', JSON.stringify(t.s.sent[t.s.sent.length - 1]) === JSON.stringify({ t: 'fr', a: 'decline', h: 'Ann' }));
  prof({ s: 'friend', on: false });
  check('a friend can be removed, and shows as offline', !!t.$('userRemove') && t.$('userState').textContent.includes('Offline'));
  t.$('userRemove').click();
  check('Remove friend sends the right frame', JSON.stringify(t.s.sent[t.s.sent.length - 1]) === JSON.stringify({ t: 'fr', a: 'remove', h: 'Ann' }));
  t.$('userBlock').click();
  check('Block sends the right frame', JSON.stringify(t.s.sent[t.s.sent.length - 1]) === JSON.stringify({ t: 'blk', a: 'add', h: 'Ann' }));
  prof({ s: 'blocked' });
  check('someone I blocked can be unblocked, and there is no Block', !!t.$('userUnblock') && !t.$('userBlock'));
  t.$('userUnblock').click();
  check('Unblock sends the right frame', JSON.stringify(t.s.sent[t.s.sent.length - 1]) === JSON.stringify({ t: 'blk', a: 'del', h: 'Ann' }));
  t.s.recv({ t: 'uprofile', h: 'Boss', o: 'admin', j: Date.UTC(2026, 0, 1), on: true, s: 'none' });
  check('a profile for another person is ignored', t.$('userName').textContent === '@Ann');
  prof({ h: 'Ann', r: undefined, p: undefined, o: 'mod' });
  check('staff show a badge and no tier or points', !!t.$('userProfile').querySelector('.bd.r-mod') && !t.$('userTier') && !t.$('userPoints'));
  t.s.recv({ t: 'uprofile', h: 'Ann', s: 'none', r: 0, p: 0, j: 0, on: false });
  check('a profile with no join date still shows', !t.$('userJoined') && t.$('userName').textContent === '@Ann');
  t.$('back').click();
  check('back returns to the room', !t.$('chat').hidden && t.$('userProfile').hidden);
  // my own profile
  t.d.querySelector('#log .m[data-id="1"] .who').click();
  t.s.recv({ t: 'left' });
  t.s.recv({ t: 'uprofile', h: 'Mike', r: 0, p: 19, j: Date.now(), on: true });
  t = inRoom();
  t.s.recv({ t: 'm', i: 5, h: 'Mike', b: 'mine', ts: 5, r: 0 });
  t.d.querySelector('#log .m[data-id="5"] .who').click();
  t.s.recv({ t: 'uprofile', h: 'Mike', r: 0, p: 19, j: Date.now(), on: true });
  check('my own name opens my profile with a link to my wallet and settings, and no actions on myself', t.$('userProfile').textContent.includes('This is you') && !!t.$('userMine') && !t.$('userMessage') && !t.$('userBlock'));
  t.$('userMine').click();
  check('that link goes to my profile tab', t.s.sent.some(x => x.t === 'leave'));

  // ---- the host of a community room is a link
  t = boot(); t.s.open(); t.s.recv(okf({ c: 20, h: 'Mike' })); t.s.recv({ t: 'rooms', r: ROOMS });
  t.s.recv({ t: 'joined', r: 'c-1', n: 'Night Owls', k: 'public', o: 'Ann' }); t.s.recv({ t: 'hist', m: [] });
  check('a community room says who hosts it', t.$('roomMeta').textContent.includes('Hosted by') && t.$('roomMeta').textContent.includes('@Ann'));
  t.$('roomMeta').querySelector('button').click();
  check('and the host name opens their profile', JSON.stringify(t.s.sent[t.s.sent.length - 1]) === JSON.stringify({ t: 'profile', h: 'Ann' }));
  t.$('back').click();
  t.$('back').click(); t.s.recv({ t: 'left' });
  t.s.recv({ t: 'joined', r: 'c-1', n: 'Night Owls', k: 'public', o: 'Mike', y: true, i: 'ABCD2345' });
  check('the host does not see that line about themselves', !t.$('roomMeta'));
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' });
  check('official rooms have no host line', !t.$('roomMeta'));

  // ---- room icons are pictures
  t = boot(); t.s.open(); t.s.recv(okf({ c: 20 })); t.s.recv({ t: 'rooms', r: [
    { s: 'football', n: 'Sport', c: 'Sports', g: [], u: 0, h: 0 },
    { s: 'music', n: 'Music Heads', c: 'Entertainment', g: [], u: 0, h: 0 },
    { s: 'new-one', n: 'Brand New', c: 'Tech', g: [], u: 0, h: 0 },
    { s: 'c-9', n: 'Gist', c: 'Community', g: [], u: 1, h: 0, k: 'public', o: 'Ann' },
    { s: 'zz', n: 'Odd', c: 'Whatever', g: [], u: 0, h: 0 },
  ] });
  const icon = slug => t.d.querySelector('.room[data-slug="' + slug + '"] .icon').textContent;
  check('rooms are drawn with pictures: a trophy for Sport, notes for music', icon('football') === '\u{1F3C6}' && icon('music') === '\u{1F3B5}');
  check('a new room uses its category and an unknown one a speech bubble', icon('new-one') === '\u{1F4BB}' && icon('zz') === '\u{1F5E8}\uFE0F' && icon('c-9') === '\u{1F5E8}\uFE0F');
  check('the Sport room is called Sport', t.d.querySelector('.room[data-slug="football"]').textContent.includes('Sport'));

  // ---- logging out of a handle with no password
  t = boot(); t.s.open(); t.s.recv(okf({ c: 20, h: 'Mike', g: false })); t.s.recv({ t: 'rooms', r: ROOMS });
  t.$('tabProfile').click();
  t.$('logout').click();
  check('without a password, Log out asks first and explains the loss', !!t.$('logoutWarn') && t.$('logoutWarn').textContent.includes('@Mike') && t.$('logoutWarn').textContent.includes('can not get back') && t.$('toggle') !== null);
  check('and nothing was sent', !t.s.sent.some(x => x.t === 'bye'));
  t.$('logoutCancel').click();
  check('Cancel keeps me signed in', !t.$('logoutWarn') && !!t.$('logout') && !t.$('profile').hidden);
  t.$('logout').click(); t.$('logoutProtect').click();
  check('Protect opens the password form instead', !t.$('logoutWarn') && !t.$('secure').hidden);
  t.$('logout').click(); t.$('logoutAnyway').click();
  check('Log out anyway logs out', !t.$('join').hidden);
  t = boot(); t.s.open(); t.s.recv(okf({ c: 20, h: 'Mike', g: true })); t.s.recv({ t: 'rooms', r: ROOMS });
  t.$('tabProfile').click(); t.$('logout').click();
  check('with a password Log out is one tap', !t.$('join').hidden && !t.$('logoutWarn'));

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
})();
