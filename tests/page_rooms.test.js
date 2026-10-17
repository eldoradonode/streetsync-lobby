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
      w.localStorage.setItem('lobby.k', 'tok');
      if (opts.room) w.sessionStorage.setItem('lobby.r', opts.room);
      w.Element.prototype.scrollIntoView = function () {};
      const real = w.setTimeout.bind(w);
      w.setTimeout = (fn, ms, ...a) => (ms >= 60000 ? 0 : real(fn, ms, ...a));
      w.WebSocket = class {
        constructor() { this.readyState = 0; this.sent = []; sockets.push(this); }
        send(d) { this.sent.push(JSON.parse(d)); }
        close() { this.readyState = 3; }
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
  { s: 'lobby', n: 'General Chat', c: 'General', g: ['Vibes', 'Friends', 'Random'], u: 12, h: 3 },
  { s: 'football', n: 'Football', c: 'Sports', g: ['Matches', 'Banter'], u: 0, h: 0 },
  { s: 'elite-lounge', n: 'Elite Lounge', c: 'Members', g: ['Elite'], u: 2, h: 5, t: 3 },
];
const okf = (extra = {}) => ({ t: 'ok', h: 'Me', k: 'tok', p: 5, r: 0, x: 300, s: 1, g: true, ...extra });
const rows = t => [...t.d.querySelectorAll('#roomList .room')];
const view = t => (!t.$('join').hidden ? 'join' : !t.$('chat').hidden ? 'room' : !t.$('hub').hidden ? 'hub' : 'none');

(async () => {
  // ---- hub
  let t = boot();
  t.s.open();
  check('starts signed in on the join screen state only when there is no token', true);
  t.s.recv(okf());
  t.s.recv({ t: 'rooms', r: ROOMS });
  check('after sign in you land on the hub, not in a room', view(t) === 'hub' && !t.$('app').hidden);
  check('the hub header says Home, with no back button and no head count', t.$('title').textContent === 'Home' && t.$('back').hidden && t.$('online').hidden);
  check('one row per room, in order', rows(t).map(r => r.querySelector('.nm').textContent).join('|') === 'General Chat|Football|Elite Lounge');
  check('rows show tags', [...rows(t)[0].querySelectorAll('.tag2')].map(x => x.textContent).join(',') === 'Vibes,Friends,Random');
  check('rows show how many are online', rows(t)[0].querySelector('.on').textContent === '12 online' && rows(t)[1].querySelector('.on').textContent === '0 online');
  const flames = r => [...r.querySelectorAll('.flames span')];
  check('heat is five flames, filled by level', flames(rows(t)[0]).length === 5 && flames(rows(t)[0]).filter(f => !f.classList.contains('off')).length === 3 && flames(rows(t)[1]).every(f => f.classList.contains('off')));
  check('a locked room says which tier opens it and looks locked', rows(t)[2].classList.contains('locked') && rows(t)[2].querySelector('.lock').textContent === 'Needs Elite' && !rows(t)[0].querySelector('.lock'));

  // ---- entering and refusing
  rows(t)[2].click();
  check('tapping a locked room sends nothing and explains', t.s.sent.filter(x => x.t === 'join').length === 0 && t.$('status').textContent === 'That room needs Elite or higher.', t.$('status').textContent);
  rows(t)[1].click();
  check('tapping a room asks to join it', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'join', r: 'football', l: 0 }));
  t.s.recv({ t: 'joined', r: 'football', n: 'Football' });
  t.s.recv({ t: 'hist', m: [{ i: 5, h: 'Matt', b: 'kick off', ts: 1, r: 0 }, { i: 6, h: 'Ada', b: 'goal', ts: 2, r: 0 }] });
  t.s.recv({ t: 'n', c: 7 });
  check('joining shows the room: name, back button and head count', view(t) === 'room' && t.$('title').textContent === 'Football' && !t.$('back').hidden && !t.$('online').hidden && t.$('count').textContent === '7');
  check('the room shows its messages', t.d.querySelectorAll('#log .m').length === 2);

  // ---- leaving and returning
  t.$('back').click();
  check('the back button asks to leave', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'leave' }));
  t.s.recv({ t: 'left' });
  t.s.recv({ t: 'rooms', r: ROOMS.map(r => r.s === 'football' ? { ...r, u: 3, h: 2 } : r) });
  check('leaving returns to the hub with fresh counts', view(t) === 'hub' && rows(t)[1].querySelector('.on').textContent === '3 online');
  check('and forgets the saved room', t.w.sessionStorage.getItem('lobby.r') === null);
  rows(t)[1].click();
  check('going back into the same room asks only for what is new', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'join', r: 'football', l: 6 }));
  t.s.recv({ t: 'joined', r: 'football', n: 'Football' });
  t.s.recv({ t: 'hist', m: [{ i: 7, h: 'Matt', b: 'half time', ts: 3, r: 0 }] });
  check('the old messages are still there and the new one is added', t.d.querySelectorAll('#log .m').length === 3);
  t.$('back').click(); t.s.recv({ t: 'left' });
  rows(t)[0].click(); t.s.sent.pop();
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' });
  t.s.recv({ t: 'hist', m: [{ i: 20, h: 'Zed', b: 'lobby talk', ts: 4, r: 0 }] });
  check('another room starts with its own clean screen', t.d.querySelectorAll('#log .m').length === 1 && t.$('title').textContent === 'General Chat');
  t.$('back').click(); t.s.recv({ t: 'left' });
  rows(t)[1].click();
  check('coming back to a room you left for another asks for its whole history', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'join', r: 'football', l: 0 }));
  t.s.recv({ t: 'joined', r: 'football', n: 'Football' });
  t.s.recv({ t: 'hist', m: [{ i: 5, h: 'Matt', b: 'kick off', ts: 1, r: 0 }, { i: 6, h: 'Ada', b: 'goal', ts: 2, r: 0 }, { i: 7, h: 'Matt', b: 'half time', ts: 3, r: 0 }] });
  check('and gets all of it, nothing lost to the old marker', t.d.querySelectorAll('#log .m').length === 3);

  // ---- live hub updates and tier
  t.$('back').click(); t.s.recv({ t: 'left' }); t.s.recv({ t: 'rooms', r: ROOMS });
  t.s.recv({ t: 'hub', r: [{ s: 'lobby', u: 40, h: 4 }, { s: 'football', u: 9, h: 1 }, { s: 'elite-lounge', u: 2, h: 5 }] });
  check('hub updates change the counts and flames', rows(t)[0].querySelector('.on').textContent === '40 online' && flames(rows(t)[0]).filter(f => !f.classList.contains('off')).length === 4);
  t.s.recv({ t: 'hub', r: [{ s: 'ghost', u: 1, h: 1 }] });
  check('an update for an unknown room is ignored', rows(t).length === 3);
  t.s.recv({ t: 'me', p: 10000, r: 3, x: 40000, s: 2, g: true });
  check('reaching the tier unlocks the room in the list', !rows(t)[2].classList.contains('locked') && !rows(t)[2].querySelector('.lock'));
  rows(t)[2].click();
  check('and it can be entered', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'join', r: 'elite-lounge', l: 0 }));

  // ---- errors from the server
  t.s.recv({ t: 'err', c: 'tier_locked', m: 3 });
  check('a refusal while on the hub only shows a message', view(t) === 'hub' && t.$('status').textContent === 'That room needs Elite or higher.');
  t.s.recv({ t: 'err', c: 'no_such_room' });
  check('unknown room', t.$('status').textContent === 'That room does not exist.');

  // ---- reconnecting into the room
  t = boot();
  t.s.open(); t.s.recv(okf()); t.s.recv({ t: 'rooms', r: ROOMS });
  rows(t)[1].click(); t.s.sent.pop();
  t.s.recv({ t: 'joined', r: 'football', n: 'Football' });
  t.s.recv({ t: 'hist', m: [{ i: 5, h: 'Matt', b: 'kick off', ts: 1, r: 0 }, { i: 9, h: 'Ada', b: 'goal', ts: 2, r: 0 }] });
  t.s.drop();
  await new Promise(r => setTimeout(r, 700));
  const s2 = t.sockets[1]; s2.open();
  check('reconnecting names the room and the last message seen', JSON.stringify(s2.sent[0]) === JSON.stringify({ t: 'hi', k: 'tok', r: 'football', l: 9 }), JSON.stringify(s2.sent[0]));
  check('the room stays on screen while reconnecting', view(t) === 'room');
  s2.recv(okf()); s2.recv({ t: 'rooms', r: ROOMS });
  s2.recv({ t: 'joined', r: 'football', n: 'Football' });
  s2.recv({ t: 'hist', m: [{ i: 10, h: 'Matt', b: 'second half', ts: 3, r: 0 }] });
  check('after reconnecting you are still in the room with the new message added', view(t) === 'room' && t.d.querySelectorAll('#log .m').length === 3);
  s2.recv({ t: 'err', c: 'tier_locked', m: 3 });
  check('if the saved room no longer works you land on the hub', view(t) === 'hub' && t.w.sessionStorage.getItem('lobby.r') === null);
  s2.recv({ t: 'joined', r: 'football', n: 'Football' });
  s2.recv({ t: 'err', c: 'no_room' });
  check('a server that says you are in no room sends you to the hub', view(t) === 'hub' && t.$('status').textContent === 'Pick a room first.');

  // ---- reload inside a room
  t = boot({ room: 'football' });
  check('reloading inside a room shows the room right away', view(t) === 'room');
  t.s.open();
  check('and asks to go straight back into it', JSON.stringify(t.s.sent[0]) === JSON.stringify({ t: 'hi', k: 'tok', r: 'football', l: 0 }), JSON.stringify(t.s.sent[0]));

  // ---- staff badges
  t = boot();
  t.s.open(); t.s.recv(okf({ o: 'admin' })); t.s.recv({ t: 'rooms', r: ROOMS });
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' });
  t.s.recv({ t: 'hist', m: [
    { i: 1, h: 'Boss', b: 'admin here', ts: 1, r: 0, o: 'admin' },
    { i: 2, h: 'Cleo', b: 'mod here', ts: 2, r: 1, o: 'mod' },
    { i: 3, h: 'Matt', b: 'member here', ts: 3, r: 0 },
  ]});
  const msg = id => t.d.querySelector('#log .m[data-id="' + id + '"]');
  check('an admin message shows ADMIN instead of a tier badge', msg(1).querySelector('.bd').textContent === 'ADMIN' && msg(1).querySelector('.bd').classList.contains('r-admin') && !msg(1).textContent.includes('NEW'));
  check('an admin message has its own look', msg(1).classList.contains('r-admin') && msg(1).querySelector('.who').classList.contains('r-admin'));
  check('a mod message shows MOD', msg(2).querySelector('.bd').textContent === 'MOD' && msg(2).classList.contains('r-mod') && msg(2).querySelector('.who').classList.contains('r-mod'));
  check('a member still shows the tier badge', msg(3).querySelector('.bd').textContent === 'NEW' && !msg(3).className.includes('r-'));
  check('my own badge shows the role in the role colors', t.$('myBadge').textContent === 'ADMIN' && t.$('myBadge').classList.contains('r-admin'));
  t.s.recv({ t: 'me', p: 5, r: 0, x: 300, s: 1, g: true, o: 'mod' });
  check('the badge follows a role change', t.$('myBadge').textContent === 'MOD' && t.$('myBadge').classList.contains('r-mod'));

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
})();
