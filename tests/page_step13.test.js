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
  const sub = (t, el) => el.dispatchEvent(new t.w.Event('submit', { cancelable: true }));
  const typeInto = (t, el, v) => { el.value = v; el.dispatchEvent(new t.w.Event('input', { bubbles: true })); };
  const sentOf = (t, k) => t.s.sent.filter(x => x.t === k);
  const adminBoot = () => { const t = boot(); t.s.open(); t.s.recv(okf({ c: 5, o: 'admin' })); t.s.recv({ t: 'rooms', r: ROOMS }); return t; };

  // ---- bio and profile
  let t = boot(); t.s.open(); t.s.recv(okf({ c: 20, g: true })); t.s.recv({ t: 'rooms', r: ROOMS });
  t.$('tabProfile').click();
  check('opening Profile asks for my own profile too (for the join date)', t.s.sent.some(x => x.t === 'profile' && x.h === 'Me'));
  t.s.recv({ t: 'uprofile', h: 'Me', j: Date.UTC(2026, 8, 5), r: 0, p: 19 });
  check('the profile shows when I joined', t.$('myJoined').textContent.startsWith('Member since') && !t.$('myJoined').hidden, t.$('myJoined').textContent);
  check('with no bio it invites me to add one', t.$('bioText').textContent.includes('No bio yet') && t.$('bioEdit').textContent === 'Add a bio');
  t.$('bioEdit').click();
  check('Add a bio opens an editor', !!t.$('bioInput') && !!t.$('bioSave'));
  typeInto(t, t.$('bioInput'), 'x'.repeat(170));
  check('a bio over 160 characters cannot be saved', t.$('bioSave').disabled && t.$('bioLeft').textContent.startsWith('-10'), t.$('bioLeft').textContent);
  typeInto(t, t.$('bioInput'), '  Abuja   builder  ');
  check('the counter shows what is left', t.$('bioLeft').textContent === '141 characters left' || t.$('bioLeft').textContent.startsWith('14'), t.$('bioLeft').textContent);
  sub(t, t.$('bioForm'));
  check('saving sends the tidied bio', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'bio', b: 'Abuja builder' }));
  t.s.recv({ t: 'me', p: 19, r: 0, x: 300, s: 1, g: true, c: 20, bio: 'Abuja builder' });
  t.s.recv({ t: 'biook' });
  check('the saved bio shows and the editor closes', t.$('bioText').textContent === 'Abuja builder' && !t.$('bioForm') && t.$('bioEdit').textContent === 'Edit bio');
  check('Bio saved is confirmed', t.$('status').textContent === 'Bio saved.');
  t.$('bioEdit').click(); t.$('bioCancel').click();
  check('Cancel closes the editor without sending', !t.$('bioForm') && !t.s.sent.some(x => x.t === 'bio' && x.b === ''));
  check('the profile shows real stats only: tier, points, streak, credits', [...t.$('profile').querySelectorAll('.statrow dt')].map(x => x.textContent).join() === 'Tier,Points,Streak,Credits');
  t.$('profCredits').click();
  check('the credits number opens the wallet', !t.$('walletPage').hidden);

  // ---- rooms: directory and category
  t = boot(); t.s.open(); t.s.recv(okf({ c: 100, p: 400, r: 1 })); t.s.recv({ t: 'rooms', r: ROOMS });
  check('the hub asks for the full public room directory', t.s.sent.some(x => x.t === 'dir'));
  const empty = { s: 'quiet-aaaa', n: 'Quiet Corner', c: 'Music', g: ['Chill'], u: 0, h: 0, k: 'public', o: 'Zed' };
  t.s.recv({ t: 'dir', r: [empty] });
  check('a public room with nobody in it is on the hub list', !!t.d.querySelector('#communityList .room[data-slug="quiet-aaaa"]'));
  t.$('tabRooms').click();
  check('and on the Rooms page', !!t.d.querySelector('#rdList .room[data-slug="quiet-aaaa"]'));
  check('the room category becomes a filter', [...t.d.querySelectorAll('#rdChips .chipbtn')].some(b => b.textContent === 'Music'));
  const slugs = sel => [...t.d.querySelectorAll(sel + ' .room')].map(r => r.dataset.slug);
  check('no room appears in both Trending and the list', !slugs('#rdTrending').some(x => slugs('#rdList').includes(x)), slugs('#rdTrending').join() + ' | ' + slugs('#rdList').join());
  [...t.d.querySelectorAll('#rdChips .chipbtn')].find(b => b.textContent === 'Music').click();
  check('the category filter finds it', slugs('#rdList').includes('quiet-aaaa') && !t.$('rdTrending'));
  const many = Array.from({ length: 10 }, (_, i) => ({ s: 'c' + i + '-aaaa', n: 'Room ' + i, c: 'General', g: [], u: 0, h: 0, k: 'public', o: 'Zed' }));
  t.s.recv({ t: 'dir', r: many });
  t.$('tabHub').click();
  check('the hub shows 8 community rooms and a link to all of them', t.d.querySelectorAll('#communityList .room').length === 8 && /See all 10/.test(t.$('hubAllRooms').textContent), t.d.querySelectorAll('#communityList .room').length);
  t.$('hubAllRooms').click();
  check('the link opens the Rooms page', !t.$('roomsDir').hidden);
  t.$('rdCreate').click();
  const opts = [...t.$('crCat').options].map(o => o.value);
  check('the create dialog has a category with ten choices, General first', opts.length === 10 && opts[0] === 'General' && opts.includes('Sport') && t.$('crCat').value === 'General');
  t.$('crName').value = 'Match Day'; t.$('crCat').value = 'Sport';
  sub(t, t.$('createRoom'));
  check('the chosen category is sent with the room', JSON.stringify(sentOf(t, 'croom').pop()) === JSON.stringify({ t: 'croom', n: 'Match Day', k: 'public', g: '', f: 0, c: 'Sport' }), JSON.stringify(sentOf(t, 'croom').pop()));

  // ---- admin console
  t = adminBoot();
  check('the admin sees the Staff group with both links', !!t.$('staffNav') && !!t.$('tabAdmin') && !!t.$('tabMod'));
  t.$('tabAdmin').click();
  check('the console opens with its own look and name', !t.$('staffArea').hidden && t.$('shell').dataset.area === 'admin' && t.$('areaTag').textContent === 'ADMIN CONSOLE' && t.$('title').textContent === 'Admin console / Overview');
  check('opening it asks for the statistics', t.s.sent.some(x => x.t === 'astats'));
  check('the sidebar lists every admin section', ['overview', 'users', 'moderators', 'rooms', 'reports', 'log'].every(x => !!t.$('sec-' + x)) && !t.$('tabHub'));
  const days = k => Array.from({ length: 7 }, (_, i) => ({ d: '2026-10-' + (11 + i), n: k + i }));
  t.s.recv({ t: 'astats', on: 42, rep: 3, mu: 2, ki: 1, ro: 10, rc: 7, ir: 31, us: 1284, pw: 1190, n1: 12, n7: 65, a1: 311, mo: 3, ad: 1, cr: 48210, m1: 620, md: days(100), ud: days(1) });
  check('statistics show the numbers the server sent', t.$('stUsers').textContent === '1,284' && t.$('stOnline').textContent === '42' && t.$('stMsgs').textContent === '620' && t.$('stCredits').textContent === '48,210' && t.$('stRooms').textContent === '17' && t.$('stStaff').textContent === '4' && t.$('stReports').textContent.includes('3'));
  check('two seven day charts are drawn from the data', t.d.querySelectorAll('#chartMsgs .bar').length === 7 && t.d.querySelectorAll('#chartUsers .bar').length === 7);
  t.$('stReports').click();
  check('the Open reports card jumps to the reports queue', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'reports' }) && t.$('title').textContent.endsWith('Reports'));

  t.$('sec-users').click();
  check('the Users section asks for the newest accounts', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'ausers', h: '' }));
  t.s.recv({ t: 'ausers', q: '', u: [
    { h: 'ann', o: 'user', p: 10, r: 0, j: 1, g: true, on: true, c: 7, ls: 1 },
    { h: 'dan', o: 'user', p: 10, r: 0, j: 1, g: false, on: false, c: 0, ls: 1, mu: 12 },
    { h: 'kofi', o: 'mod', p: 0, r: 0, j: 1, g: true, on: false, c: 0, ls: 1 },
    { h: 'Boss', o: 'admin', p: 0, r: 0, j: 1, g: true, on: true, c: 0, ls: 1 },
    { h: 'Me', o: 'admin', p: 0, r: 0, j: 1, g: true, on: true, c: 0, ls: 1 },
  ] });
  const card = h => t.d.querySelector('#staffUsers [data-handle="' + h + '"]');
  const act = (h, a) => card(h).querySelector('[data-act="' + a + '"]');
  check('five people are listed', t.d.querySelectorAll('#staffUsers .ucard2').length === 5);
  check('a person sees mute, kick, clear bio, promote and give on a member', ['mute', 'kick', 'clearbio', 'promote', 'grant'].every(a => !!act('ann', a)) && !act('ann', 'demote'));
  check('a muted person has an Unmute button and shows the minutes', !!act('dan', 'unmute') && card('dan').textContent.includes('Muted 12 min'));
  check('a person without a password cannot be promoted, with the reason shown', act('dan', 'promote').disabled && card('dan').textContent.includes('need a password'));
  check('a moderator card offers Remove moderator, not Make moderator', !!act('kofi', 'demote') && !act('kofi', 'promote'));
  check('admin accounts have no actions at all', card('Boss').querySelectorAll('button').length === 0 && card('Boss').textContent.includes('cannot be acted on'));
  check('you cannot act on yourself', card('Me').querySelectorAll('button').length === 0 && card('Me').textContent.includes('This is you'));
  t.s.sent.length = 0;
  act('ann', 'kick').click();
  check('a risky action needs a second tap', t.s.sent.length === 0 && act('ann', 'kick').textContent === 'Tap again to confirm');
  act('ann', 'kick').click();
  check('the second tap sends a 10 minute kick', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'kick', h: 'ann', m: 10 }));
  const sel = card('ann').querySelector('select'); sel.value = '1440'; sel.dispatchEvent(new t.w.Event('change', { bubbles: true }));
  act('ann', 'mute').click(); act('ann', 'mute').click();
  check('the length can be chosen: 24 hours', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'mute', h: 'ann', m: 1440 }));
  act('dan', 'unmute').click();
  check('unmute is one tap', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'unmute', h: 'dan' }));
  act('ann', 'promote').click(); act('ann', 'promote').click();
  check('promote sends the role change after a confirm tap', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'promote', h: 'ann' }));
  act('kofi', 'demote').click(); act('kofi', 'demote').click();
  check('revoking sends demote after a confirm tap', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'demote', h: 'kofi' }));
  act('ann', 'clearbio').click(); act('ann', 'clearbio').click();
  check('clearing a bio sends the staff action', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'clearbio', h: 'ann' }));
  typeInto(t, t.$('staffQuery'), '@an'); sub(t, t.$('staffSearch'));
  check('searching sends the handle without the @', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'ausers', h: 'an' }));
  t.s.sent.length = 0;
  t.s.recv({ t: 'modok', a: 'kick', h: 'ann' });
  check('after an action the list is fetched again and the result is confirmed', t.s.sent.some(x => x.t === 'ausers') && t.$('status').textContent === 'Removed for now.', t.$('status').textContent);

  t.$('sec-moderators').click();
  check('the Moderators section asks for the staff list', t.s.sent.some(x => x.t === 'astaff'));
  t.s.recv({ t: 'astaff', s: [
    { h: 'Boss', o: 'admin', j: 1, ls: 1, on: true },
    { h: 'kofi', o: 'mod', j: 1, ls: 1, on: false, by: 'Boss', at: Date.UTC(2026, 8, 17) },
    { h: 'lola', o: 'mod', j: 1, ls: 1, on: true },
  ] });
  check('moderators and admins are listed apart', t.d.querySelectorAll('#modList .ucard2').length === 2 && t.d.querySelectorAll('#adminList .ucard2').length === 1);
  check('it shows who promoted each moderator', t.$('modList').textContent.includes('Made moderator by @Boss') && t.$('modList').textContent.includes('before records began'));
  t.s.sent.length = 0;
  typeInto(t, t.$('addModWho'), '!!'); sub(t, t.$('addMod'));
  check('a bad handle is refused in the page', t.s.sent.length === 0 && t.$('status').textContent.includes('full handle'));
  typeInto(t, t.$('addModWho'), '@Ann'); sub(t, t.$('addMod'));
  check('adding sends promote', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'promote', h: 'Ann' }));
  const rev = t.d.querySelector('#modList [data-handle="kofi"] [data-act="demote"]');
  rev.click(); check('removing needs a confirm tap', t.s.sent.length === 0 || t.s.sent[t.s.sent.length - 1].a !== 'demote');
  rev.click(); check('then sends demote', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'mod', a: 'demote', h: 'kofi' }));
  check('admins have no remove button', t.d.querySelectorAll('#adminList button').length === 0);

  t.$('sec-rooms').click();
  t.s.recv({ t: 'arooms', r: [
    { s: 'lobby', n: 'General Chat', c: 'General', u: 5, k: 'official', h: 2 },
    { s: 'n-1', n: 'Night Owls', c: 'Sport', u: 2, k: 'public', h: 1, o: 'ann', f: 2, la: Date.now() - 120000 },
    { s: 's-1', n: 'Secret', c: 'Tech', u: 0, k: 'private', h: 0, o: 'dan', w: true, la: Date.now() },
  ] });
  check('every room is listed with its kind', t.d.querySelectorAll('#staffRooms .ucard2').length === 3 && t.$('staffRooms').textContent.includes('Private') && t.$('staffRooms').textContent.includes('hosted by @dan'));
  check('official rooms have no close button', !t.d.querySelector('#staffRooms [data-slug="lobby"] [data-act="close"]'));
  const cl = t.d.querySelector('#staffRooms [data-slug="n-1"] [data-act="close"]');
  t.s.sent.length = 0; cl.click();
  check('closing a room needs a confirm tap', t.s.sent.length === 0);
  cl.click();
  check('then sends the close request with the room', JSON.stringify(t.s.sent.pop()) === JSON.stringify({ t: 'rclose', r: 'n-1' }));
  t.$('staffArea').querySelectorAll('.chipbtn').forEach(b => { if (b.textContent === 'Private') b.click(); });
  check('the room list can be filtered', t.d.querySelectorAll('#staffRooms .ucard2').length === 1);

  t.$('sec-log').click();
  t.s.recv({ t: 'alog', l: [{ a: 'Boss', x: 'promote', h: 'kofi', d: 'mod', ts: 5 }, { a: 'kofi', x: 'mute', h: 'dan', d: '25 minutes', ts: 4 }] });
  check('the audit log says who did what to whom', t.$('auditLog').textContent.includes('@Boss made a moderator @kofi') && t.$('auditLog').textContent.includes('@kofi muted @dan'));

  // ---- losing or never having the role
  t.s.recv({ t: 'me', p: 0, r: 0, x: 0, s: 0, g: true, o: 'mod', c: 5 });
  check('an admin who is demoted to moderator is moved out of the admin console at once', t.$('staffArea').hidden && !t.$('hub').hidden && !t.$('tabAdmin') && !!t.$('tabMod'));
  t.$('tabMod').click();
  check('a moderator opens the moderator dashboard instead', t.$('shell').dataset.area === 'mod' && !t.$('sec-moderators') && !t.$('sec-log'));
  t.s.recv({ t: 'astats', on: 3, rep: 0, mu: 0, ki: 0, ro: 10, rc: 1, ir: 2 });
  check('the moderator overview has no money or user totals', !t.$('stUsers') && !t.$('stCredits') && !t.$('chartMsgs') && !!t.$('permCard'));
  t.s.recv({ t: 'ausers', q: '', u: [{ h: 'ann', o: 'user', p: 1, r: 0, j: 1, g: true, on: false }, { h: 'zed', o: 'mod', p: 1, r: 0, j: 1, g: true, on: false }] });
  t.$('sec-users').click(); t.s.recv({ t: 'ausers', q: '', u: [{ h: 'ann', o: 'user', p: 1, r: 0, j: 1, g: true, on: false }, { h: 'zed', o: 'mod', p: 1, r: 0, j: 1, g: true, on: false }] });
  check('a moderator can mute, kick and clear a bio on a member', ['mute', 'kick', 'clearbio'].every(a => !!t.d.querySelector('#staffUsers [data-handle="ann"] [data-act="' + a + '"]')));
  check('but cannot promote, give credits, or act on another moderator', !t.d.querySelector('#staffUsers [data-act="promote"]') && !t.d.querySelector('#staffUsers [data-act="grant"]') && t.d.querySelector('#staffUsers [data-handle="zed"]').querySelectorAll('button').length === 0);
  t.s.recv({ t: 'me', p: 0, r: 0, x: 0, s: 0, g: true, c: 5 });
  check('a moderator who loses the role is moved out of the dashboard', t.$('staffArea').hidden && !t.$('tabMod') && !t.$('staffNav') && t.$('staffCard').hidden);

  // ---- an ordinary member never sees any of it
  t = boot(); t.s.open(); t.s.recv(okf({ c: 5 })); t.s.recv({ t: 'rooms', r: ROOMS });
  check('a member has no staff links, no admin sections and no staff card', !t.$('staffNav') && !t.$('tabAdmin') && !t.$('tabMod') && !t.$('sec-users') && t.$('staffArea').hidden && t.$('staffCard').hidden);
  check('a member never asked the server for staff data', !t.s.sent.some(x => ['astats', 'ausers', 'astaff', 'alog', 'arooms', 'reports'].includes(x.t)));
  t.s.recv({ t: 'err', c: 'forbidden' });
  check('a refusal from the server is shown to the person', t.$('status').textContent.includes('cannot do that'), t.$('status').textContent);

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
})();
