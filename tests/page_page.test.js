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
      if (opts.token) w.localStorage.setItem('lobby.k', opts.token);
      w.WebSocket = class {
        constructor(url) { this.url = url; this.readyState = 0; this.sent = []; sockets.push(this); }
        send(d) { this.sent.push(JSON.parse(d)); }
        close() { this.readyState = 3; }
        open() { this.readyState = 1; this.onopen && this.onopen(); flush(); }
        recv(o) { this.onmessage && this.onmessage({ data: JSON.stringify(o) }); flush(); }
        drop() { this.readyState = 3; this.onclose && this.onclose(); flush(); }
      };
    }
  });
  if (dom.window.__lobbyFlush) dom.window.__lobbyFlush();
  return { dom, w: dom.window, d: dom.window.document, sockets };
}
const ok = (h, extra = {}) => ({ t: 'ok', h, k: 'tok', p: 19, r: 0, x: 300, s: 1, g: false, ...extra });

(async () => {
  let { w, d, sockets } = boot();
  const $ = id => d.getElementById(id);
  check('starts on the join form', !$('join').hidden && $('chat').hidden);
  check('rules list has 5 tiers', $('tiers').children.length === 5);
  check('rules start hidden', $('rules').hidden);

  const submitForm = () => $('join').dispatchEvent(new w.Event('submit', { cancelable: true }));
  check('sign up asks for an account, not just a handle', $('joinTitle').textContent === 'Create your account' && $('enter').textContent === 'Create account');
  check('the password box is shown when signing up', !$('pass').hidden && $('pass').getAttribute('autocomplete') === 'new-password');
  $('handle').value = '@Eldorado';
  submitForm();
  check('no password: no connection is made and the person is told', sockets.length === 0 && /8 characters/.test($('joinErr').textContent), $('joinErr').textContent);
  $('pass').value = 'short'; submitForm();
  check('a short password is refused before connecting', sockets.length === 0 && /8 characters/.test($('joinErr').textContent));
  check('the password is hidden until Show is pressed', $('pass').type === 'password');
  $('showPass').click(); if (w.__lobbyFlush) await w.__lobbyFlush();
  check('Show reveals it and Hide hides it again', $('pass').type === 'text' && $('showPass').textContent === 'Hide');
  $('showPass').click(); if (w.__lobbyFlush) await w.__lobbyFlush();
  check('and back', $('pass').type === 'password');
  $('pass').value = 'secret pass'; submitForm();
  const s = sockets[0]; s.open();
  check('join sends hi with the handle, the password and the avatar picked', JSON.stringify(s.sent[0]) === JSON.stringify({ t: 'hi', h: 'Eldorado', p: 'secret pass', v: 'x0' }), s.sent[0]);

  s.recv(ok('Eldorado'));
  s.recv({ t: 'rooms', r: [{ s: 'lobby', n: 'General Chat', c: 'General', g: [], u: 1, h: 0 }] });
  s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' });
  s.recv({ t: 'hist', m: [
    { i: 1, h: 'Matt', b: 'Same thing I thought', ts: 1, r: 0 },
    { i: 2, h: 'James', b: 'hey @eldorado are you there', ts: 2, r: 2 },
    { i: 3, h: 'Eldorado', b: 'talking to @James and @Matt', ts: 3, r: 0 },
    { i: 4, h: 'Matt', b: '<img src=x onerror="window.pwned=1"> <b>bold</b>', ts: 4, r: 0 },
    { i: 5, h: 'Matt', b: 'mail me at matt@example.com please', ts: 5, r: 1 },
    { i: 6, h: 'James', b: 'no tag here but @eldoradox is someone else', ts: 6, r: 0 },
  ]});
  check('chat is shown after ok', $('join').hidden && !$('chat').hidden);
  const msgs = [...d.querySelectorAll('#log .m')];
  check('six messages rendered', msgs.length === 6, msgs.length);
  check('message that tags me is highlighted', msgs[1].classList.contains('mention') && msgs[1].querySelector('.tag.tome').textContent === '@eldorado');
  check('my own message is not a mention', !msgs[2].classList.contains('mention') && msgs[2].classList.contains('me'));
  check('other handles are tagged but not as me', [...msgs[2].querySelectorAll('.tag')].map(t => t.textContent).join(',') === '@James,@Matt' && !msgs[2].querySelector('.tome'));
  check('html in a message stays text', !w.pwned && !msgs[3].querySelector('img') && !msgs[3].querySelector('b') && msgs[3].textContent.includes('<img src=x'));
  check('email address is not a tag', msgs[4].querySelectorAll('.tag').length === 0);
  check('a longer handle is not mistaken for me', !msgs[5].classList.contains('mention') && msgs[5].querySelectorAll('.tome').length === 0);
  check('tier class and badge on messages', msgs[1].classList.contains('t2') && msgs[1].querySelector('.bd').textContent === 'INS' && msgs[4].querySelector('.bd').textContent === 'REG');
  const colors = ['Matt', 'James', 'Eldorado'].map(n => [...d.querySelectorAll('.who')].find(b => b.textContent === '@' + n).style.getPropertyValue('--nc'));
  check('names get colors from the palette', colors.every(c => /^#[0-9a-f]{6}$/.test(c)), colors);
  const again = [...d.querySelectorAll('.who')].filter(b => b.textContent === '@Matt').map(b => b.style.getPropertyValue('--nc'));
  check('same handle always gets the same color', new Set(again).size === 1);
  check('the three friends are not all one color', new Set(colors).size >= 2, colors);

  // tapping a name opens the profile, and Mention is in the menu
  const text = $('text');
  const mentionVia = name => {
    const row = [...d.querySelectorAll('.who')].find(b => b.textContent === name).closest('.m');
    row.querySelector('button[aria-label^="More actions"]').click();
    [...row.querySelectorAll('.actions button')].find(b => b.textContent === 'Mention').click();
  };
  mentionVia('@James');
  check('Mention in the menu inserts the mention', text.value === '@James ', JSON.stringify(text.value));
  text.value = 'hello';
  mentionVia('@Matt');
  check('mention is added after existing text', text.value === 'hello @Matt ', JSON.stringify(text.value));
  s.sent.length = 0;
  [...d.querySelectorAll('.who')].find(b => b.textContent === '@Matt').click();
  check('tapping a name opens that person\'s profile', JSON.stringify(s.sent[s.sent.length - 1]) === JSON.stringify({ t: 'profile', h: 'Matt' }) && !$('userProfile').hidden, JSON.stringify(s.sent));
  $('back').click();
  text.value = '';
  [...d.querySelectorAll('.who')].find(b => b.textContent === '@Eldorado').click();
  check('tapping my own name opens my own profile and does not mention me', text.value === '' && !$('userProfile').hidden);
  $('back').click();
  check('and back returns to the room', !$('chat').hidden && $('userProfile').hidden);

  // points panel and progress
  check('points line shows tier, points and streak', $('myPts').textContent === 'Newbie · 19 / 300 pts · day 1', $('myPts').textContent);
  $('myPts').click();
  check('tapping points opens the rules', !$('rules').hidden && $('myPts').getAttribute('aria-expanded') === 'true');
  check('current tier is highlighted in the rules', $('tiers').children[0].classList.contains('now') && !$('tiers').children[1].classList.contains('now'));
  $('myPts').click();
  check('tapping again closes the rules', $('rules').hidden);
  s.recv({ t: 'me', p: 310, r: 1, x: 2000, s: 2, g: true });
  check('me frame updates tier line', $('myPts').textContent === 'Regular · 310 / 2000 pts · day 2' && $('myBadge').textContent === 'REG');
  check('progress bar follows points', $('fill').style.width === Math.round((310 - 300) / (2000 - 300) * 100) + '%', $('fill').style.width);
  check('password button hides once protected', $('secureBtn').parentElement.hidden);
  check('highlight moves to the new tier', $('tiers').children[1].classList.contains('now'));
  s.recv({ t: 'me', p: 45000, r: 4, x: 0, s: 9, g: true });
  check('top tier shows a full bar and no target', $('fill').style.width === '100%' && $('myPts').textContent === 'Legend · 45000 pts · day 9', $('myPts').textContent);

  // live mention while the tab is hidden
  Object.defineProperty(d, 'visibilityState', { configurable: true, get: () => 'hidden' });
  s.recv({ t: 'm', i: 7, h: 'James', b: 'ping @Eldorado', ts: 7, r: 0 });
  check('hidden tab gets an unread count in the title', d.title === '(1) STREETSYNC', d.title);
  s.recv({ t: 'm', i: 8, h: 'James', b: 'nothing for you', ts: 8, r: 0 });
  check('only mentions count', d.title === '(1) STREETSYNC');
  Object.defineProperty(d, 'visibilityState', { configurable: true, get: () => 'visible' });
  d.dispatchEvent(new w.Event('visibilitychange'));
  check('title resets when you come back', d.title === 'STREETSYNC');

  // announcements and errors
  s.recv({ t: 'up', h: 'Matt', r: 1 });
  check('tier up line shown', [...d.querySelectorAll('#log .m.up')].pop().textContent === '@Matt reached Regular');
  s.recv({ t: 'err', c: 'slow_down' });
  check('error shown in the log', [...d.querySelectorAll('#log .m.sys')].pop().textContent === 'Slow down a little.');
  s.recv({ t: 'n', c: 3 });
  check('online count', $('count').textContent === '3');
  s.recv({ t: 'm', i: 7, h: 'Matt', b: 'duplicate id', ts: 9, r: 0 });
  check('old ids are ignored', ![...d.querySelectorAll('#log .m')].some(m => m.textContent.includes('duplicate id')));

  // reconnect with the saved token
  s.drop();
  check('drop shows an offline status', $('status').textContent.startsWith('Offline. Retrying'), $('status').textContent);
  await new Promise(r => setTimeout(r, 700));
  const s2 = sockets[1];
  check('a new socket is opened by itself', !!s2 && s2 !== s);
  s2.open();
  check('resume sends the saved token and last id', JSON.stringify(s2.sent[0]) === JSON.stringify({ t: 'hi', k: 'tok', r: 'lobby', l: 8 }), JSON.stringify(s2.sent[0]));
  s.recv({ t: 'm', i: 99, h: 'Ghost', b: 'from the dead socket', ts: 1, r: 0 });
  check('frames from the old socket are ignored', ![...d.querySelectorAll('#log .m')].some(m => m.textContent.includes('dead socket')));
  s2.recv(ok('Eldorado', { p: 400, r: 1 }));
  check('status clears after reconnect', $('status').textContent === '');
  s.drop();
  check('the old socket closing later does not start another retry', sockets.length === 2);

  // expired session
  s2.recv({ t: 'err', c: 'session_expired' });
  check('expired session shows the join form again', !$('join').hidden && $('chat').hidden && $('joinErr').textContent.includes('session ended'));
  check('expired token is removed', w.localStorage.getItem('lobby.k') === null);

  // login mode
  $('toggle').click();
  check('login mode shows the password field', !$('pass').hidden && $('enter').textContent === 'Log in');
  $('handle').value = 'Ada'; $('pass').value = 'secret pass';
  $('join').dispatchEvent(new w.Event('submit', { cancelable: true }));
  const s3 = sockets[sockets.length - 1]; s3.open();
  check('login sends handle and password', JSON.stringify(s3.sent[0]) === JSON.stringify({ t: 'login', h: 'Ada', p: 'secret pass' }), JSON.stringify(s3.sent[0]));
  s3.recv({ t: 'err', c: 'bad_login' }); s3.drop();
  check('bad login shows a message and keeps the form', $('joinErr').textContent === 'Wrong handle or password.' && !$('join').hidden);

  // saved token on page load goes straight to chat
  const b = boot({ token: 'saved' });
  check('a saved login opens the app right away, on the hub', !b.d.getElementById('app').hidden && b.d.getElementById('join').hidden && b.d.getElementById('chat').hidden);
  b.sockets[0].open();
  check('and resumes with the saved token', JSON.stringify(b.sockets[0].sent[0]) === JSON.stringify({ t: 'hi', k: 'saved' }));

  // password form
  b.sockets[0].recv(ok('Ada'));
  b.d.getElementById('secureBtn').click();
  check('secure form opens', !b.d.getElementById('secure').hidden);
  b.d.getElementById('newpass').value = 'short';
  b.d.getElementById('secure').dispatchEvent(new b.w.Event('submit', { cancelable: true }));
  check('short password is refused in the page', b.d.getElementById('secureErr').textContent.includes('8'));
  b.d.getElementById('newpass').value = 'long enough pw';
  b.d.getElementById('secure').dispatchEvent(new b.w.Event('submit', { cancelable: true }));
  check('password is sent', JSON.stringify(b.sockets[0].sent.pop()) === JSON.stringify({ t: 'pw', p: 'long enough pw' }));
  b.sockets[0].recv({ t: 'pwok' });
  check('form closes on success', b.d.getElementById('secure').hidden);

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
})();
