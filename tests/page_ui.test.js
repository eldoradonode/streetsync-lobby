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

(async () => {
  // ---- tabs and screens
  let t = boot();
  t.s.open(); t.s.recv(okf()); t.s.recv({ t: 'rooms', r: ROOMS });
  check('signed in members land on the hub with the tab bar', view(t) === 'hub' && !t.$('tabs').hidden && t.$('tabHub').classList.contains('active'));
  t.$('tabProfile').click();
  check('the Profile tab opens the profile', view(t) === 'profile' && t.$('title').textContent === 'Profile' && t.$('tabProfile').classList.contains('active') && !t.$('tabHub').classList.contains('active'));
  t.$('tabHub').click();
  check('the Home tab goes back', view(t) === 'hub' && t.$('title').textContent === 'Home');
  t.$('tabs').querySelectorAll('button')[1].click();
  [...t.d.querySelectorAll('#roomList .room')][0].click();
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' }); t.s.recv({ t: 'hist', m: [] });
  check('the tab bar is hidden inside a room', view(t) === 'room' && t.$('shell').classList.contains('inroom'));
  t.$('tabProfile').click();
  check('tabs do nothing while in a room', view(t) === 'room');

  // ---- profile content for a member
  t.$('back').click(); t.s.recv({ t: 'left' }); t.s.recv({ t: 'rooms', r: ROOMS });
  t.$('tabProfile').click();
  check('profile shows the member name, tier badge and points', t.$('myName').textContent === '@Me' && t.$('myBadge').textContent === 'NEW' && t.$('myPts').textContent === 'Newbie · 19 / 300 pts · day 1', t.$('myPts').textContent);
  check('profile shows an avatar', !!t.$('profile').querySelector('.av svg'));
  check('profile lists all five tiers with the current one marked', t.$('tiers').children.length === 5 && t.$('tiers').children[0].classList.contains('now'));
  check('the progress bar is shown for members', !t.$('bar').hidden && !t.$('myPts').hidden && !t.$('tiers').hidden);
  check('moderator tools are hidden for members', t.$('staffCard').hidden && !t.$('tabMod') && !t.$('tabAdmin') && !t.$('staffNav'));
  t.$('myPts').click();
  check('the points rules open', !t.$('rules').hidden);

  // ---- password from the profile
  check('the protect button is offered', !t.$('secureBtn').parentElement.hidden && t.$('secure').hidden);
  t.$('secureBtn').click();
  check('the password form opens', !t.$('secure').hidden);
  t.$('newpass').value = 'short';
  t.$('secure').dispatchEvent(new t.w.Event('submit', { cancelable: true }));
  check('a short password is refused in the page', t.$('secureErr').textContent.includes('8') && !t.s.sent.some(x => x.t === 'pw'));
  t.$('newpass').value = 'long enough pw';
  t.$('secure').dispatchEvent(new t.w.Event('submit', { cancelable: true }));
  check('a good password is sent', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'pw', p: 'long enough pw' }));
  t.s.recv({ t: 'err', c: 'has_password' });
  check('a server error shows inside the form', t.$('secureErr').textContent === 'This handle already has a password.');
  t.s.recv({ t: 'pwok' }); t.s.recv({ t: 'me', p: 19, r: 0, x: 300, s: 1, g: true });
  check('success closes the form and hides the button', t.$('secure').hidden && t.$('secureBtn').parentElement.hidden);
  t.$('secureBtn').parentElement.hidden = false;

  // ---- staying on the profile through a reconnect
  t.s.drop();
  await new Promise(r => setTimeout(r, 700));
  const s2 = t.sockets[1]; s2.open(); s2.recv(okf({ g: true })); s2.recv({ t: 'rooms', r: ROOMS });
  check('a reconnect keeps you on the screen you were on', view(t) === 'profile');

  // ---- log out
  t.$('logout').click();
  check('log out tells the server', JSON.stringify(s2.sent.pop()) === JSON.stringify({ t: 'bye' }));
  check('log out returns to the sign in screen', view(t) === 'join' && t.$('title').textContent === 'STREETSYNC');
  check('log out forgets the login and the room', t.w.localStorage.getItem('lobby.k') === null && t.w.sessionStorage.getItem('lobby.r') === null);
  check('log out closes the connection', s2.closed === true);
  await new Promise(r => setTimeout(r, 900));
  check('and nothing reconnects afterwards', t.sockets.length === 2);
  check('the next person starts clean', t.d.querySelectorAll('#roomList .room').length === 0 && t.$('myName').textContent === '@');

  // ---- staff
  t = boot();
  t.s.open(); t.s.recv(okf({ o: 'admin', h: 'Boss' })); t.s.recv({ t: 'rooms', r: ROOMS });
  const rooms = () => [...t.d.querySelectorAll('#roomList .room')];
  check('staff see no locks', rooms().every(r => !r.classList.contains('locked') && !r.querySelector('.lock')));
  rooms()[1].click();
  check('staff can open the Elite Lounge', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'join', r: 'elite-lounge', l: 0 }));
  t.s.recv({ t: 'joined', r: 'elite-lounge', n: 'Elite Lounge' }); t.s.recv({ t: 'hist', m: [] });
  t.$('back').click(); t.s.recv({ t: 'left' }); t.s.recv({ t: 'rooms', r: ROOMS });
  t.$('tabProfile').click();
  check('staff profile shows the role badge, not a tier', t.$('myBadge').textContent === 'ADMIN' && t.$('myBadge').classList.contains('r-admin'));
  check('staff profile hides points, bar and tiers', t.$('myPts').hidden && t.$('bar').hidden && t.$('tiers').hidden && t.$('rules').hidden);
  check('staff profile says they can enter every room', t.$('profile').textContent.includes('Admin accounts can enter every room'));
  check('staff get the staff tools card with the moderation and admin buttons', !t.$('staffCard').hidden && !!t.$('staffMod') && !!t.$('staffAdmin'));
  t.s.recv({ t: 'rn', c: 3 });
  check('the Profile tab shows how many reports are open', t.$('tabProfile').querySelector('.badge').textContent === '3');
  t.s.recv({ t: 'me', p: 19, r: 0, x: 300, s: 1, g: true, o: 'mod' });
  check('a moderator sees the mod badge and the same access', t.$('myBadge').textContent === 'MOD' && rooms().every(r => !r.classList.contains('locked')));
  t.s.recv({ t: 'me', p: 19, r: 0, x: 300, s: 1, g: true });
  check('without a role the locks come back and the points show', rooms()[1].classList.contains('locked') && !t.$('myPts').hidden && !t.$('tiers').hidden);
  check('the report badge goes away without the role', !t.$('tabProfile').querySelector('.badge'));

  // ---- avatars and safe text
  t = boot();
  t.s.open(); t.s.recv(okf()); t.s.recv({ t: 'rooms', r: ROOMS });
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' });
  t.s.recv({ t: 'hist', m: [
    { i: 1, h: 'Matt', b: 'one', ts: 1, r: 0 }, { i: 2, h: 'matt', b: 'two', ts: 2, r: 0 },
    { i: 3, h: 'Ruth', b: 'three', ts: 3, r: 4 }, { i: 4, h: 'Boss', b: 'four', ts: 4, r: 0, o: 'admin' },
  ]});
  const av = id => t.d.querySelector('.m[data-id="' + id + '"]').parentElement.querySelector('.av');
  check('every message has an avatar', [1, 2, 3, 4].every(i => !!av(i) && !!av(i).querySelector('svg')));
  const art = e => e.innerHTML.replace(/aria-label="[^"]*"/, '');
  check('the same handle gets the same avatar, whatever the case', art(av(1)) === art(av(2)));
  check('different handles get different avatars', art(av(1)) !== art(av(3)) && art(av(3)) !== art(av(4)));
  check('the avatar ring follows tier and role', av(3).style.getPropertyValue('--ring') === 'var(--t4)' && av(4).style.getPropertyValue('--ring') === '#e11d48', av(3).style.getPropertyValue('--ring') + ' ' + av(4).style.getPropertyValue('--ring'));
  check('avatars have a text alternative', av(1).querySelector('svg').getAttribute('aria-label') === 'Avatar of Matt');
  t.$('back').click(); t.s.recv({ t: 'left' }); t.s.recv({ t: 'rooms', r: ROOMS });
  const hubRooms = [...t.d.querySelectorAll('#roomList .room')];
  check('room names and tags from the server are shown as plain text', !t.w.pwned && !t.$('roomList').querySelector('img') && !t.$('roomList').querySelector('.tag2 b') && hubRooms[2].querySelector('.nm').textContent.startsWith('<img'));
  check('rooms get an illustrated icon, not a letter', hubRooms[0].querySelector('.icon').textContent === '\u{1F4AC}' && hubRooms[1].querySelector('.icon').textContent === '\u{1F451}');

  // ---- sign in screen
  t = boot({ token: false });
  check('with no saved login the sign in screen shows', view(t) === 'join' && t.sockets.length === 0 && t.$('app').hidden);
  t.$('toggle').click();
  check('the toggle switches to log in', !t.$('pass').hidden && t.$('enter').textContent === 'Log in' && t.$('toggle').textContent === 'I am new here');
  t.$('handle').value = 'Ada'; t.$('pass').value = 'secret pass';
  t.$('join').dispatchEvent(new t.w.Event('submit', { cancelable: true }));
  t.sockets[0].open();
  check('log in sends the handle and password', JSON.stringify(t.sockets[0].sent[0]) === JSON.stringify({ t: 'login', h: 'Ada', p: 'secret pass' }), JSON.stringify(t.sockets[0].sent[0]));
  check('the button is disabled while connecting', t.$('enter').disabled);
  t.sockets[0].recv({ t: 'err', c: 'bad_login' }); t.sockets[0].drop();
  check('a failed log in shows the reason and frees the button', t.$('joinErr').textContent === 'Wrong handle or password.' && !t.$('enter').disabled && view(t) === 'join');
  t.$('handle').value = 'ab';
  t.$('join').dispatchEvent(new t.w.Event('submit', { cancelable: true }));
  check('a bad handle is refused before connecting', t.$('joinErr').textContent.includes('3 to 16') && t.sockets.length === 1);

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
})();
