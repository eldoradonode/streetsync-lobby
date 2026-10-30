// Step 17 page tests: mobile search, create room description, DM ticks, room leave and notices, homepage.
// Runs the built page in jsdom with a fake WebSocket. jsdom has no real keyboard or touch: see docs/STEP17-REPORT.md.
const { boot, check, ROOMS, okf, sub, typeInto, sleep, done } = require('./helpers16.js');

const signed = (opts = {}, extra = {}) => { const t = boot(opts); t.s.open(); t.s.recv(okf({ c: 20, g: true, ...extra })); t.s.recv({ t: 'rooms', r: ROOMS }); return t; };
const sentOf = (t, type) => t.s.sent.filter(m => m.t === type);
const vis = (t, state) => { Object.defineProperty(t.d, 'visibilityState', { configurable: true, get: () => state }); t.d.dispatchEvent(new t.w.Event('visibilitychange')); };
const roomRes = (t, q, r) => t.s.recv({ t: 'found', q, r });
const R = (s, n) => ({ s, n, c: 'Sport', g: [], u: 1, h: 1, k: 'public', o: 'zed', j: false });

(async () => {
  // ---- 1. Search on a phone: first letter capitalised, no Search button pressed
  let t = signed();
  t.$('tabRooms').click();
  const inp = t.$('rdSearch');
  check('the search box turns off capitalising and autocorrect, and asks for the search key', inp.getAttribute('autocapitalize') === 'off' && inp.getAttribute('autocorrect') === 'off' && inp.getAttribute('enterkeyhint') === 'search' && inp.getAttribute('inputmode') === 'search');
  typeInto(t, inp, 'A'); typeInto(t, inp, 'Ad'); typeInto(t, inp, 'Ada');
  check('nothing is sent while typing continues', sentOf(t, 'usearch').length === 0 && sentOf(t, 'find').length === 0);
  await sleep(320);
  check('one search goes out after the 250 ms pause, with the cleaned words', sentOf(t, 'usearch').length === 1 && sentOf(t, 'usearch')[0].n === 'ada' && sentOf(t, 'find').length === 1);
  t.s.recv({ t: 'ufound', q: 'ada', r: [{ h: 'Ada_l', r: 1 }, { h: 'adaeze', r: 2 }] });
  t.$('tabUsersRes').click();
  check('the answer to a capitalised query is shown (the mobile failure)', t.d.querySelectorAll('#userResults .urow').length === 2);
  check('the People tab shows the count', t.$('tabUsersRes').textContent.includes('(2)'));

  // ---- 2. Older answers never replace newer ones
  t = signed(); t.$('tabRooms').click();
  typeInto(t, t.$('rdSearch'), 'foot'); await sleep(320);
  typeInto(t, t.$('rdSearch'), 'football'); await sleep(320);
  check('two searches were sent', sentOf(t, 'find').length === 2 && sentOf(t, 'usearch').length === 2);
  roomRes(t, 'foot', [R('old1', 'Foot Old')]);
  check('the first answer is held back while a newer search is open', !t.$('rdList').textContent.includes('Foot Old'));
  roomRes(t, 'football', [R('new1', 'Football Fans')]);
  check('the newest answer is shown and the old one never was', t.$('rdList').textContent.includes('Football Fans') && !t.$('rdList').textContent.includes('Foot Old'));
  t.s.recv({ t: 'ufound', q: 'foot', r: [{ h: 'footman', r: 1 }] });
  t.$('tabUsersRes').click();
  check('a stale people answer is ignored', t.d.querySelectorAll('#userResults .urow').length === 0);
  t.s.recv({ t: 'ufound', q: 'football', r: [{ h: 'footballer', r: 1 }] });
  check('the current people answer is used', t.d.querySelectorAll('#userResults .urow').length === 1);

  // ---- 3. Counts and empty states
  t = signed(); t.$('tabRooms').click();
  typeInto(t, t.$('rdSearch'), 'q'); await sleep(320);
  t.$('tabUsersRes').click();
  check('one letter explains the minimum', !!t.$('peopleHint') && sentOf(t, 'usearch').length === 0);
  typeInto(t, t.$('rdSearch'), 'zzz'); await sleep(320);
  check('while waiting it says so', !!t.$('peopleBusy'));
  t.s.recv({ t: 'ufound', q: 'zzz', r: [] });
  check('people empty state names the search', t.$('userResults').textContent.includes('No one matches "zzz"'));
  roomRes(t, 'zzz', []);
  t.$('tabRoomsRes').click();
  check('rooms empty state and count', t.$('rdList').textContent.includes('No rooms match') && t.d.body.textContent.includes('Matching rooms (0)'));
  typeInto(t, t.$('rdSearch'), 'general'); await sleep(320);
  check('local rooms are counted as you type', t.d.body.textContent.includes('Matching rooms (1)'));

  // ---- 4. The server asks to slow down: no message, the search is repeated
  t = signed(); t.$('tabRooms').click();
  typeInto(t, t.$('rdSearch'), 'mar'); await sleep(320);
  const n0 = sentOf(t, 'usearch').length;
  t.s.recv({ t: 'err', c: 'slow_down' });
  check('slow down is not shown while searching', t.$('status').hidden || !t.$('status').textContent.includes('Slow'));
  await sleep(400);
  check('and the search is asked again', sentOf(t, 'usearch').length === n0 + 1);
  typeInto(t, t.$('rdSearch'), 'mario');
  sub(t, t.$('rdSearch').form);
  check('Enter searches at once', sentOf(t, 'usearch').pop().n === 'mario');

  // ---- 5. Create room description
  t = signed(); t.$('tabRooms').click(); t.$('rdCreate').click();
  const ta = t.$('crDesc');
  check('the description field is a visible textarea with a counter at 0/200', !!ta && ta.tagName === 'TEXTAREA' && !ta.closest('[hidden]') && t.$('crDescLeft').textContent === '0/200');
  check('the counter sits inside the field wrapper (bottom right by CSS)', ta.parentElement.classList.contains('descwrap') && ta.parentElement.contains(t.$('crDescLeft')));
  check('the page ships styles for it', t.d.documentElement.innerHTML.includes('#crDesc'));
  typeInto(t, ta, 'a'.repeat(250));
  check('250 typed characters are cut to 200', ta.value.length === 200 && t.$('crDescLeft').textContent === '200/200');
  ta.value = '\u{1F600}'.repeat(150); ta.dispatchEvent(new t.w.Event('input', { bubbles: true }));
  check('an emoji counts as one character', t.$('crDescLeft').textContent === '150/200');
  typeInto(t, ta, ''); typeInto(t, t.$('crName'), 'No Desc Room'); sub(t, t.$('createRoom'));
  let cr = sentOf(t, 'croom').pop();
  check('an empty description is accepted and sent empty', cr && cr.n === 'No Desc Room' && (cr.d === '' || cr.d === undefined));
  t = signed(); t.$('tabRooms').click(); t.$('rdCreate').click();
  typeInto(t, t.$('crDesc'), 'Late night gist'); typeInto(t, t.$('crName'), 'Gist Room'); sub(t, t.$('createRoom'));
  cr = sentOf(t, 'croom').pop();
  check('a valid description is sent with the room', cr && cr.d === 'Late night gist' && cr.n === 'Gist Room');

  // ---- 6. Homepage
  t = signed();
  check('no category grid on the home page', !t.$('hubCats') && !t.$('hub').textContent.includes('Browse by category'));
  check('official rooms are listed', t.$('roomList').querySelectorAll('.room').length === ROOMS.length);

  // ---- 7. Ticks on direct messages
  t = signed(); t.$('tabInbox').click();
  t.$('dmWho').value = '@Ann'; sub(t, t.$('dmFind'));
  check('opening a visible conversation asks for it normally (it is read)', JSON.stringify(sentOf(t, 'thread').pop()) === JSON.stringify({ t: 'thread', h: 'Ann' }));
  const now = Date.now();
  t.s.recv({ t: 'thread', h: 'Ann', s: 'none', c: 1, m: [
    { i: 5, f: 'Ann', w: 'Me', b: 'hello', ts: now, st: 3 },
    { i: 6, f: 'Me', w: 'Ann', b: 'one', ts: now + 1, st: 1 },
    { i: 7, f: 'Me', w: 'Ann', b: 'two', ts: now + 2, st: 2 },
    { i: 8, f: 'Me', w: 'Ann', b: 'three', ts: now + 3, st: 3 } ] });
  await sleep(60);
  const st = () => [...t.d.querySelectorAll('#tlog .dm')].map(el => { const k = el.querySelector('.ticks'); return k ? k.dataset.st : '-'; }).join();
  check('ticks only on my own messages: sent, delivered, read', st() === '-,1,2,3', st());
  check('read ticks use the read colour class, others stay grey', t.d.querySelector('[data-id="8"] .ticks').classList.contains('t3') && !t.d.querySelector('[data-id="7"] .ticks').classList.contains('t3'));
  t.s.recv({ t: 'rcpt', h: 'Ann', s: 2, ids: [6] });
  check('a delivered receipt updates live', st() === '-,2,2,3', st());
  t.s.recv({ t: 'rcpt', h: 'Ann', s: 3, u: 7 });
  check('a read receipt covers everything up to its id', st() === '-,3,3,3', st());
  t.s.recv({ t: 'rcpt', h: 'Ann', s: 2, ids: [6, 7] });
  check('a late delivered receipt cannot lower a status', st() === '-,3,3,3', st());
  t.s.recv({ t: 'rcpt', h: 'Zed', s: 3, u: 99 });
  check('receipts for another conversation change nothing here', st() === '-,3,3,3');
  // a message I send shows one tick, and a receipt that came first is not lost
  t.s.recv({ t: 'rcpt', h: 'Ann', s: 2, ids: [20] });
  t.s.recv({ t: 'dm', i: 20, f: 'Me', w: 'Ann', b: 'sent now', ts: now + 9, st: 1 });
  await sleep(60);
  check('a receipt that beat its message is applied', st().endsWith(',2'), st());
  t.s.recv({ t: 'dm', i: 21, f: 'Me', w: 'Ann', b: 'fresh', ts: now + 10, st: 1 });
  await sleep(60);
  check('a fresh message starts with one tick', st().endsWith(',1'), st());

  // incoming: visible conversation reads, hidden page only confirms delivery
  const reads0 = sentOf(t, 'read').length;
  t.s.recv({ t: 'dm', i: 30, f: 'Ann', w: 'Me', b: 'hey', ts: now + 11 });
  await sleep(60);
  check('a message in the open, visible conversation is marked read', sentOf(t, 'read').length === reads0 + 1 && sentOf(t, 'dack').length === 0);
  vis(t, 'hidden');
  t.s.recv({ t: 'dm', i: 31, f: 'Ann', w: 'Me', b: 'still there?', ts: now + 12 });
  await sleep(60);
  const ack = sentOf(t, 'dack').pop();
  check('while the page is hidden it confirms delivery and does not read', ack && ack.h === 'Ann' && ack.ids[0] === 31 && sentOf(t, 'read').length === reads0 + 1);
  check('the ack carries only ids', Object.keys(ack).sort().join() === 'h,ids,t');
  vis(t, 'visible');
  check('coming back to the conversation reads it', sentOf(t, 'read').length === reads0 + 2);

  // opened while hidden
  t = signed(); vis(t, 'hidden'); t.$('tabInbox').click();
  t.$('dmWho').value = '@Ann'; sub(t, t.$('dmFind'));
  check('a hidden page opens the conversation without marking it read', sentOf(t, 'thread').pop().nr === true);
  t.s.recv({ t: 'thread', h: 'Ann', s: 'none', c: 1, m: [{ i: 5, f: 'Ann', w: 'Me', b: 'x', ts: now, st: 1 }, { i: 6, f: 'Ann', w: 'Me', b: 'y', ts: now, st: 2 }] });
  await sleep(60);
  check('and confirms delivery only of what was undelivered', JSON.stringify(sentOf(t, 'dack').pop()) === JSON.stringify({ t: 'dack', h: 'Ann', ids: [5] }));
  // a message in another view, not the conversation: delivered but never read
  t = signed();
  t.s.recv({ t: 'dm', i: 40, f: 'Bo', w: 'Me', b: 'psst', ts: now });
  await sleep(60);
  check('a message while on another screen is delivered, not read', sentOf(t, 'dack').length === 1 && sentOf(t, 'read').length === 0);

  // ---- 8. Rooms: joined pill, Leave room, notices
  const CROOMS = ROOMS.concat([{ s: 'c1', n: 'Night Owls', c: 'Social', g: [], u: 2, h: 1, k: 'public', o: 'zed', j: true, y: true }, { s: 'c2', n: 'Day Owls', c: 'Social', g: [], u: 1, h: 1, k: 'public', o: 'zed', j: false }]);
  t = boot(); t.s.open(); t.s.recv(okf({ c: 20, g: true })); t.s.recv({ t: 'rooms', r: CROOMS });
  check('joined rooms say Joined, others say Join', t.d.querySelector('#communityList [data-slug=c1] .joinpill').textContent.includes('Joined') && t.d.querySelector('#communityList [data-slug=c2] .joinpill').textContent === 'Join');
  t.d.querySelector('#communityList [data-slug=c2]').click();
  check('Join asks the server', JSON.stringify(sentOf(t, 'join').pop().r) === '"c2"');
  check('while the server has not answered the tile says Joining', t.d.querySelector('[data-slug=c2] .joinpill').textContent.includes('Joining'));
  t.d.querySelector('#communityList [data-slug=c2]').click();
  check('a second tap while waiting sends nothing more', sentOf(t, 'join').length === 1);
  t.s.recv({ t: 'joined', r: 'c2', n: 'Day Owls', k: 'public', o: 'zed', j: true });
  t.s.recv({ t: 'hist', m: [] });
  check('the header has Leave room in a room', !t.$('leaveRoomBtn').hidden);
  check('after the server confirms, the tile shows Joined', t.d.querySelector('[data-slug=c2] .joinpill').textContent.includes('Joined'));
  check('and the room header says Joined', t.$('roomState').textContent.includes('Joined'));
  t.s.recv({ t: 'rs', a: 'j', h: 'bob' });
  t.s.recv({ t: 'rs', a: 'l', h: 'bob' });
  const notes = [...t.d.querySelectorAll('#log .m.sys')].map(e => e.textContent);
  check('join and leave notices appear as system lines', notes.includes('@bob joined the room') && notes.includes('@bob left the room'), notes.join('|'));
  t.$('leaveRoomBtn').click();
  check('Leave room on a joined community room ends the membership', JSON.stringify(sentOf(t, 'rleave').pop()) === JSON.stringify({ t: 'rleave', r: 'c2' }));
  t.s.recv({ t: 'left' }); t.s.recv({ t: 'rleft', s: 'c2', n: 'Day Owls' });
  check('and the tile goes back to Join', t.d.querySelector('[data-slug=c2] .joinpill').textContent !== 'Joined' && !t.$('hub').hidden);

  t = boot(); t.s.open(); t.s.recv(okf({ c: 20, g: true })); t.s.recv({ t: 'rooms', r: CROOMS });
  t.d.querySelector('#roomList [data-slug=lobby]').click();
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' });
  t.$('leaveRoomBtn').click();
  check('Leave room in an official room leaves at once (explicit)', JSON.stringify(sentOf(t, 'leave').pop()) === JSON.stringify({ t: 'leave', a: 'x' }));
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' });
  t.$('back').click();
  check('the back arrow is the quiet leave (grace period on the server)', JSON.stringify(sentOf(t, 'leave').pop()) === JSON.stringify({ t: 'leave' }));

  done();
})();
