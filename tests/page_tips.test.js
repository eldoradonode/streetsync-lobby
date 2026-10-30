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



const btn = (root, label) => [...root.querySelectorAll('button')].find(b => b.textContent.trim() === label);
const last = t => t.s.sent[t.s.sent.length - 1];
const sentOf = (t, type) => t.s.sent.filter(x => x.t === type);
const submitForm = (t, id) => t.$(id).dispatchEvent(new t.w.Event('submit', { cancelable: true }));

const KEY_RE = /^[A-Za-z0-9_-]{8,64}$/;

function room(credits = 30) {
  const t = boot();
  t.s.open(); t.s.recv(okf({ c: credits, h: 'Mike' })); t.s.recv({ t: 'rooms', r: ROOMS });
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' });
  t.s.recv({ t: 'hist', m: [
    { i: 1, h: 'Matt', b: 'great game last night', ts: 1, r: 0 },
    { i: 2, h: 'Mike', b: 'my own words', ts: 2, r: 0 },
    { i: 3, h: 'Ann', b: 'Burger', ts: 3, r: 0, tt: 'Matt', ta: 5 },
    { i: 4, h: 'Zed', b: 'Pizza', ts: 4, r: 0, tt: 'Mike', ta: 10 },
    { i: 5, h: 'Eve', b: 'staff here', ts: 5, r: 0, o: 'mod' },
  ] });
  return t;
}
const msg = (t, id) => t.d.querySelector('#log .m[data-id="' + id + '"]');
const more = el => el.querySelector('button[aria-label^="More actions"]');
const tipsSent = t => t.s.sent.filter(x => x.t === 'tip');

(async () => {
  let t = room();

  // ---- the cards in the feed
  const c1 = msg(t, 3), c2 = msg(t, 4);
  check('a tip is shown as a card with the treat, both people and the amount', c1.classList.contains('tipcard') && c1.textContent.includes('@Ann') && c1.textContent.includes('@Matt') && c1.textContent.includes('Burger') && c1.textContent.includes('5 credits'), c1.textContent);
  check('the card carries the treat icon', c1.querySelector('.te').textContent === '\u{1F354}' && c2.querySelector('.te').textContent === '\u{1F355}');
  check('a tip to me is highlighted more', c2.classList.contains('tome') && !c1.classList.contains('tome'));
  check('a card has no menu, reply or avatar', !more(c1) && !btn(c1, 'Reply') && !c1.parentElement.querySelector('.av'));
  check('a card is not shown as a normal message', !c1.querySelector('.head'));
  t.s.recv({ t: 'm', i: 6, h: 'Ann', b: 'Suya', ts: 6, r: 0, tt: 'Matt', ta: 1 });
  check('a live tip appears as a card, with a singular credit', msg(t, 6).classList.contains('tipcard') && msg(t, 6).textContent.includes('1 credit') && !msg(t, 6).textContent.includes('1 credits'), msg(t, 6).textContent);
  t.s.recv({ t: 'm', i: 7, h: 'Ann', b: '<img src=x onerror="window.pwned=1">', ts: 7, r: 0, tt: 'Matt', ta: 5 });
  check('a hostile treat name is shown as text, never run', !t.w.pwned && !msg(t, 7).querySelector('img'));
  t.s.recv({ t: 'm', i: 8, h: 'Ann', b: 'Mystery', ts: 8, r: 0, tt: 'Matt', ta: 7 });
  check('an amount the page does not know still shows as a card', msg(t, 8).classList.contains('tipcard') && msg(t, 8).textContent.includes('7 credits'));

  // ---- who can be tipped
  more(msg(t, 2)).click();
  check('there is no tip option on my own message', !btn(msg(t, 2), 'Send a tip'));
  more(msg(t, 5)).click();
  check('staff messages cannot be tipped by an ordinary user, and offer a staff report instead', !btn(msg(t, 5), 'Send a tip') && !btn(msg(t, 5), 'Message') && !btn(msg(t, 5), 'Add friend') && !btn(msg(t, 5), 'Report') && !!btn(msg(t, 5), 'Report staff misconduct'));
  more(msg(t, 1)).click();
  btn(msg(t, 1), 'Send a tip').click();

  // ---- the sheet
  check('the sheet opens for that person', !!t.$('tipSheet') && t.$('tipSheet').textContent.includes('Tip @Matt'));
  check('it shows the balance', t.$('tipBal').textContent.includes('30 credits'), t.$('tipBal').textContent);
  const opts = [...t.$('tipChoices').querySelectorAll('.tipopt')];
  check('there are five fixed treats with names and prices', JSON.stringify(opts.map(o => [o.querySelector('b').textContent, o.querySelector('small').textContent])) === JSON.stringify([['Suya', '1 credit'], ['Burger', '5 credits'], ['Pizza', '10 credits'], ['Cake', '25 credits'], ['Feast', '50 credits']]), opts.map(o => o.textContent));
  check('nothing is chosen at first and Send is off', t.$('tipSend').disabled && t.$('tipSend').textContent.trim() === 'Pick a treat' && opts.every(o => o.getAttribute('aria-checked') === 'false'));
  t.$('tipSend').click();
  check('Send does nothing without a choice', tipsSent(t).length === 0);
  opts[2].click();
  check('choosing a treat selects it and names it on the button', opts[2].classList.contains('sel') && opts[2].getAttribute('aria-checked') === 'true' && t.$('tipSend').textContent.trim() === 'Send Pizza (10 credits)' && !t.$('tipSend').disabled, t.$('tipSend').textContent);
  opts[1].click();
  check('choosing another moves the selection', !opts[2].classList.contains('sel') && opts[1].classList.contains('sel') && t.$('tipSend').textContent.trim() === 'Send Burger (5 credits)');
  opts[2].click();

  // ---- sending, double taps and retries
  t.$('tipSend').click(); t.$('tipSend').click(); t.$('tipSend').click();
  check('three quick taps send one request', tipsSent(t).length === 1, tipsSent(t).length);
  const f = tipsSent(t)[0];
  check('the request names the message, the amount and a key', f.i === 1 && f.amt === 10 && KEY_RE.test(f.c), JSON.stringify(f));
  check('while waiting the button is off and says so', t.$('tipSend').disabled && t.$('tipSend').textContent.trim() === 'Sending' && [...t.$('tipChoices').querySelectorAll('.tipopt')].every(o => o.disabled));
  t.s.recv({ t: 'err', c: 'no_credits' });
  check('a refusal is shown in the sheet and the sheet stays open', !!t.$('tipSheet') && t.$('tipErr').textContent.includes('not have enough credits') && !t.$('tipSend').disabled, t.$('tipErr') && t.$('tipErr').textContent);
  t.$('tipSend').click();
  check('trying again sends the same key', tipsSent(t).length === 2 && tipsSent(t)[1].c === f.c && tipsSent(t)[1].amt === 10, JSON.stringify(tipsSent(t)[1]));
  t.s.recv({ t: 'err', c: 'server_busy' });
  check('a server problem says retrying is safe', t.$('tipErr').textContent.includes('not be charged twice'), t.$('tipErr').textContent);
  t.s.recv({ t: 'me', p: 5, r: 0, x: 300, s: 1, g: false, c: 20 });
  t.s.recv({ t: 'tipok', a: 10, n: 'Pizza', h: 'Matt' });
  check('success closes the sheet and says what was sent', !t.$('tipSheet') && t.$('status').textContent === 'You sent @Matt a Pizza (10 credits).', t.$('status').textContent);
  more(msg(t, 1)).click(); btn(msg(t, 1), 'Send a tip').click();
  t.$('tipChoices').querySelectorAll('.tipopt')[2].click(); t.$('tipSend').click();
  check('a new tip gets a new key', tipsSent(t)[2].c !== f.c && KEY_RE.test(tipsSent(t)[2].c));
  t.s.recv({ t: 'tipok', a: 10, n: 'Pizza', h: 'Matt', dup: true });
  check('a repeat that the server recognised is explained', t.$('status').textContent === 'That tip was already sent.', t.$('status').textContent);

  // ---- every refusal has a clear message in the sheet
  const sheetWith = code => {
    more(msg(t, 1)).click(); btn(msg(t, 1), 'Send a tip').click();
    t.$('tipChoices').querySelectorAll('.tipopt')[1].click(); t.$('tipSend').click();
    t.s.recv({ t: 'err', c: code });
    const text = t.$('tipErr') && t.$('tipErr').textContent;
    t.$('tipCancel').click();
    return text;
  };
  const expect = { self_tip: 'cannot tip yourself', blocked: 'cannot tip this person', msg_gone: 'message is gone', slow_down: 'Slow down', forbidden: 'cannot be tipped', no_room: 'Join a room', bad_amount: 'could not be sent' };
  for (const [code, part] of Object.entries(expect)) {
    const text = sheetWith(code);
    check('the ' + code + ' refusal reads clearly', !!text && text.includes(part), text);
  }

  // ---- not enough credits is caught before anything is sent
  t = room(3);
  more(msg(t, 1)).click(); btn(msg(t, 1), 'Send a tip').click();
  t.$('tipChoices').querySelectorAll('.tipopt')[1].click(); t.$('tipSend').click();
  check('a tip above the balance is stopped in the page', tipsSent(t).length === 0 && t.$('tipErr').textContent === 'You have 3 credits. This tip needs 5.', t.$('tipErr') && t.$('tipErr').textContent);
  t.$('tipChoices').querySelectorAll('.tipopt')[0].click();
  check('picking something cheaper clears the message', !t.$('tipErr'));
  t.$('tipSend').click();
  check('and sends it', tipsSent(t).length === 1 && tipsSent(t)[0].amt === 1);

  // ---- closing the sheet
  t = room();
  more(msg(t, 1)).click(); btn(msg(t, 1), 'Send a tip').click();
  t.$('tipCancel').click();
  check('Cancel closes the sheet', !t.$('tipSheet'));
  more(msg(t, 1)).click(); btn(msg(t, 1), 'Send a tip').click();
  t.$('tipScrim').click();
  check('tapping outside closes it', !t.$('tipSheet'));
  more(msg(t, 1)).click(); btn(msg(t, 1), 'Send a tip').click();
  t.$('back').click(); t.s.recv({ t: 'left' });
  check('leaving the room closes it', !t.$('tipSheet'));

  // ---- receiving
  t.s.recv({ t: 'tipgot', f: 'Ann', n: 'Cake', a: 25 });
  check('the person tipped is told, whatever screen they are on', t.$('status').textContent === '@Ann sent you a Cake (25 credits).', t.$('status').textContent);

  // ---- a dropped connection while sending lets the person retry safely
  t = room();
  more(msg(t, 1)).click(); btn(msg(t, 1), 'Send a tip').click();
  t.$('tipChoices').querySelectorAll('.tipopt')[3].click(); t.$('tipSend').click();
  const key1 = tipsSent(t)[0].c;
  t.s.drop();
  await new Promise(r => setTimeout(r, 700));
  const s2 = t.sockets[1]; s2.open(); s2.recv(okf({ c: 30, h: 'Mike' })); s2.recv({ t: 'rooms', r: ROOMS });
  check('after reconnecting the sheet can be used again', !!t.$('tipSheet') && !t.$('tipSend').disabled);
  t.$('tipSend').click();
  check('and the retry carries the same key, so it can not pay twice', s2.sent.filter(x => x.t === 'tip').length === 1 && s2.sent.filter(x => x.t === 'tip')[0].c === key1);

  // ---- the wallet tells tips and room fees apart
  t = boot();
  t.s.open(); t.s.recv(okf({ c: 20, h: 'Mike' })); t.s.recv({ t: 'rooms', r: ROOMS });
  t.$('tabProfile').click();
  const now = Date.now();
  t.s.recv({ t: 'wallet', c: 20, l: [
    { i: 6, k: 'tip_received', a: 10, b: 20, n: 'Pizza from @Ann', ts: now },
    { i: 5, k: 'tip_sent', a: -5, b: 10, n: 'Burger for @Matt', ts: now },
    { i: 4, k: 'room_entry', a: -10, b: 15, n: 'Entered Paid Lounge', ts: now },
    { i: 3, k: 'room_income', a: 8, b: 25, n: 'Entry to My Den', ts: now },
    { i: 2, k: 'room_create', a: -25, b: 17, n: 'Created the room My Den', ts: now },
    { i: 1, k: 'welcome', a: 20, b: 42, n: 'Welcome gift', ts: now },
  ] });
  const rows = [...t.$('walletList').querySelectorAll('li')];
  const kindOf = i => rows[i].querySelector('.kind');
  check('tips are labelled Tip received and Tip sent', kindOf(0).textContent === 'Tip received' && kindOf(1).textContent === 'Tip sent');
  check('room fees are labelled differently', kindOf(2).textContent === 'Room entry' && kindOf(3).textContent === 'Room income' && kindOf(4).textContent === 'Room created');
  check('tips and room fees have different styles', kindOf(0).classList.contains('k-tip_received') && kindOf(2).classList.contains('k-room_entry') && !kindOf(0).classList.contains('k-room_entry'));
  check('the note still says who and what', rows[0].textContent.includes('Pizza from @Ann') && rows[1].textContent.includes('Burger for @Matt'));
  check('amounts keep their signs', rows[0].querySelector('b').textContent === '+10' && rows[1].querySelector('b').textContent === '-5');
  t.s.recv({ t: 'wallet', c: 1, l: [{ i: 9, k: 'mystery_kind', a: 1, b: 1, n: '', ts: now }] });
  check('an unknown kind still shows something', t.$('walletList').querySelector('.kind').textContent === 'mystery_kind');

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
})();
