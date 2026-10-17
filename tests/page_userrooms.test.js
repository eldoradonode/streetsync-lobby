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
  let t = hub();
  // ---- the hub sections
  check('official rooms are listed apart from community rooms', t.$('roomList').querySelectorAll('.room').length === ROOMS.length && t.$('communityList').querySelectorAll('.room').length === 4);
  check('a community tile shows its host', tile(t, 'free-aaaa').textContent.includes('@Ann') && tile(t, 'free-aaaa').textContent.includes('Gist'));
  check('my own room says Yours and shows Private and Password', tile(t, 'mine-cccc').textContent.includes('Yours') && tile(t, 'mine-cccc').textContent.includes('Private') && tile(t, 'mine-cccc').textContent.includes('Password'));
  check('a fee shows on the tile', tile(t, 'paid-bbbb').textContent.includes('10 credits'));
  check('an official tile has no host line', !tile(t, ROOMS[0].s).textContent.includes('@'));

  // ---- entering: free, paid, and already a member
  const sent0 = t.s.sent.length;
  tile(t, 'free-aaaa').click();
  check('a free public room is entered with one tap', JSON.stringify(last(t)) === JSON.stringify({ t: 'join', r: 'free-aaaa', l: 0 }), JSON.stringify(last(t)));
  tile(t, 'joined-dddd') && 0;
  tile(t, 'paid-dddd').click();
  check('a room I already paid for is entered with one tap', last(t).r === 'paid-dddd' && last(t).f === undefined, JSON.stringify(last(t)));
  tile(t, 'mine-cccc').click();
  check('my own password room is entered with one tap', last(t).r === 'mine-cccc');
  const n1 = t.s.sent.length;
  tile(t, 'paid-bbbb').click();
  check('a paid room asks first and sends nothing', !t.$('rpPanel') === false && t.s.sent.length === n1, t.s.sent.length - n1);
  check('the panel shows the room, host, price and balance', t.$('rpPanel').textContent.includes('Paid Lounge') && t.$('rpPanel').textContent.includes('@Bob') && t.$('rpFee').textContent.includes('10 credits') && t.$('rpFee').textContent.includes('100'), t.$('rpPanel').textContent);
  check('there is no password box for a room without one', !t.$('rpPw'));
  check('the button names the price', t.$('rpGo').textContent === 'Pay 10 and enter');
  t.$('rpGo').click();
  check('paying sends the price that was shown', JSON.stringify(last(t)) === JSON.stringify({ t: 'join', r: 'paid-bbbb', l: 0, f: 10 }), JSON.stringify(last(t)));
  t.s.recv({ t: 'err', c: 'fee_changed', m: 15 });
  check('a changed price is shown in the panel and the new price is ready', t.$('rpErr').textContent.includes('15 credits') && t.$('rpGo').textContent === 'Pay 15 and enter', t.$('rpErr') && t.$('rpErr').textContent);
  t.s.recv({ t: 'err', c: 'no_credits', m: 15 });
  check('not enough credits is shown in the panel', t.$('rpErr').textContent.includes('15 credits') && !t.$('status').textContent.includes('15'), t.$('rpErr').textContent);
  t.$('rpGo').click();
  check('the second tap accepts the new price', last(t).f === 15);
  t.s.recv({ t: 'err', c: 'room_banned', m: 20 });
  check('a ban is explained in the panel', t.$('rpErr').textContent.includes('20 minutes'), t.$('rpErr').textContent);
  t.$('rpNo').click();
  check('Cancel closes the panel', !t.$('rpPanel'));
  tile(t, 'paid-bbbb').click();
  t.s.recv({ t: 'joined', r: 'paid-bbbb', n: 'Paid Lounge', k: 'public', o: 'Bob', f: 10 });
  check('getting in closes the panel and opens the room', !t.$('rpPanel') && !t.$('chat').hidden && t.$('hub').hidden);
  check('a visitor gets no room tools', !t.$('hostTools'));
  t.$('back').click(); t.s.recv({ t: 'left' });

  // ---- using an invite code
  t.$('toolCode').click();
  field(t, 'codeInput', 'ABC'); submitForm(t, 'codeForm');
  check('a short code is refused in the page', t.$('status').textContent.includes('8 letters') && last(t).t !== 'rinfo', t.$('status').textContent);
  field(t, 'codeInput', 'abcd2345'); submitForm(t, 'codeForm');
  check('a code is looked up first', JSON.stringify(last(t)) === JSON.stringify({ t: 'rinfo', c: 'abcd2345' }), JSON.stringify(last(t)));
  t.s.recv({ t: 'rinfo', s: 'sec-eeee', n: 'Secret Club', o: 'Zed', f: 5, w: true, k: 'private', y: false, c: 'ABCD2345' });
  check('a locked room asks for the password and shows the price', !!t.$('rpPw') && t.$('rpFee').textContent.includes('5 credits') && t.$('rpPanel').textContent.includes('Secret Club'));
  field(t, 'rpPw', 'open sesame'); submitForm(t, 'rpPanel');
  check('the join carries code, password and price', JSON.stringify(last(t)) === JSON.stringify({ t: 'join', r: 'sec-eeee', l: 0, f: 5, c: 'ABCD2345', p: 'open sesame' }), JSON.stringify(last(t)));
  t.s.recv({ t: 'err', c: 'bad_room_password' });
  check('a wrong password is shown in the panel', t.$('rpErr').textContent === 'Wrong password.');
  t.s.recv({ t: 'err', c: 'too_many_attempts' });
  check('too many tries is shown in the panel', t.$('rpErr').textContent.toLowerCase().includes('too many'), t.$('rpErr').textContent);
  t.$('rpNo').click();
  t.$('toolCode').click();
  field(t, 'codeInput', 'ZZZZ9999'); submitForm(t, 'codeForm');
  t.s.recv({ t: 'rinfo', s: 'my-ffff', n: 'Mine', o: 'Mike', f: 0, w: false, k: 'private', y: true, c: 'ZZZZ9999' });
  check('a code for my own or a joined room goes straight in', JSON.stringify(last(t)) === JSON.stringify({ t: 'join', r: 'my-ffff', l: 0, c: 'ZZZZ9999', f: 0 }) && !t.$('rpPanel'), JSON.stringify(last(t)));
  t.s.recv({ t: 'err', c: 'no_such_room' });
  check('an unknown code is explained', t.$('status').textContent.includes('does not exist'), t.$('status').textContent);

  // ---- searching
  t.$('toolFind').click();
  field(t, 'findInput', 'gist'); submitForm(t, 'findForm');
  check('a search sends the words', JSON.stringify(last(t)) === JSON.stringify({ t: 'find', n: 'gist' }), JSON.stringify(last(t)));
  t.s.recv({ t: 'found', r: [{ s: 'free-aaaa', n: 'Free Chat', c: 'Community', g: ['Gist'], u: 0, h: 0, k: 'public', o: 'Ann' }] });
  check('results are shown as tiles', t.$('foundList').querySelectorAll('.room').length === 1 && t.$('foundList').textContent.includes('Free Chat'));
  t.$('foundList').querySelector('.room').click();
  check('a result opens like any room', last(t).t === 'join' && last(t).r === 'free-aaaa');
  t.$('toolFind').click();
  check('closing the search hides the form', !t.$('findForm'));

  // ---- creating
  t.$('toolCreate').click();
  check('the create form shows the cost and the balance', t.$('crNote').textContent.includes('25 credits') && t.$('crNote').textContent.includes('100'));
  field(t, 'crName', 'ab'); submitForm(t, 'createRoom');
  check('a bad name is refused in the page', !!t.$('createRoom') && t.$('status').textContent.includes('3 to 30'), t.$('status').textContent);
  field(t, 'crName', '  Abuja   Night Owls '); field(t, 'crKind', 'public'); field(t, 'crTags', 'Gist, Night'); field(t, 'crFee', '60');
  submitForm(t, 'createRoom');
  check('a fee above 50 is refused in the page', !!t.$('createRoom') && t.$('status').textContent.includes('0 to 50'), t.$('status').textContent);
  field(t, 'crFee', '10'); field(t, 'crPw', 'abc'); submitForm(t, 'createRoom');
  check('a short password is refused in the page', !!t.$('createRoom') && t.$('status').textContent.includes('4 to 32'));
  field(t, 'crPw', 'open sesame'); submitForm(t, 'createRoom');
  check('creating sends everything, tidied', JSON.stringify(last(t)) === JSON.stringify({ t: 'croom', n: 'Abuja Night Owls', k: 'public', g: 'Gist, Night', f: 10, c: 'General', p: 'open sesame' }), JSON.stringify(last(t)));
  check('the form closes when sent', !t.$('createRoom'));
  t.s.recv({ t: 'croomok', s: 'abuja-night-owls-x7k2', n: 'Abuja Night Owls', i: 'ABCD2345' });
  check('the new room is entered at once', JSON.stringify(last(t)) === JSON.stringify({ t: 'join', r: 'abuja-night-owls-x7k2' }) && t.$('status').textContent === 'Room created.', JSON.stringify(last(t)));
  t.s.recv({ t: 'joined', r: 'abuja-night-owls-x7k2', n: 'Abuja Night Owls', k: 'public', o: 'Mike', y: true, i: 'ABCD2345', f: 10, w: true });
  t.s.recv({ t: 'hist', m: [{ i: 1, h: 'Matt', b: 'hey all', ts: 1, r: 0 }, { i: 2, h: 'Eve', b: 'staff here', ts: 2, r: 0, o: 'mod' }, { i: 3, h: 'Mike', b: 'my own', ts: 3, r: 0 }] });

  // ---- the host in their room
  check('the host sees room tools with the invite code', !!t.$('hostTools') && t.$('inviteCode').value === 'ABCD2345');
  check('the tools show the current fee and that there is a password', t.$('feeInput').value === '10' && !!t.$('pwRemove') && t.$('pwInput').placeholder === 'New password' && t.$('kindInput').value === 'public');
  field(t, 'feeInput', '25'); submitForm(t, 'feeForm');
  check('saving a fee sends only the fee', JSON.stringify(last(t)) === JSON.stringify({ t: 'rset', f: 25 }), JSON.stringify(last(t)));
  field(t, 'feeInput', '99'); const n2 = t.s.sent.length; submitForm(t, 'feeForm');
  check('a fee above 50 is refused in the page', t.s.sent.length === n2 && t.$('status').textContent.includes('0 to 50'));
  field(t, 'pwInput', 'abc'); submitForm(t, 'pwForm');
  check('a short password is refused in the page', t.s.sent.length === n2 && t.$('status').textContent.includes('4 to 32'));
  field(t, 'pwInput', 'new secret'); submitForm(t, 'pwForm');
  check('saving a password sends only the password', JSON.stringify(last(t)) === JSON.stringify({ t: 'rset', pw: 'new secret' }) && t.$('pwInput').value === '');
  t.$('pwRemove').click();
  check('Remove sends an empty password', JSON.stringify(last(t)) === JSON.stringify({ t: 'rset', pw: '' }));
  field(t, 'kindInput', 'private'); t.$('kindInput').dispatchEvent(new t.w.Event('change', { bubbles: true }));
  check('changing the kind sends only the kind', JSON.stringify(last(t)) === JSON.stringify({ t: 'rset', k: 'private' }), JSON.stringify(last(t)));
  t.s.recv({ t: 'rsetok', f: 25, w: false, k: 'private' });
  check('the saved settings show up in the tools', t.$('feeInput').value === '25' && !t.$('pwRemove') && t.$('pwInput').placeholder === 'Add a password' && t.$('status').textContent === 'Room settings saved.');
  t.$('copyInvite').click();
  await new Promise(r => setTimeout(r, 50));
  check('copying tells the person, whatever the browser allows', t.$('status').textContent.includes('copied') || t.$('status').textContent.includes('ABCD2345'), t.$('status').textContent);
  const nc = t.s.sent.length;
  t.$('closeRoom').click();
  check('closing needs a second tap', t.s.sent.length === nc && t.$('closeRoom').textContent.includes('Tap again'));
  t.$('closeRoom').click();
  check('the second tap closes the room', JSON.stringify(last(t)) === JSON.stringify({ t: 'rclose' }));
  t.s.recv({ t: 'rclosed', s: 'abuja-night-owls-x7k2', n: 'Abuja Night Owls' });
  check('everyone is moved to the hub and told', !t.$('hub').hidden && t.$('chat').hidden && t.$('status').textContent === 'The room Abuja Night Owls was closed.' && !t.$('hostTools'), t.$('status').textContent);

  // ---- host actions on messages
  t = hub();
  t.s.recv({ t: 'joined', r: 'mine-cccc', n: 'My Den', k: 'private', o: 'Mike', y: true, i: 'ABCD2345', w: true });
  t.s.recv({ t: 'hist', m: [{ i: 1, h: 'Matt', b: 'hey all', ts: 1, r: 0 }, { i: 2, h: 'Eve', b: 'staff here', ts: 2, r: 0, o: 'mod' }, { i: 3, h: 'Mike', b: 'my own', ts: 3, r: 0 }] });
  const msg = id => t.d.querySelector('#log .m[data-id="' + id + '"]');
  const more = el => el.querySelector('button[aria-label^="More actions"]');
  const labels = el => [...el.querySelectorAll('.actions button')].map(b => b.textContent);
  more(msg(1)).click();
  check('the host can mute, kick and delete a member message', JSON.stringify(labels(msg(1))) === JSON.stringify(['Mention', 'Send a tip', 'Message', 'Add friend', 'Report', 'Mute 10 min here', 'Kick 10 min', 'Delete']), labels(msg(1)));
  btn(msg(1), 'Mute 10 min here').click();
  check('mute sends the right frame', JSON.stringify(last(t)) === JSON.stringify({ t: 'hmod', a: 'mute', h: 'Matt', m: 10 }), JSON.stringify(last(t)));
  more(msg(1)).click(); btn(msg(1), 'Kick 10 min').click();
  check('kick sends the right frame', JSON.stringify(last(t)) === JSON.stringify({ t: 'hmod', a: 'kick', h: 'Matt', m: 10 }));
  more(msg(1)).click(); btn(msg(1), 'Delete').click();
  check('delete sends the message id', JSON.stringify(last(t)) === JSON.stringify({ t: 'del', i: 1 }), JSON.stringify(last(t)));
  more(msg(2)).click();
  check('there are no host powers over a staff message', !btn(msg(2), 'Kick 10 min') && !btn(msg(2), 'Mute 10 min here'));
  more(msg(3)).click();
  check('on the host\'s own message there is only Delete', JSON.stringify(labels(msg(3))) === JSON.stringify(['Delete']), labels(msg(3)));

  // ---- being muted and kicked by a host
  t.s.recv({ t: 'rmuted', m: 5 });
  check('a muted member is told in the room', [...t.d.querySelectorAll('#log .m')].some(e => e.textContent.includes('The host muted you in this room for 5 minutes')));
  t.s.recv({ t: 'err', c: 'room_muted' });
  check('and gets a plain refusal when they try', [...t.d.querySelectorAll('#log .m')].some(e => e.textContent.includes('The host muted you in this room.')));
  t.s.recv({ t: 'rkicked', m: 10, n: 'My Den' });
  check('a kicked member goes to the hub with an explanation', !t.$('hub').hidden && t.$('chat').hidden && t.$('status').textContent.includes('removed you from My Den for 10 minutes'), t.$('status').textContent);

  // ---- a member in someone else's room has no tools, and errors show on the hub
  t.s.recv({ t: 'joined', r: 'free-aaaa', n: 'Free Chat', k: 'public', o: 'Ann' });
  check('no tools in a room somebody else hosts', !t.$('hostTools'));
  t.s.recv({ t: 'hist', m: [{ i: 1, h: 'Matt', b: 'hey all', ts: 1, r: 0 }] });
  more(msg(1)).click();
  check('and no host actions on messages', !btn(msg(1), 'Kick 10 min'));
  t.$('back').click(); t.s.recv({ t: 'left' });
  t.s.recv({ t: 'err', c: 'room_limit', m: 1 });
  check('the room limit is explained on the hub', t.$('status').textContent === 'You can host 1 room at a time. Close one first.', t.$('status').textContent);
  t.s.recv({ t: 'err', c: 'no_credits', m: 25 });
  check('and so is the creation fee', t.$('status').textContent.includes('25 credits'), t.$('status').textContent);
  t.s.recv({ t: 'err', c: 'room_name_taken' });
  check('and a taken name', t.$('status').textContent === 'That room name is taken.');
  t.$('toolCreate').click(); field(t, 'crName', 'Newbie Room'); submitForm(t, 'createRoom');
  t.s.recv({ t: 'err', c: 'tier_locked', m: 1 });
  check('creating before Regular says what is needed', t.$('status').textContent === 'Creating a room needs Regular or higher.', t.$('status').textContent);
  t.s.recv({ t: 'err', c: 'no_credits' });
  check('a no credits error without a number still talks about messages', t.$('status').textContent.includes('message'), t.$('status').textContent);

  // ---- logging out forgets it all
  t.s.recv({ t: 'joined', r: 'mine-cccc', n: 'My Den', k: 'private', o: 'Mike', y: true, i: 'ABCD2345' });
  t.$('back').click(); t.s.recv({ t: 'left' });
  t.$('tabProfile').click(); t.$('logout').click(); t.$('logoutAnyway') && t.$('logoutAnyway').click();
  check('logging out clears the room state', !t.$('rpPanel') && !t.$('hostTools'));

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
})();
