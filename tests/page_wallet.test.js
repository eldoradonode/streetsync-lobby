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
  let t = boot();
  t.s.open(); t.s.recv(okf({ c: 20 })); t.s.recv({ t: 'rooms', r: ROOMS });
  check('the header shows the balance on the hub', !t.$('credChip').hidden && t.$('credCount').textContent === '20', t.$('credCount').textContent);
  check('the online counter is not shown on the hub', t.$('online').hidden);

  t.$('tabWallet').click();
  check('the Wallet tab opens the wallet page and keeps the balance chip', !t.$('walletPage').hidden && t.$('hub').hidden && !t.$('credChip').hidden && t.$('title').textContent === 'Wallet');
  check('opening the wallet asks for a fresh balance', t.s.sent.some(x => x.t === 'wallet'), JSON.stringify(t.s.sent));
  check('the wallet card shows the balance', t.$('walletBal').textContent === '20');
  check('an empty history says so', t.$('walletList').textContent.includes('Nothing here yet'));

  t.s.recv({ t: 'wallet', c: 27, l: [
    { i: 3, k: 'streak', a: 5, b: 27, n: '3 day streak', ts: Date.now() },
    { i: 2, k: 'daily', a: 2, b: 22, n: 'Daily chat bonus', ts: Date.now() },
    { i: 1, k: 'welcome', a: 20, b: 20, n: 'Welcome gift', ts: Date.now() },
    { i: 4, k: 'spend', a: -3, b: 24, n: '<img src=x onerror="window.pwned=1">', ts: Date.now() },
  ] });
  const rows = [...t.$('walletList').querySelectorAll('li')];
  check('the wallet frame updates the balance everywhere', t.$('walletBal').textContent === '27' && t.$('credCount').textContent === '27');
  check('one line per ledger entry, in the order sent', rows.length === 4 && rows[0].textContent.includes('3 day streak') && rows[2].textContent.includes('Welcome gift'), rows.map(r => r.textContent));
  check('gains show a plus and spends a minus', rows[0].querySelector('b').textContent === '+5' && rows[0].querySelector('b').classList.contains('plus') && rows[3].querySelector('b').textContent === '-3' && rows[3].querySelector('b').classList.contains('minus'));
  check('a hostile note is shown as text, never run', !t.w.pwned && !rows[3].querySelector('img'));

  // balance pushes arrive on the me frame
  t.s.recv({ t: 'me', p: 25, r: 0, x: 300, s: 1, g: false, c: 29 });
  check('a me frame moves the balance', t.$('walletBal').textContent === '29' && t.$('credCount').textContent === '29');
  t.s.recv({ t: 'me', p: 26, r: 0, x: 300, s: 1, g: false });
  check('a me frame without a balance counts as zero', t.$('walletBal').textContent === '0');

  // inside a room the chip goes away and the online count returns
  t.s.recv({ t: 'me', p: 26, r: 0, x: 300, s: 1, g: false, c: 29 });
  t.$('tabHub').click();
  [...t.d.querySelectorAll('#roomList .room')][0].click();
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' }); t.s.recv({ t: 'hist', m: [] });
  check('the chip is hidden in a room and the online count is shown', t.$('credChip').hidden && !t.$('online').hidden);
  t.s.recv({ t: 'me', p: 27, r: 0, x: 300, s: 1, g: false, c: 31 });
  t.$('back').click(); t.s.recv({ t: 'left' }); t.s.recv({ t: 'rooms', r: ROOMS });
  check('earning inside a room shows on the hub after', !t.$('credChip').hidden && t.$('credCount').textContent === '31');

  // big numbers and the admin error text
  t.s.recv({ t: 'me', p: 27, r: 0, x: 300, s: 1, g: false, c: 1234567 });
  check('large balances get separators', /1.234.567/.test(t.$('credCount').textContent), t.$('credCount').textContent);

  // log out clears the wallet
  t.$('tabProfile').click();
  t.$('logout').click(); t.$('logoutAnyway') && t.$('logoutAnyway').click();
  check('logging out empties the wallet', t.$('walletBal').textContent === '0' && t.$('walletList').textContent.includes('Nothing here yet'));

  // ---- the admin form
  t = boot();
  t.s.open(); t.s.recv(okf({ c: 5 })); t.s.recv({ t: 'rooms', r: ROOMS });
  check('members have no admin console link', !t.$('tabAdmin') && !t.$('staffNav'));
  t.s.recv({ t: 'me', p: 0, r: 0, x: 0, s: 0, g: true, o: 'admin', c: 5 });
  check('the admin gets the admin console link', !!t.$('tabAdmin'));
  t.$('tabAdmin').click();
  t.$('sec-users').click();
  t.s.recv({ t: 'ausers', q: '', u: [{ h: 'bob', o: 'user', p: 10, r: 0, j: 1, g: true, on: false, c: 7, ls: 1 }] });
  const give = () => t.d.querySelector('#staffUsers [data-handle="bob"] form');
  const amount = v => { const i = give().querySelector('input'); i.value = v; i.dispatchEvent(new t.w.Event('input', { bubbles: true })); };
  amount('50'); give().dispatchEvent(new t.w.Event('submit', { cancelable: true }));
  check('a grant is sent with the handle and amount', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'grant', h: 'bob', m: 50 }));
  t.s.recv({ t: 'modok', a: 'grant', h: 'bob', m: 50 });
  check('the admin gets a confirmation', t.$('status').textContent === 'Sent 50 credits to @bob.', t.$('status').textContent);
  const before = t.s.sent.length;
  amount('0'); give().dispatchEvent(new t.w.Event('submit', { cancelable: true }));
  check('a zero amount is refused in the page', !t.s.sent.slice(before).some(x => x.a === 'grant') && t.$('status').textContent.includes('1 to 100000'), t.$('status').textContent);
  amount('2.9'); give().dispatchEvent(new t.w.Event('submit', { cancelable: true }));
  check('a decimal amount is rounded down', JSON.stringify(t.s.sent.filter(x => x.a === 'grant').pop()) === JSON.stringify({ t: 'mod', a: 'grant', h: 'bob', m: 2 }));

  // ---- wallet page: filters and totals
  t = boot();
  t.s.open(); t.s.recv(okf({ c: 20 })); t.s.recv({ t: 'rooms', r: ROOMS });
  t.$('tabWallet').click();
  t.s.recv({ t: 'wallet', c: 20, l: [{ i: 3, k: 'daily', a: 2, n: '', ts: 3 }, { i: 2, k: 'tip_sent', a: -5, n: 'Burger to @Tunde', ts: 2 }, { i: 1, k: 'welcome', a: 20, n: '', ts: 1 }] });
  const wrows = () => [...t.$('walletList').querySelectorAll('li')];
  check('the wallet lists every movement', wrows().length === 3);
  check('earned and spent totals are summed from the list', t.$('wallet').textContent.includes('+22') && t.$('wallet').textContent.includes('-5'), t.$('wallet').textContent);
  const chipBtn = label => [...t.$('walletPage').querySelectorAll('.chipbtn')].find(b => b.textContent === label);
  chipBtn('Earned').click();
  check('Earned shows only credits coming in', wrows().length === 2 && wrows().every(r => r.querySelector('.plus')));
  chipBtn('Spent').click();
  check('Spent shows only credits going out', wrows().length === 1 && wrows()[0].textContent.includes('Burger'));
  chipBtn('All').click();
  check('All shows everything again', wrows().length === 3);

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
})();
