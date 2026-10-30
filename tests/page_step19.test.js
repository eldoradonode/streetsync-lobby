// Step 19 page tests: pinned message banner, reactions, polls. The server enforces every rule again; these check what the page shows and sends.
const { boot, check, ROOMS, okf, sub, typeInto, sleep, done } = require('./helpers16.js');

const enter = (extra = {}, joined = {}) => {
  const t = boot(); t.s.open(); t.s.recv(okf({ c: 20, g: true, ...extra })); t.s.recv({ t: 'rooms', r: ROOMS });
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat', ...joined });
  t.s.recv({ t: 'hist', m: [{ i: 1, h: 'Matt', b: 'Meetup on Saturday at 5pm', ts: Date.now(), r: 0 }, { i: 2, h: 'Eve', b: 'second message', ts: Date.now(), r: 0 }] });
  return t;
};
const sent = (t, type) => t.s.sent.filter(m => m.t === type);
const msg = (t, id) => t.d.querySelector('#log .m[data-id="' + id + '"]');
const actions = (t, id) => { msg(t, id).querySelector('button[aria-label^="More actions"]').click(); return [...msg(t, id).querySelectorAll('.actions button')].map(b => b.textContent); };
const PIN = { i: 1, h: 'Matt', b: 'Meetup on Saturday at 5pm', ts: 1, by: 'Host' };
const POLL = { id: 7, q: 'Best city?', o: ['Abuja', 'Lagos', 'Kano'], n: [2, 1, 1], t: 4, c: false, by: 'Host', ts: 1, y: -1 };

(async () => {
  // ---- pinned banner
  let t = enter();
  check('no banner when nothing is pinned', !t.$('pinbar'));
  t.s.recv({ t: 'x19', r: 'lobby', pin: PIN, poll: null, rx: {} });
  check('the banner shows the sender and the text', !!t.$('pinbar') && t.$('pinbar').textContent.includes('@Matt') && t.$('pinbar').textContent.includes('Meetup on Saturday'));
  check('a member has no Unpin button', !t.$('pinbar').querySelector('.pinx'));
  let target = msg(t, 1);
  t.$('pinbar').querySelector('.pinmain').click();
  check('tapping the banner scrolls to the message when it is on screen', target.classList.contains('flash'));
  t.s.recv({ t: 'pin', p: { ...PIN, i: 999, b: 'An old announcement that is no longer on screen' } });
  t.$('pinbar').querySelector('.pinmain').click();
  check('when the message is not on screen the whole text opens in place', !!t.$('pinbar').querySelector('.pinfull') && t.$('pinbar').querySelector('.pinfull').textContent.includes('old announcement'));
  t.s.recv({ t: 'pin', p: null });
  check('an unpin removes the banner for everyone', !t.$('pinbar'));
  t.s.recv({ t: 'pin', p: { ...PIN, b: '<img src=x onerror="window.pwned=1">' } });
  check('pinned text is shown as text, never as markup', !t.d.querySelector('#pinbar img') && !t.w.pwned);

  // permissions of the controls (the server checks again)
  check('a member sees no Pin action', !actions(t, 2).includes('Pin to room'));
  t = enter({ o: 'mod' });
  check('a moderator sees Pin to room', actions(t, 2).includes('Pin to room'));
  const pinBtn = [...msg(t, 2).querySelectorAll('.actions button')].find(b => b.textContent === 'Pin to room');
  pinBtn.click();
  check('pinning sends the message id', JSON.stringify(sent(t, 'pin')[0]) === JSON.stringify({ t: 'pin', i: 2 }));
  t.s.recv({ t: 'pin', p: PIN });
  t.$('pinbar').querySelector('.pinx').click();
  check('a moderator can unpin', sent(t, 'unpin').length === 1);
  t = enter({}, { k: 'public', o: 'Mike', y: true });
  check('the host of a community room sees Pin to room', actions(t, 2).includes('Pin to room'));

  // the pin does not follow the person to another room
  t.s.recv({ t: 'pin', p: PIN });
  t.s.recv({ t: 'joined', r: 'abuja', n: 'Abuja' });
  check('changing room clears the old pin', !t.$('pinbar'));

  // ---- reactions
  t = enter();
  check('no reaction chips on a plain message', !t.d.querySelector('.rxchip'));
  t.s.recv({ t: 'x19', r: 'lobby', pin: null, poll: null, rx: { 1: { c: { '\u{1F44D}': 2, '\u2764\uFE0F': 1 }, y: '\u{1F44D}' } } });
  const chips = [...msg(t, 1).querySelectorAll('.rxchip')];
  check('counts show on the message, mine highlighted', chips.length === 2 && chips[0].textContent.includes('2') && chips[0].classList.contains('on') && !chips[1].classList.contains('on'));
  chips[1].click();
  check('tapping a chip I have not used sends that reaction', JSON.stringify(sent(t, 'rx').pop()) === JSON.stringify({ t: 'rx', i: 1, em: '\u2764\uFE0F' }));
  msg(t, 1).querySelector('.rxbar .rxchip.on').click();
  check('tapping my own chip removes it', JSON.stringify(sent(t, 'rx').pop()) === JSON.stringify({ t: 'rx', i: 1, em: '' }));
  msg(t, 2).querySelector('button[aria-label="Add a reaction"]').click();
  const pick = [...msg(t, 2).querySelectorAll('.rxpick button')];
  check('the picker offers a small fixed set', pick.length === 6);
  pick[2].click();
  check('picking sends the reaction and closes the picker', JSON.stringify(sent(t, 'rx').pop()) === JSON.stringify({ t: 'rx', i: 2, em: '\u{1F602}' }) && !msg(t, 2).querySelector('.rxpick'));
  t.s.recv({ t: 'rx', i: 2, c: { '\u{1F602}': 3 }, h: 'Zed', y: '\u{1F602}' });
  const c2 = msg(t, 2).querySelector('.rxchip');
  check('a live update from someone else changes the count but not my choice', !!c2 && c2.textContent.includes('3') && !c2.classList.contains('on'));
  t.s.recv({ t: 'rx', i: 2, c: { '\u{1F602}': 4 }, h: 'Me', y: '\u{1F602}' });
  check('my own update marks my choice', msg(t, 2).querySelector('.rxchip').classList.contains('on'));
  t.s.recv({ t: 'del', i: 2 });
  check('a deleted message takes its reactions with it', !msg(t, 2));
  check('the reply and more buttons are still the ones the older tests expect', !!msg(t, 1).querySelector('.replybtn') && msg(t, 1).querySelector('.replybtn').textContent.trim() === 'Reply');

  // ---- polls
  t = enter();
  t.s.recv({ t: 'x19', r: 'lobby', pin: null, poll: POLL, rx: {} });
  let card = t.$('pollbox');
  check('a poll shows the question, options, percentages and totals', !!card && card.textContent.includes('Best city?') && card.textContent.includes('50%') && card.textContent.includes('25%') && card.textContent.includes('4 votes'));
  check('a member cannot start or close a poll', !card.textContent.includes('Start a poll') && !card.textContent.includes('Close poll'));
  const opts = () => [...t.d.querySelectorAll('#pollbox .popt')];
  opts()[1].click();
  check('voting sends the poll id and the option', JSON.stringify(sent(t, 'pvote').pop()) === JSON.stringify({ t: 'pvote', pid: 7, opt: 1 }));
  t.s.recv({ t: 'poll', p: { ...POLL, n: [2, 2, 1], t: 5, y: 1 } });
  check('results update live and my vote is marked', opts()[1].classList.contains('mine') && t.$('pollbox').textContent.includes('40%') && t.$('pollbox').textContent.includes('5 votes'));
  t.s.recv({ t: 'poll', p: { ...POLL, n: [2, 2, 1], t: 5, y: 1, c: true } });
  check('a closed poll can no longer be voted on', opts().every(b => b.disabled) && t.$('pollbox').textContent.includes('Closed'));
  t.s.recv({ t: 'err', c: 'poll_closed' });
  check('a refused vote is explained', t.$('log').textContent.includes('That poll is closed.'));

  // host: start, validate on the page, send
  t = enter({}, { k: 'public', o: 'Mike', y: true });
  check('the host can start a poll', t.$('pollbox') && t.$('pollbox').textContent.includes('Start a poll'));
  [...t.d.querySelectorAll('#pollbox button')].find(b => b.textContent.includes('Start a poll')).click();
  const form = () => t.d.querySelector('#pollbox .pollform');
  check('the form opens with a question and two options', !!form() && form().querySelectorAll('input').length === 3);
  const ins = () => [...form().querySelectorAll('input')];
  typeInto(t, ins()[0], 'Best city?');
  typeInto(t, ins()[1], 'Abuja');
  sub(t, form());
  check('one option is refused on the page and nothing is sent', sent(t, 'pcreate').length === 0 && !!form().querySelector('.pollerr'));
  typeInto(t, ins()[2], 'abuja');
  sub(t, form());
  check('duplicate options are refused on the page', sent(t, 'pcreate').length === 0);
  typeInto(t, ins()[2], 'Lagos');
  sub(t, form());
  check('a valid poll is sent with trimmed text', JSON.stringify(sent(t, 'pcreate')[0]) === JSON.stringify({ t: 'pcreate', b: 'Best city?', os: ['Abuja', 'Lagos'] }));
  t.s.recv({ t: 'poll', p: { ...POLL, o: ['Abuja', 'Lagos'], n: [0, 0], t: 0 } });
  check('the form closes when the poll arrives', !form());
  const closeBtn = () => [...t.d.querySelectorAll('#pollbox button')].find(b => /Close poll|Tap again/.test(b.textContent));
  check('the host sees Close poll', !!closeBtn());
  closeBtn().click();
  check('closing needs a second tap', sent(t, 'pclose').length === 0);
  closeBtn().click();
  check('the second tap closes the poll', JSON.stringify(sent(t, 'pclose')[0]) === JSON.stringify({ t: 'pclose', pid: 7 }));
  t.s.recv({ t: 'poll', p: { ...POLL, c: true } });
  check('after closing the host can start a new one', t.$('pollbox').textContent.includes('Start a poll'));
  t.s.recv({ t: 'poll', p: { ...POLL, q: '<b>bold</b>', c: false } });
  check('poll text is shown as text, never as markup', !t.d.querySelector('#pollbox b'));

  // moderators manage polls in official rooms too
  t = enter({ o: 'mod' });
  check('a moderator can start a poll in an official room', t.$('pollbox') && t.$('pollbox').textContent.includes('Start a poll'));

  // the poll does not follow the person to another room
  t.s.recv({ t: 'poll', p: POLL });
  t.s.recv({ t: 'joined', r: 'abuja', n: 'Abuja' });
  check('changing room clears the poll', !t.d.querySelector('#pollbox .pollcard'));

  // ---- bug fix: starting a poll. One request at a time, a clear answer, the button comes back.
  t = enter({}, { k: 'public', o: 'Mike', y: true });
  [...t.d.querySelectorAll('#pollbox button')].find(b => b.textContent.includes('Start a poll')).click();
  const pf = () => t.d.querySelector('#pollbox .pollform');
  const pins = () => [...pf().querySelectorAll('input')];
  const submitBtn = () => pf().querySelector('button[type="submit"]');
  typeInto(t, pins()[0], 'Best city?');
  typeInto(t, pins()[1], 'Abuja');
  typeInto(t, pins()[2], 'Lagos');
  sub(t, pf());
  sub(t, pf());
  sub(t, pf()); // three quick taps
  check('repeated taps send one request', sent(t, 'pcreate').length === 1);
  check('the button shows that it is working and is disabled', submitBtn().disabled && /Starting/.test(submitBtn().textContent));
  t.s.recv({ t: 'err', c: 'server_busy' });
  check('a failure is shown in the form and the button comes back', !!pf() && !submitBtn().disabled && /Start poll/.test(submitBtn().textContent) && pf().querySelector('.pollerr').textContent.includes('busy'));
  check('what was typed is kept for another try', pins()[0].value === 'Best city?' && pins()[2].value === 'Lagos');
  sub(t, pf());
  check('the next try sends a request again', sent(t, 'pcreate').length === 2);
  t.s.recv({ t: 'err', c: 'poll_failed' });
  check('a server fault is told as a server fault, not as busy', pf().querySelector('.pollerr').textContent.includes('server fault'));
  sub(t, pf());
  t.s.recv({ t: 'poll', p: { ...POLL, o: ['Abuja', 'Lagos'], n: [0, 0], t: 0 } });
  check('success closes the form and shows the poll', !pf() && !!t.d.querySelector('#pollbox .pollcard') && t.$('pollbox').textContent.includes('Best city?'));
  check('after success the next form starts empty', (() => { t.s.recv({ t: 'poll', p: { ...POLL, c: true } }); [...t.d.querySelectorAll('#pollbox button')].find(b => b.textContent.includes('Start a poll')).click(); return pins()[0].value === ''; })());

  // ---- bug fix: the page tells the server when it goes to the background and when it comes back
  t = enter();
  const vis = state => { Object.defineProperty(t.d, 'visibilityState', { value: state, configurable: true }); t.d.dispatchEvent(new t.w.Event('visibilitychange')); };
  vis('hidden');
  check('going to the background sends bg', sent(t, 'bg').length === 1);
  vis('visible');
  check('coming back sends fg', sent(t, 'fg').length === 1);
  done();
})();
