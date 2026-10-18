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
const HOSTILE = '<img src=x onerror="window.pwned=1">';

(async () => {
  let t = boot();
  t.s.open(); t.s.recv(okf({ c: 20, h: 'Mike' })); t.s.recv({ t: 'rooms', r: ROOMS });

  // ---- the Inbox tab and its badge
  check('there is an Inbox tab and no badge yet', !!t.$('tabInbox') && !t.$('inboxBadge'));
  t.s.recv({ t: 'ib', u: 2, q: 1 });
  check('unread messages and requests add up on the badge', t.$('inboxBadge') && t.$('inboxBadge').textContent === '3', t.$('inboxBadge') && t.$('inboxBadge').textContent);
  t.s.recv({ t: 'ib', u: 0, q: 0 });
  check('the badge goes away at zero', !t.$('inboxBadge'));
  t.s.recv({ t: 'ib', u: 2, q: 1 });
  t.$('tabInbox').click();
  check('opening the inbox asks for it', JSON.stringify(last(t)) === JSON.stringify({ t: 'inbox' }), JSON.stringify(last(t)));
  check('the inbox is shown with its title and the balance chip', !t.$('inbox').hidden && t.$('hub').hidden && t.$('profile').hidden && t.$('title').textContent === 'Messages' && !t.$('credChip').hidden);
  check('an empty inbox says how to start', t.$('chatList').textContent.includes('No messages yet'));
  const now = Date.now();
  t.s.recv({ t: 'inbox',
    c: [{ h: 'Ann', i: 5, m: false, b: 'hello', ts: now, u: 2 }, { h: 'Bob', i: 4, m: true, b: HOSTILE, ts: now - 1000, u: 0 }],
    f: ['Cat'], q: ['Dan'], p: ['Eve'], x: ['Fay'] });
  const rows = [...t.$('chatList').querySelectorAll('li')];
  check('one row per chat', rows.length === 2);
  check('the first chat shows the person, the last message and unread count', rows[0].textContent.includes('@Ann') && rows[0].textContent.includes('hello') && rows[0].querySelector('.badge').textContent === '2', rows[0].textContent);
  check('my own last message is marked, and has no unread badge', rows[1].textContent.includes('You: ') && !rows[1].querySelector('.badge'));
  check('a hostile message is shown as text, never run', !t.w.pwned && !t.$('chatList').querySelector('img') && rows[1].textContent.includes('<img'));
  // ---- the tab header
  const tabs = [...t.$('inboxTabs').querySelectorAll('button')].map(x => x.id);
  check('the tabs are Chats, Friends, Requests and Blocked', JSON.stringify(tabs) === JSON.stringify(['tabChats', 'tabFriends', 'tabRequests', 'tabBlocked']), tabs);
  check('Chats is open first and the other lists are not on the page', t.$('tabChats').getAttribute('aria-selected') === 'true' && !t.$('friendList') && !t.$('reqList') && !t.$('blockList'));
  check('the tabs show what is waiting', t.$('tabChats').querySelector('.badge').textContent === '2' && t.$('tabFriends').textContent.includes('1') && t.$('tabRequests').querySelector('.badge').textContent === '1' && t.$('tabBlocked').textContent.includes('1'));
  t.$('tabFriends').click();
  check('Friends shows friends', t.$('tabFriends').getAttribute('aria-selected') === 'true' && t.$('friendList').textContent.includes('@Cat') && !t.$('chatList'));
  t.d.querySelector('#friendList [aria-label^="More for"]').click();
  btn(t.$('friendList'), 'Remove friend').click();
  check('Remove friend is on the Friends tab and sends the right frame', JSON.stringify(last(t)) === JSON.stringify({ t: 'fr', a: 'remove', h: 'Cat' }), JSON.stringify(last(t)));
  t.d.querySelector('#friendList [aria-label^="More for"]').click();
  btn(t.$('friendList'), 'Block').click();
  check('and so is Block', JSON.stringify(last(t)) === JSON.stringify({ t: 'blk', a: 'add', h: 'Cat' }), JSON.stringify(last(t)));
  t.d.querySelector('#friendList .chatrow').click();
  check('tapping a friend opens their profile', JSON.stringify(last(t)) === JSON.stringify({ t: 'profile', h: 'Cat' }) && !t.$('userProfile').hidden && t.$('inbox').hidden, JSON.stringify(last(t)));
  t.$('back').click();
  check('back from a profile returns to the same tab', !t.$('inbox').hidden && !!t.$('friendList'));
  t.$('tabRequests').click();
  btn(t.$('reqList'), 'Accept').click();
  check('Accept sends the right frame', JSON.stringify(last(t)) === JSON.stringify({ t: 'fr', a: 'accept', h: 'Dan' }), JSON.stringify(last(t)));
  btn(t.$('reqList'), 'Decline').click();
  check('Decline sends the right frame', JSON.stringify(last(t)) === JSON.stringify({ t: 'fr', a: 'decline', h: 'Dan' }));
  btn(t.$('sentList'), 'Cancel').click();
  check('Cancel sends the right frame', JSON.stringify(last(t)) === JSON.stringify({ t: 'fr', a: 'cancel', h: 'Eve' }));
  t.$('tabBlocked').click();
  btn(t.$('blockList'), 'Unblock').click();
  check('Unblock sends the right frame', JSON.stringify(last(t)) === JSON.stringify({ t: 'blk', a: 'del', h: 'Fay' }));
  t.$('tabChats').click();
  check('and back to Chats', !!t.$('chatList') && !t.$('blockList'));

  // a message arriving while the inbox is open refreshes it
  const n0 = sentOf(t, 'inbox').length;
  t.s.recv({ t: 'dm', i: 9, f: 'Zed', w: 'Mike', b: 'psst', ts: now });
  check('a message from someone new adds to the unread count and refreshes the inbox', t.$('inboxBadge').textContent === String(3 + 1) && sentOf(t, 'inbox').length === n0 + 1, t.$('inboxBadge').textContent);
  check('and says so in the status line', t.$('status').textContent === 'New message from @Zed', t.$('status').textContent);

  // ---- find someone by handle
  t.$('dmWho').value = 'mike'; submitForm(t, 'dmFind');
  check('you can not open a chat with yourself', !t.$('inbox').hidden && t.$('status').textContent === 'That is you.', t.$('status').textContent);
  t.$('dmWho').value = 'a!'; submitForm(t, 'dmFind');
  check('a bad handle is refused in the page', !t.$('inbox').hidden && t.$('status').textContent.includes('3 to 16'));
  t.$('dmWho').value = '@Ann'; submitForm(t, 'dmFind');
  check('a good handle opens the conversation', JSON.stringify(last(t)) === JSON.stringify({ t: 'thread', h: 'Ann' }) && !t.$('thread').hidden && t.$('inbox').hidden, JSON.stringify(last(t)));
  check('the box is cleared', t.$('dmWho').value === '');

  // ---- the conversation
  check('the tabs hide and the back arrow shows', t.$('shell').classList.contains('inroom') && !t.$('back').hidden && t.$('title').textContent === '@Ann' && t.$('online').hidden);
  t.s.recv({ t: 'thread', h: 'Ann', s: 'none', c: 1, m: [
    { i: 5, f: 'Ann', w: 'Mike', b: 'hello', ts: now }, { i: 6, f: 'Mike', w: 'Ann', b: 'hi there', ts: now + 1 } ] });
  const bubbles = [...t.$('tlog').querySelectorAll('.dm')];
  check('both messages are shown, mine on the right', bubbles.length === 2 && !bubbles[0].classList.contains('mine') && bubbles[1].classList.contains('mine'));
  check('there is a Report link only on their message', !!btn(bubbles[0], 'Report') && !btn(bubbles[1], 'Report'));
  check('strangers see the price and the balance', t.$('costNote').textContent.includes('1 credit') && t.$('costNote').textContent.includes('20'), t.$('costNote').textContent);
  check('a stranger can be added as a friend', !!btn(t.$('tbar'), 'Add friend') && t.$('tbar').textContent.includes('Not friends yet'));
  btn(t.$('tbar'), 'Add friend').click();
  check('Add friend sends the right frame', JSON.stringify(last(t)) === JSON.stringify({ t: 'fr', a: 'add', h: 'Ann' }));
  const th0 = sentOf(t, 'thread').length;
  t.s.recv({ t: 'frok', a: 'add', h: 'Ann', s: 'sent' });
  check('the page confirms and reloads the conversation', t.$('status').textContent === 'Friend request sent to @Ann.' && sentOf(t, 'thread').length === th0 + 1, t.$('status').textContent);

  const bar = s => { t.s.recv({ t: 'thread', h: 'Ann', s, c: s === 'friend' ? 0 : 1, m: [] }); return t.$('tbar') ? t.$('tbar').textContent : null; };
  let x = bar('sent');
  check('a sent request can be cancelled', x.includes('Friend request sent') && !!btn(t.$('tbar'), 'Cancel'));
  btn(t.$('tbar'), 'Cancel').click();
  check('Cancel sends the right frame', JSON.stringify(last(t)) === JSON.stringify({ t: 'fr', a: 'cancel', h: 'Ann' }));
  x = bar('got');
  check('an incoming request can be accepted or declined', x.includes('@Ann wants to be friends') && !!btn(t.$('tbar'), 'Accept') && !!btn(t.$('tbar'), 'Decline'));
  btn(t.$('tbar'), 'Accept').click();
  check('Accept sends the right frame', JSON.stringify(last(t)) === JSON.stringify({ t: 'fr', a: 'accept', h: 'Ann' }));
  x = bar('friend');
  check('between friends the conversation shows only the person, no bar and no Remove friend or Block', x === null && !t.$('costNote') && !btn(t.$('thread'), 'Remove friend') && !btn(t.$('thread'), 'Block'));
  check('and the name is shown once, in the header, not again inside the conversation', !t.$('thread').querySelector('.who') && t.$('title').textContent === '@Ann');
  check('with their avatar next to it', !!t.$('titleUser').querySelector('svg'));
  t.$('titleUser').click();
  check('tapping the header opens their profile', JSON.stringify(last(t)) === JSON.stringify({ t: 'profile', h: 'Ann' }) && !t.$('userProfile').hidden, JSON.stringify(last(t)));
  t.$('back').click();
  check('back returns to the conversation', !t.$('thread').hidden && t.$('userProfile').hidden);
  x = bar('blocked');
  check('a blocked person hides the composer and offers Unblock', x.includes('You blocked @Ann') && !!btn(t.$('tbar'), 'Unblock') && t.$('tsend').hidden && !btn(t.$('tbar'), 'Block'));
  btn(t.$('tbar'), 'Unblock').click();
  check('Unblock sends the right frame', JSON.stringify(last(t)) === JSON.stringify({ t: 'blk', a: 'del', h: 'Ann' }));
  t.s.recv({ t: 'thread', h: 'Ann', s: 'none', c: 0, m: [] });
  check('a free reply says so', t.$('costNote').textContent.includes('free'), t.$('costNote').textContent);
  check('an empty conversation invites a hello', t.$('tlog').textContent.includes('Say hello to @Ann'));
  check('Block is not in the conversation any more', !btn(t.$('thread'), 'Block'));
  t.s.recv({ t: 'thread', h: 'Zed', s: 'friend', c: 0, m: [{ i: 1, f: 'Zed', w: 'Mike', b: 'wrong person', ts: now }] });
  check('a conversation frame for another person is ignored', !t.$('tlog').textContent.includes('wrong person'));

  // ---- sending and receiving
  t.s.recv({ t: 'thread', h: 'Ann', s: 'none', c: 1, m: [{ i: 5, f: 'Ann', w: 'Mike', b: 'hello', ts: now }] });
  const sent0 = t.s.sent.length;
  t.$('ttext').value = '   '; submitForm(t, 'tsend');
  check('an empty message sends nothing', t.s.sent.length === sent0);
  t.$('ttext').value = 'hello there'; submitForm(t, 'tsend');
  check('a message is sent to this person', JSON.stringify(last(t)) === JSON.stringify({ t: 'dm', h: 'Ann', b: 'hello there' }), JSON.stringify(last(t)));
  check('the box is cleared after sending', t.$('ttext').value === '');
  t.s.recv({ t: 'dm', i: 7, f: 'Mike', w: 'Ann', b: 'hello there', ts: now + 2 });
  t.s.recv({ t: 'dm', i: 7, f: 'Mike', w: 'Ann', b: 'hello there', ts: now + 2 });
  check('my message appears once, even if delivered twice', [...t.$('tlog').querySelectorAll('.dm')].filter(e => e.textContent.includes('hello there')).length === 1);
  t.s.recv({ t: 'dm', i: 8, f: 'Ann', w: 'Mike', b: HOSTILE, ts: now + 3 });
  check('their message appears in the open conversation as text', [...t.$('tlog').querySelectorAll('.dm')].length === 3 && !t.w.pwned && !t.$('tlog').querySelector('img'));
  check('and is marked read without a new fetch', JSON.stringify(last(t)) === JSON.stringify({ t: 'read', h: 'Ann' }), JSON.stringify(last(t)));
  t.s.recv({ t: 'dm', i: 10, f: 'Bob', w: 'Mike', b: 'someone else', ts: now + 4 });
  check('a message from another person only raises the unread count', [...t.$('tlog').querySelectorAll('.dm')].length === 3 && t.$('status').textContent === 'New message from @Bob', t.$('status').textContent);

  // ---- reporting
  btn([...t.$('tlog').querySelectorAll('.dm')][0], 'Report').click();
  const choices = [...t.$('tlog').querySelectorAll('.actions button')].map(b => b.textContent);
  check('Report asks why', JSON.stringify(choices) === JSON.stringify(['Spam', 'Abuse', 'Other']), choices);
  btn(t.$('tlog'), 'Abuse').click();
  check('the report names the message and the reason', JSON.stringify(last(t)) === JSON.stringify({ t: 'drep', i: 5, c: 'abuse' }), JSON.stringify(last(t)));
  check('the choices close', t.$('tlog').querySelectorAll('.actions').length === 0);

  // ---- errors are shown on this screen, not hidden in a room
  t.s.recv({ t: 'err', c: 'no_credits' });
  check('no credits is explained here', t.$('status').textContent.includes('1 credit') && !t.$('thread').hidden, t.$('status').textContent);
  t.s.recv({ t: 'err', c: 'blocked' });
  check('a blocked message is explained here', t.$('status').textContent === 'You cannot message this person.');
  t.s.recv({ t: 'err', c: 'muted', m: 5 });
  check('being muted is explained here', t.$('status').textContent.includes('muted for 5 more minutes'), t.$('status').textContent);

  // ---- back goes to the inbox
  const i0 = sentOf(t, 'inbox').length;
  t.$('back').click();
  check('back returns to the inbox and refreshes it', !t.$('inbox').hidden && t.$('thread').hidden && sentOf(t, 'inbox').length === i0 + 1);

  // ---- reconnecting inside a conversation
  t.$('dmWho').value = 'Ann'; submitForm(t, 'dmFind');
  t.s.recv({ t: 'thread', h: 'Ann', s: 'none', c: 1, m: [] });
  t.s.drop();
  await new Promise(r => setTimeout(r, 700));
  const s2 = t.sockets[1]; s2.open(); s2.recv(okf({ c: 20 })); s2.recv({ t: 'rooms', r: ROOMS });
  check('after a dropped connection the conversation is loaded again', !t.$('thread').hidden && s2.sent.some(x => x.t === 'thread' && x.h === 'Ann'), JSON.stringify(s2.sent));

  // ---- from a room: Message and Add friend on someone's message
  t = boot();
  t.s.open(); t.s.recv(okf({ c: 20 })); t.s.recv({ t: 'rooms', r: ROOMS });
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' });
  t.s.recv({ t: 'hist', m: [{ i: 1, h: 'Matt', b: 'hey all', ts: 1, r: 0 }, { i: 2, h: 'Me', b: 'my own', ts: 2, r: 0 }] });
  const msg = id => t.d.querySelector('#log .m[data-id="' + id + '"]');
  const more = el => el.querySelector('button[aria-label^="More actions"]');
  more(msg(2)).click();
  check('on my own message there is no Message or Add friend', !btn(msg(2), 'Message') && !btn(msg(2), 'Add friend'));
  more(msg(1)).click();
  btn(msg(1), 'Add friend').click();
  check('Add friend from a room message sends the request', JSON.stringify(last(t)) === JSON.stringify({ t: 'fr', a: 'add', h: 'Matt' }), JSON.stringify(last(t)));
  more(msg(1)).click();
  btn(msg(1), 'Message').click();
  check('Message from a room message opens the conversation', JSON.stringify(last(t)) === JSON.stringify({ t: 'thread', h: 'Matt' }) && !t.$('thread').hidden && t.$('chat').hidden, JSON.stringify(last(t)));
  t.$('back').click();
  check('back from there returns to the room', !t.$('chat').hidden && t.$('thread').hidden && t.$('inbox').hidden);
  check('the room is still there', !!msg(1));

  // ---- logging out forgets everything
  t.s.recv({ t: 'inbox', c: [{ h: 'Ann', i: 5, m: false, b: 'hello', ts: now, u: 2 }], f: ['Cat'], q: ['Dan'], p: [], x: [] });
  t.s.recv({ t: 'ib', u: 2, q: 1 });
  t.$('back').click(); t.s.recv({ t: 'left' });
  t.$('tabProfile').click();
  t.$('logout').click(); t.$('logoutAnyway') && t.$('logoutAnyway').click();
  check('logging out clears the inbox and the badge', !t.$('inboxBadge') && t.$('chatList').textContent.includes('No messages yet') && !t.$('reqList'));

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
})();
