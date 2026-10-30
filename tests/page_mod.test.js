const { JSDOM } = require('jsdom');
const fs = require('fs');
const html = fs.readFileSync(require('path').join(__dirname, 'build', 'page.html'), 'utf8');
let pass = 0, fail = 0;
const check = (name, cond, extra) => { if (cond) { pass++; console.log('PASS', name); } else { fail++; console.log('FAIL', name, extra || ''); } };

function boot(role) {
  const sockets = [], longTimers = [];
  const dom = new JSDOM(html, {
    url: 'https://chat.example/', runScripts: 'dangerously', pretendToBeVisual: true,
    beforeParse(w) {
      const flush = () => { if (w.__lobbyFlush) w.__lobbyFlush(); };
      const origClick = w.HTMLElement.prototype.click;
      w.HTMLElement.prototype.click = function () { origClick.call(this); flush(); };
      const origDispatch = w.EventTarget.prototype.dispatchEvent;
      w.EventTarget.prototype.dispatchEvent = function (e) { const r = origDispatch.call(this, e); flush(); return r; };
      w.localStorage.setItem('lobby.k', 'tok');
      w.Element.prototype.scrollIntoView = function () {};
      const real = w.setTimeout.bind(w);
      w.setTimeout = (fn, ms, ...a) => { if (ms >= 60000) { longTimers.push({ fn, ms }); return 0; } return real(fn, ms, ...a); };
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
  const s = sockets[0]; s.open();
  const okf = { t: 'ok', h: 'Me', k: 'tok', p: 5, r: 0, x: 300, s: 1, g: true };
  if (role) okf.o = role;
  s.recv(okf);
  s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' });
  s.recv({ t: 'hist', m: [
    { i: 1, h: 'Matt', b: 'you are all idiots', ts: 1, r: 0 },
    { i: 2, h: 'Me', b: 'my own message', ts: 2, r: 0 },
  ]});
  return { w, d, s, sockets, longTimers, $: id => d.getElementById(id),
    msg: id => d.querySelector('#log .m[data-id="' + id + '"]'),
    lastLine: cls => [...d.querySelectorAll('#log .m.' + cls)].pop() };
}
const btn = (root, label) => [...root.querySelectorAll('button')].find(b => b.textContent === label);
const labels = root => [...root.querySelectorAll('.actions button')].map(b => b.textContent);
const more = el => el.querySelector('button[aria-label^="More actions"]');

(async () => {
  // ---- normal user
  let t = boot('');
  check('normal user: no staff tools on the profile or in the sidebar', t.$('staffCard').hidden && !t.$('tabMod') && !t.$('tabAdmin'));
  more(t.msg(1)).click();
  check('normal user: Mention, Send a tip, Message, Add friend and Report on someone else\'s message', JSON.stringify(labels(t.msg(1))) === JSON.stringify(['Mention', 'Send a tip', 'Message', 'Add friend', 'Report']), labels(t.msg(1)));
  btn(t.msg(1), 'Report').click();
  check('Report opens a sheet that asks why', !!t.$('reportSheet') && [...t.$('reportSheet').querySelectorAll('[data-reason]')].map(b => b.dataset.reason).join() === 'spam,abuse,harassment,scam,other');
  t.$('reportSheet').querySelector('[data-reason=abuse]').click();
  t.$('repSend').click();
  check('the report is sent with the message id and reason', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'rep', i: 1, c: 'abuse' }));
  check('the menu closes after reporting', !t.msg(1).querySelector('.actions'));
  more(t.msg(1)).click(); more(t.msg(1)).click();
  check('More toggles the menu', !t.msg(1).querySelector('.actions'));
  more(t.msg(2)).click();
  check('own message: only Delete, no Report', JSON.stringify(labels(t.msg(2))) === JSON.stringify(['Delete']), labels(t.msg(2)));
  btn(t.msg(2), 'Delete').click();
  check('members delete their own message with the plain delete frame', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'del', i: 2 }));
  t.s.recv({ t: 'repok' });
  check('thanks shown after a report', t.lastLine('up').textContent.startsWith('Thanks'));
  t.s.recv({ t: 'err', c: 'already_reported' });
  check('duplicate report explained', t.lastLine('sys').textContent === 'You already reported that message.');
  t.s.recv({ t: 'err', c: 'forbidden' });
  check('forbidden explained', t.lastLine('sys').textContent === 'You cannot do that.');
  t.s.recv({ t: 'err', c: 'muted', m: 7 });
  check('muted error shows minutes', t.lastLine('sys').textContent === 'You are muted for 7 more minutes.');
  t.s.recv({ t: 'err', c: 'muted', m: 1 });
  check('one minute is singular', t.lastLine('sys').textContent === 'You are muted for 1 more minute.');

  // ---- moderator
  t = boot('mod');
  check('moderator: role badge, staff card and Moderation link visible, no admin console', !t.$('staffCard').hidden && t.$('myBadge').textContent === 'MOD' && !!t.$('tabMod') && !t.$('tabAdmin') && !t.$('staffAdmin'));
  t.s.recv({ t: 'rn', c: 3 });
  check('report count shows on the Moderation link', t.$('tabMod').querySelector('.badge').textContent === '3');
  more(t.msg(1)).click();
  check('moderator menu on someone else\'s message', JSON.stringify(labels(t.msg(1))) === JSON.stringify(['Pin to room', 'Mention', 'Send a tip', 'Message', 'Add friend', 'Report', 'Mute 10 min', 'Mute 1 hour', 'Kick 10 min', 'Delete']), labels(t.msg(1)));
  btn(t.msg(1), 'Mute 10 min').click();
  check('mute 10 sends the right frame', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'mute', h: 'Matt', m: 10 }));
  more(t.msg(1)).click(); btn(t.msg(1), 'Mute 1 hour').click();
  check('mute 1 hour', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'mute', h: 'Matt', m: 60 }));
  more(t.msg(1)).click(); btn(t.msg(1), 'Kick 10 min').click();
  check('kick', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'kick', h: 'Matt', m: 10 }));
  more(t.msg(1)).click(); btn(t.msg(1), 'Delete').click();
  check('delete', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'del', i: 1 }));
  more(t.msg(2)).click();
  check('moderator on own message: Pin and Delete', JSON.stringify(labels(t.msg(2))) === JSON.stringify(['Pin to room', 'Delete']), labels(t.msg(2)));
  btn(t.msg(2), 'Delete').click();
  check('a moderator deleting their own message uses the plain frame too', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'del', i: 2 }));

  // ---- moderation dashboard (its own page, reached from the sidebar)
  const m = boot('mod');
  m.$('tabMod').click(); m.s.recv({ t: 'left' });
  check('the Moderation link opens the dashboard and leaves the room', !!m.$('staffArea') && m.$('shell').dataset.area === 'mod' && m.$('title').textContent.startsWith('Moderation'));
  check('opening it asks for the overview numbers', m.s.sent.some(x => x.t === 'astats'));
  check('the sidebar switches to the moderator sections, with no Moderators or Audit log', !!m.$('sec-reports') && !!m.$('sec-users') && !!m.$('sec-rooms') && !m.$('sec-moderators') && !m.$('sec-log'));
  m.s.sent.length = 0;
  m.$('sec-reports').click();
  check('the Reports section asks for the list', JSON.stringify(m.s.sent.pop()) === JSON.stringify({ t: 'reports' }));
  m.s.recv({ t: 'reps', r: [
    { id: 11, by: 'dan', h: 'bob', i: 1, b: '<img src=x onerror="window.pwned=1"> rude', c: 'abuse', ts: 5 },
    { id: 12, by: 'ann', h: 'cleo', i: 9, b: 'buy cheap stuff', c: 'spam', ts: 6 },
  ]});
  const items = [...m.d.querySelectorAll('#repList .rep')];
  check('two reports listed', items.length === 2);
  check('report shows who, what and why', items[0].querySelector('.meta').textContent.startsWith('@dan reported @bob for abuse') && items[1].querySelector('.what').textContent === 'buy cheap stuff');
  check('html in a report stays text', !m.w.pwned && !items[0].querySelector('img') && items[0].querySelector('.what').textContent.includes('<img'));
  btn(items[0], 'Mute 1 hour').click();
  check('mute from a report carries the report id', JSON.stringify(m.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'mute', h: 'bob', m: 60, rid: 11 }));
  btn(items[0], 'Kick 10 min').click();
  check('kick from a report', JSON.stringify(m.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'kick', h: 'bob', m: 10, rid: 11 }));
  btn(items[1], 'Delete message').click();
  check('delete from a report', JSON.stringify(m.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'del', i: 9, rid: 12 }));
  btn(items[1], 'Dismiss').click();
  check('dismiss', JSON.stringify(m.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'dismiss', id: 12 }));
  m.s.sent.length = 0;
  m.s.recv({ t: 'rn', c: 1 });
  check('a new count while the Reports section is open refreshes the list', JSON.stringify(m.s.sent.pop()) === JSON.stringify({ t: 'reports' }));
  m.s.recv({ t: 'reps', r: [] });
  check('empty list says so', m.$('repList').textContent.trim() === 'No open reports.');
  m.$('sec-overview').click(); m.s.sent.length = 0;
  m.s.recv({ t: 'rn', c: 4 });
  check('on another section a new count does not ask for the list', !m.s.sent.some(x => x.t === 'reports'));
  m.s.recv({ t: 'modok', a: 'mute', h: 'bob' });
  check('actions in the dashboard are confirmed and the page refreshes', m.$('status').textContent === 'Muted.' && m.s.sent.some(x => x.t === 'astats'), m.$('status').textContent);
  m.$('areaExit').click();
  check('Back to the app returns to the hub', !m.$('hub').hidden && !m.$('staffArea') && !m.$('shell').dataset.area);

  t.s.recv({ t: 'modok', a: 'dismiss', id: 12 });
  const before = t.d.querySelectorAll('#log .m').length;
  t.s.recv({ t: 'modok', a: 'mute', h: 'bob' });
  check('confirmation line for actions but not for dismiss', t.d.querySelectorAll('#log .m').length === before + 1 && t.lastLine('up').textContent === 'Done.');

  // ---- room notices
  t.s.recv({ t: 'mod', a: 'mute', h: 'bob', m: 10 });
  check('room sees a mute', t.lastLine('mod').textContent === '@bob was muted for 10 minutes.');
  t.s.recv({ t: 'mod', a: 'kick', h: 'cleo', m: 1 });
  check('room sees a kick', t.lastLine('mod').textContent === '@cleo was removed for 1 minute.');
  t.s.recv({ t: 'muted', m: 30 });
  check('the muted user is told', t.lastLine('sys').textContent === 'You were muted for 30 minutes.');
  t.s.recv({ t: 'unmuted' });
  check('unmute is announced to the user', t.lastLine('up').textContent === 'You can chat again.');

  // ---- delete removes the message and a pending reply
  t.msg(1).querySelector('.replybtn').click();
  check('reply bar open before the delete', !t.$('replybar').hidden);
  t.s.recv({ t: 'del', i: 1 });
  check('deleted message disappears', !t.msg(1));
  check('a reply to a deleted message is cancelled', t.$('replybar').hidden);
  t.s.recv({ t: 'del', i: 777 });
  check('deleting an unknown id is harmless', !!t.msg(2));

  // ---- losing the role
  t.s.recv({ t: 'me', p: 5, r: 0, x: 300, s: 1, g: true });
  check('losing the role hides the staff tools', t.$('staffCard').hidden && !t.$('tabMod') && !t.$('staffNav'));
  more(t.msg(2)).click();
  check('and the menu changes with it', JSON.stringify(labels(t.msg(2))) === JSON.stringify(['Delete']), labels(t.msg(2)));

  // ---- offline click keeps the menu
  t = boot('mod');
  more(t.msg(1)).click();
  t.s.readyState = 3;
  const sentBefore = t.s.sent.length;
  btn(t.msg(1), 'Kick 10 min').click();
  check('offline: nothing is sent and the menu stays', t.s.sent.length === sentBefore && !!t.msg(1).querySelector('.actions') && t.$('status').textContent.startsWith('Offline'));

  // ---- kicked
  t = boot('');
  t.s.recv({ t: 'kicked', m: 5 });
  check('kicked: blocked screen replaces the app', !t.$('blocked').hidden && t.$('app').hidden && t.$('join').hidden);
  check('kicked: message says how long', t.$('blockedMsg').textContent.includes('5 minutes'), t.$('blockedMsg').textContent);
  t.s.drop();
  await new Promise(r => setTimeout(r, 1200));
  check('kicked: no reconnect attempts while removed', t.sockets.length === 1, t.sockets.length);
  check('kicked: a return timer was set for the kick time plus a margin', t.longTimers.length === 1 && t.longTimers[0].ms === 5 * 60000 + 1500, t.longTimers.map(x => x.ms));
  t.longTimers[0].fn(); t.w.__lobbyFlush();
  check('kicked: when time is up the page returns by itself, to the same room', t.$('blocked').hidden && !t.$('app').hidden && !t.$('chat').hidden && t.sockets.length === 2);
  t.sockets[1].open();
  check('kicked: it comes back with the saved login', JSON.stringify(t.sockets[1].sent[0]) === JSON.stringify({ t: 'hi', k: 'tok', r: 'lobby', l: 2 }), JSON.stringify(t.sockets[1].sent[0]));
  t.sockets[1].recv({ t: 'err', c: 'kicked', m: 3 });
  check('still kicked when it returns: blocked again with the new time', !t.$('blocked').hidden && t.$('blockedMsg').textContent.includes('3 minutes'));
  t.sockets[1].drop();
  await new Promise(r => setTimeout(r, 800));
  check('and still no reconnect loop', t.sockets.length === 2);
  check('the login token is kept', t.w.localStorage.getItem('lobby.k') === 'tok');

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
})();
