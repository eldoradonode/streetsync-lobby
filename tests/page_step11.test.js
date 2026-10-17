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

const field = (t, id, v) => { t.$(id).value = v; };

function hub(opts = {}) {
  const t = boot();
  t.s.open(); t.s.recv(okf({ c: 100, h: 'Mike' }));
  t.s.recv({ t: 'rooms', r: [
    ...ROOMS,
    { s: 'free-aaaa', n: 'Free Chat', c: 'Community', g: ['Gist'], u: 3, h: 2, k: 'public', o: 'Ann' },
    { s: 'paid-bbbb', n: 'Paid Lounge', c: 'Community', g: [], u: 1, h: 1, k: 'public', o: 'Bob', f: 10 },
    { s: 'mine-cccc', n: 'My Den', c: 'Community', g: [], u: 0, h: 0, k: 'private', o: 'Mike', w: true, y: true },
    { s: 'paid-dddd', n: 'Joined Lounge', c: 'Community', g: [], u: 2, h: 1, k: 'public', o: 'Cy', f: 20, y: true },
  ] });
  return t;
}
const tile = (t, slug) => t.d.querySelector('.room[data-slug="' + slug + '"]');

(async () => {
  const flush = t => t.w.__lobbyFlush && t.w.__lobbyFlush();
  let t = hub();
  const dirRows = () => [...t.d.querySelectorAll('#rdList .room')].map(r => r.dataset.slug);
  const chip = label => [...t.d.querySelectorAll('#rdChips .chipbtn')].find(b => b.textContent.trim() === label);

  // ---- the Rooms tab
  check('there is a Rooms tab beside Home, Messages, Wallet and Profile', !!t.$('tabRooms') && t.d.querySelectorAll('#tabs button').length === 5);
  check('the directory starts hidden', t.$('roomsDir').hidden);
  t.$('tabRooms').click();
  check('the Rooms tab opens the directory and hides the hub', !t.$('roomsDir').hidden && t.$('hub').hidden && t.$('title').textContent === 'Rooms' && t.$('tabRooms').classList.contains('active'));
  check('rooms in Trending are not listed a second time', dirRows().length === 4 && !dirRows().some(x => ['free-aaaa', 'lobby', 'paid-dddd'].includes(x)), dirRows().join());
  check('Trending plus the list holds every room', dirRows().length + t.d.querySelectorAll('#rdTrending .room').length === 7);
  check('trending shows the busiest rooms first', [...t.d.querySelectorAll('#rdTrending .room')].map(r => r.dataset.slug).join() === 'free-aaaa,lobby,paid-dddd', [...t.d.querySelectorAll('#rdTrending .room')].map(r => r.dataset.slug).join());
  check('filter chips come from the categories', ['All', 'Official', 'Community', 'General', 'Members', 'Test'].every(c => !!chip(c)));
  chip('Community').click();
  check('Community shows only community rooms and hides trending', dirRows().length === 4 && !t.$('rdTrending'), dirRows().join());
  chip('Official').click();
  check('Official shows only official rooms', dirRows().length === 3 && !dirRows().includes('free-aaaa'));
  chip('Members').click();
  check('a category shows only its rooms', dirRows().join() === 'elite-lounge');
  chip('All').click();
  t.$('rdSearch').value = 'gist'; t.$('rdSearch').dispatchEvent(new t.w.Event('input', { bubbles: true })); flush(t);
  check('typing filters by name or tag at once', dirRows().join() === 'free-aaaa', dirRows().join());
  t.$('rdSearch').value = 'zzzz'; t.$('rdSearch').dispatchEvent(new t.w.Event('input', { bubbles: true })); flush(t);
  check('no match shows a friendly message', dirRows().length === 0 && !!t.d.querySelector('#rdList .empty'));
  t.$('rdSearch').value = 'night';
  t.$('rdSearch').dispatchEvent(new t.w.Event('input', { bubbles: true })); flush(t);
  t.d.querySelector('#roomsDir form').dispatchEvent(new t.w.Event('submit', { cancelable: true })); flush(t);
  check('Search also asks the server', JSON.stringify(last(t)).includes('night'), JSON.stringify(last(t)));
  t.s.recv({ t: 'found', r: [{ s: 'night-zzzz', n: 'Night Owls', c: 'Community', g: ['Night'], u: 0, h: 0, k: 'public', o: 'Zed' }] });
  check('server results are added below', dirRows().includes('night-zzzz'), dirRows().join());
  t.$('rdSearch').value = ''; t.$('rdSearch').dispatchEvent(new t.w.Event('input', { bubbles: true })); flush(t);
  t.d.querySelector('#roomsDir .room[data-slug="free-aaaa"]').click();
  check('a tile in the directory joins like on the hub', JSON.stringify(last(t)).includes('free-aaaa'), JSON.stringify(last(t)));

  // ---- create dialog
  t = hub();
  t.$('tabRooms').click();
  check('the create dialog is closed at first', !t.$('createRoom'));
  t.$('rdCreate').click();
  check('Create a room opens a dialog', !!t.$('createRoom') && !!t.d.querySelector('.modal'));
  const cards = [...t.d.querySelectorAll('#createRoom .typecard')];
  check('there are four room type cards, Public selected', cards.length === 4 && cards[0].classList.contains('sel') && cards.map(c => c.dataset.type).join() === 'public,private,password,paid');
  check('fee and password fields are hidden until needed', t.$('crFee').parentElement.hidden && t.$('crPw').parentElement.hidden);
  cards[3].click();
  check('the Paid card shows the fee field with a starting fee', !t.$('crFee').parentElement.hidden && t.$('crFee').value === '5' && t.$('crPw').parentElement.hidden);
  cards[2].click();
  check('the Password card shows the password field and clears the fee', !t.$('crPw').parentElement.hidden && t.$('crFee').value === '0' && t.$('crFee').parentElement.hidden);
  cards[1].click();
  check('the Private card makes the room private', t.$('crKind').value === 'private');
  field(t, 'crName', 'Quiet Corner'); submitForm(t, 'createRoom');
  check('and sends a private room with no fee or password', JSON.stringify(last(t)) === JSON.stringify({ t: 'croom', n: 'Quiet Corner', k: 'private', g: '', f: 0, c: 'General' }), JSON.stringify(last(t)));
  check('the dialog closes', !t.$('createRoom'));
  t.$('rdCreate').click();
  t.d.querySelector('.modal').dispatchEvent(new t.w.MouseEvent('click', { bubbles: true })); flush(t);
  check('tapping outside the dialog closes it', !t.$('createRoom'));
  t.$('tabHub').click(); t.$('toolCreate').click();
  check('the hub Create button opens the same dialog', !!t.$('createRoom'));
  t.$('toolCreate').click();
  check('and closes it again', !t.$('createRoom'));

  // ---- legal pages
  t = boot({ token: false });
  check('the sign up page links to Terms and Privacy', !!t.$('legalTerms') && !!t.$('legalPrivacy'));
  check('no legal page at first', !t.$('legalPage'));
  t.$('legalTerms').click();
  check('Terms opens', !!t.$('legalPage') && /Terms of use/.test(t.$('legalPage').textContent));
  check('the terms say credits are not money and name the minimum age', /not money/.test(t.$('legalPage').textContent) && /16 or older/.test(t.$('legalPage').textContent));
  [...t.$('legalPage').querySelectorAll('[role=tab]')].find(b => b.textContent === 'Privacy').click();
  check('the Privacy tab shows the privacy page', /What we keep/.test(t.$('legalPage').textContent) && /7 days/.test(t.$('legalPage').textContent));
  t.$('legalClose').click();
  check('Close hides it', !t.$('legalPage'));
  t = hub(); t.$('tabProfile').click();
  check('Profile has Terms and Privacy links', !!t.$('profTerms') && !!t.$('profPrivacy'));
  t.$('profPrivacy').click();
  check('and they open the page on top of the app', !!t.$('legalPage') && /Privacy/.test(t.$('legalPage').textContent));

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
})().catch(e => { console.log('CRASH', e); process.exit(1); });
