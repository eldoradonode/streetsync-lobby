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
  const signed = (extra = {}) => { const t = boot(); t.s.open(); t.s.recv(okf({ c: 20, g: true, ...extra })); t.s.recv({ t: 'rooms', r: ROOMS }); return t; };
  const last = t => t.s.sent[t.s.sent.length - 1];
  const sentOf = (t, type) => t.s.sent.filter(m => m.t === type);

  // ---- Settings: delete account, two stages, password account
  let t = signed();
  t.$('tabSettings').click();
  check('settings has a clearly labelled Delete account section', !!t.$('setDelete') && t.$('setDelete').textContent.includes('Delete account'));
  check('settings has a Help section', !!t.$('setHelp') && t.$('setHelp').textContent.includes('Report staff misconduct'));
  t.$('delStart').click();
  const txt = t.$('setDelete').textContent;
  check('the consequences are explained before anything is asked', txt.includes('forfeited') && txt.includes('Community rooms you host are closed') && txt.includes('Credit records stay'));
  check('a password account is asked for its password', !!t.$('delPw') && !t.$('delHandle'));
  typeInto(t, t.$('delPw'), 'secret-pass'); sub(t, t.$('delVerify'));
  check('step one sends the password with delreq', JSON.stringify(last(t)) === JSON.stringify({ t: 'delreq', p: 'secret-pass' }));
  t.s.recv({ t: 'err', c: 'bad_password' });
  check('a wrong password is explained', t.$('delErr') && t.$('delErr').textContent === 'That password is not right.');
  check('and nothing was deleted yet', !t.$('delFinal') && !!t.$('delVerify'));
  sub(t, t.$('delVerify')); t.s.recv({ t: 'delready', k: 'KEY123', x: 300 });
  check('the final confirmation appears only after the server accepted step one', !!t.$('delFinal') && !t.$('delVerify'));
  check('the delete button stays disabled until DELETE is typed', t.$('delGo').disabled);
  typeInto(t, t.$('delWord'), 'delete');
  check('lower case does not enable it', t.$('delGo').disabled);
  typeInto(t, t.$('delWord'), 'DELETE'); sub(t, t.$('delFinal'));
  check('step two sends the one time token and the word', JSON.stringify(last(t)) === JSON.stringify({ t: 'delgo', k: 'KEY123', c: 'DELETE' }));
  t.s.recv({ t: 'delok', n: 0, f: 0 });
  check('after deletion the person is signed out, the saved login is gone', view(t) === 'join' && t.w.localStorage.getItem('lobby.k') === null);
  check('and told why', t.$('join').textContent.includes('Your account was deleted'));

  // guest account: confirms by typing the handle
  t = signed({ g: false });
  t.$('tabSettings').click(); t.$('delStart').click();
  check('a guest is asked to type their handle', !!t.$('delHandle') && !t.$('delPw'));
  typeInto(t, t.$('delHandle'), 'Me'); sub(t, t.$('delVerify'));
  check('the guest request carries the handle', JSON.stringify(last(t)) === JSON.stringify({ t: 'delreq', h: 'Me' }));
  t.$('setDelete').querySelector('.ghost').click();
  check('cancel closes the flow without sending delgo', sentOf(t, 'delgo').length === 0 && !!t.$('delStart'));

  // ---- Create room: description
  t = signed();
  t.$('tabRooms').click(); t.$('rdCreate').click();
  check('the create form has a description with a counter', !!t.$('crDesc') && t.$('crDescLeft').textContent === '0/200');
  typeInto(t, t.$('crDesc'), 'Late night gist'); typeInto(t, t.$('crName'), 'Night Gist');
  check('the counter follows typing', t.$('crDescLeft').textContent === '15/200');
  sub(t, t.$('createRoom'));
  const cr = sentOf(t, 'croom').pop();
  check('the description is sent with the room', cr && cr.d === 'Late night gist' && cr.n === 'Night Gist');

  // ---- Hub: featured and categories
  t = signed();
  check('the home page has a Featured section', !!t.$('hubFeatured') && t.$('hubFeatured').textContent.includes('General Chat'));
  check('the category grid is gone from the home page (step 17)', !t.$('hubCats') && !t.d.querySelector('.cattile') && !t.$('hub').textContent.includes('Browse by category'));
  const before = (x, y) => !!(x.compareDocumentPosition(y) & 4); // y follows x
  const between = [...t.$('hub').querySelectorAll('.room')].filter(r => before(t.$('hub').querySelector('.pagehead'), r) && !t.$('roomList').contains(r) && before(r, t.$('roomList')));
  check('official rooms come right after the welcome banner', t.$('roomList').querySelectorAll('.room').length > 0 && between.length === 0 && before(t.$('roomList'), t.$('hubFeatured')) && before(t.$('roomList'), t.$('communityHead')));
  t.$('tabRooms').click();
  check('category filtering still works on the rooms page', !!t.$('rdChips') && t.$('rdChips').querySelectorAll('.chipbtn').length > 2);

  // ---- Rooms: people tab
  t = signed();
  t.$('tabRooms').click();
  typeInto(t, t.$('rdSearch'), 'ann');
  t.$('rdSearch').form.dispatchEvent(new t.w.Event('submit', { cancelable: true }));
  check('searching asks for rooms and people', sentOf(t, 'find').length > 0 && sentOf(t, 'usearch').pop()?.n === 'ann');
  t.$('tabUsersRes').click();
  t.s.recv({ t: 'ufound', q: 'ann', r: [{ h: 'annie', r: 1 }, { h: 'Boss', o: 'admin' }] });
  check('people results are listed with a staff badge', t.d.querySelectorAll('#userResults .urow').length === 2 && t.$('userResults').textContent.includes('ADMIN'));
  t.d.querySelector('#userResults [data-handle=annie]').click();
  check('choosing a person opens their profile', JSON.stringify(sentOf(t, 'profile').pop()) === JSON.stringify({ t: 'profile', h: 'annie' }));

  // ---- Public profile: bio, staff shield, staff complaint path
  const openProfile = (t, h, p) => {
    t.$('tabRooms').click(); typeInto(t, t.$('rdSearch'), h.toLowerCase());
    t.$('rdSearch').form.dispatchEvent(new t.w.Event('submit', { cancelable: true }));
    t.$('tabUsersRes').click(); t.s.recv({ t: 'ufound', q: h.toLowerCase(), r: [{ h, r: 1 }] });
    t.d.querySelector('#userResults [data-handle=' + h + ']').click(); t.s.recv({ t: 'uprofile', h, ...p });
  };
  t = signed();
  openProfile(t, 'Boss', { o: 'admin', sh: true, s: 'none', bio: 'Keeping it tidy', p: 900 }); void ({ t: 'uprofile', h: 'Boss', o: 'admin', sh: true, s: 'none', bio: 'Keeping it tidy', p: 900 });
  check('the public bio is shown', t.$('userBio') && t.$('userBio').textContent === 'Keeping it tidy');
  check('no message, friend or tip buttons for staff', !t.$('userMessage') && !t.$('userFriend') && !t.d.querySelector('#user [data-act=tip]'));
  check('there is a way to report staff misconduct', !!t.$('userReportStaff') && !t.$('userReport'));
  t.$('userReportStaff').click();
  const reasons = [...t.$('reportSheet').querySelectorAll('[data-reason]')].map(b => b.dataset.reason).join();
  check('the staff report sheet has staff reasons and says only admins read it', reasons === 'abuse,harassment,unfair,other' && t.$('reportSheet').textContent.includes('admins only'));
  t.$('repSend').click();
  check('a reason is required first', sentOf(t, 'scomp').length === 0 && t.$('repErr').textContent.includes('reason'));
  t.$('reportSheet').querySelector('[data-reason=abuse]').click();
  typeInto(t, t.$('repDetails'), 'muted me for no reason'); t.$('repSend').click();
  check('the complaint carries target, reason and details', JSON.stringify(last(t)) === JSON.stringify({ t: 'scomp', h: 'Boss', i: 0, c: 'abuse', b: 'muted me for no reason' }));
  t.s.recv({ t: 'scompok' });
  check('the sheet closes once sent', !t.$('reportSheet'));

  t.$('back').click();
  openProfile(t, 'ann', { s: 'none', bio: '', p: 10 });
  check('an ordinary profile keeps message and report buttons', !!t.$('userMessage') && !!t.$('userReport'));
  t.$('userReport').click(); t.$('reportSheet').querySelector('[data-reason=harassment]').click(); typeInto(t, t.$('repDetails'), 'x'.repeat(10)); t.$('repSend').click();
  check('a person report is a urep frame', JSON.stringify(last(t)) === JSON.stringify({ t: 'urep', h: 'ann', c: 'harassment', b: 'xxxxxxxxxx' }));
  t.s.recv({ t: 'err', c: 'already_reported' });
  check('a duplicate is explained inside the sheet', t.$('repErr').textContent.includes('already'));
  t.s.recv({ t: 'err', c: 'staff_shield' });

  // ---- Room tiles: joined state and leave
  t = signed();
  t.s.recv({ t: 'rooms', r: [{ s: 'g1', n: 'Gist Room', c: 'General', g: [], u: 2, h: 1, k: 'public', o: 'zed', d: 'All the gist', j: true }, { s: 'g2', n: 'Other', c: 'General', g: [], u: 1, h: 0, k: 'public', o: 'zed', d: '' }] });
  t.$('tabRooms').click();
  const tile = t.d.querySelector('[data-slug=g1]').closest('.roomwrap');
  check('a room card shows its description', tile.textContent.includes('All the gist'));
  check('a joined room says Joined and offers Leave', !!tile.querySelector('[data-joined]') && !!tile.querySelector('[data-act=leave]'));
  check('a room I have not joined has no Leave', !t.d.querySelector('[data-slug=g2]').closest('.roomwrap').querySelector('[data-act=leave]'));
  tile.querySelector('[data-act=leave]').click();
  check('leaving needs a second tap', sentOf(t, 'rleave').length === 0);
  tile.querySelector('[data-act=leave]').click();
  check('then sends rleave for that room', JSON.stringify(last(t)) === JSON.stringify({ t: 'rleave', r: 'g1' }));

  // ---- Banned while connected
  t = signed();
  t.s.recv({ t: 'err', c: 'banned' });
  check('a ban shows the blocked screen and drops the saved login', view(t) === 'blocked' && t.w.localStorage.getItem('lobby.k') === null);

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
})();
