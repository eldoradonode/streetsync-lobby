const { JSDOM } = require('jsdom');
const fs = require('fs');
const html = fs.readFileSync(require('path').join(__dirname, 'build', 'page.html'), 'utf8');
let pass = 0, fail = 0;
const check = (name, cond, extra) => { if (cond) { pass++; console.log('PASS', name); } else { fail++; console.log('FAIL', name, extra || ''); } };

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
    w.Element.prototype.scrollIntoView = function () { this.scrolled = true; };
    w.WebSocket = class {
      constructor() { this.readyState = 0; this.sent = []; sockets.push(this); }
      send(d) { this.sent.push(JSON.parse(d)); }
      close() { this.readyState = 3; }
      open() { this.readyState = 1; this.onopen && this.onopen(); flush(); }
      recv(o) { this.onmessage && this.onmessage({ data: JSON.stringify(o) }); flush(); }
    };
  }
});
if (dom.window.__lobbyFlush) dom.window.__lobbyFlush();
const w = dom.window, d = w.document, $ = id => d.getElementById(id);
const s = sockets[0]; s.open();
s.recv({ t: 'ok', h: 'Eldorado', k: 'tok', p: 5, r: 0, x: 300, s: 1, g: true });
s.recv({ t: 'joined', r: 'lobby', n: 'General Chat' });
s.recv({ t: 'hist', m: [
  { i: 1, h: 'Matt', b: 'Now this is clean guys', ts: 1, r: 0 },
  { i: 2, h: 'James', b: 'Oh nice', ts: 2, r: 0, q: 1, qh: 'Matt', qb: 'Now this is clean guys' },
  { i: 3, h: 'Matt', b: 'thanks for the earlier answer', ts: 3, r: 1, q: 90, qh: 'Eldorado', qb: 'a message of mine <b>bold</b>' },
  { i: 4, h: 'Eldorado', b: 'replying to James', ts: 4, r: 0, q: 2, qh: 'James', qb: 'Oh nice' },
  { i: 5, h: 'Peter', b: 'x'.repeat(200), ts: 5, r: 0 },
]});
const msgs = () => [...d.querySelectorAll('#log .m')].filter(m => m.dataset.id);
const byId = id => msgs().find(m => m.dataset.id === String(id));

check('replies show a quote block, plain messages do not', byId(2).querySelector('.quote') && !byId(1).querySelector('.quote'));
check('quote shows the sender and the snippet', byId(2).querySelector('.quote b').textContent === '@Matt' && byId(2).querySelector('.quote span').textContent === 'Now this is clean guys');
check('the quote sits between the header and the text', [...byId(2).children].map(c => c.className).join(',') === 'head,quote,');
check('html inside a quote stays text', !byId(3).querySelector('.quote b b') && byId(3).querySelector('.quote span').textContent.includes('<b>bold</b>'));
check('a reply to my message is highlighted like a mention', byId(3).classList.contains('mention'));
check('my own reply is not highlighted', !byId(4).classList.contains('mention'));
check('a reply to someone else is not highlighted', !byId(2).classList.contains('mention'));
check('every message has a reply button', msgs().every(m => m.querySelector('.replybtn')));
check('the reply bar starts hidden', $('replybar').hidden);

// reply button
byId(1).querySelector('.replybtn').click();
check('reply button opens the bar', !$('replybar').hidden && $('replyWho').textContent === 'Replying to @Matt' && $('replySnip').textContent === 'Now this is clean guys');
check('the input is focused for typing', d.activeElement === $('text'));
$('text').value = 'agreed';
$('send').dispatchEvent(new w.Event('submit', { cancelable: true }));
check('sending a reply includes the message id', JSON.stringify(s.sent.pop()) === JSON.stringify({ t: 'm', b: 'agreed', q: 1 }));
check('the bar closes after sending', $('replybar').hidden);
$('text').value = 'just a normal one';
$('send').dispatchEvent(new w.Event('submit', { cancelable: true }));
check('a normal message after that has no reply id', JSON.stringify(s.sent.pop()) === JSON.stringify({ t: 'm', b: 'just a normal one' }));

// long snippet in the bar
byId(5).querySelector('.replybtn').click();
check('long messages are cut to 80 characters in the bar', $('replySnip').textContent === 'x'.repeat(80) + '\u2026', $('replySnip').textContent.length);

// cancel
$('replyCancel').click();
check('cancel closes the bar', $('replybar').hidden);
byId(1).querySelector('.replybtn').click();
$('text').dispatchEvent(new w.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
check('Escape closes the bar', $('replybar').hidden);

// swipe
const touch = (el, type, x, y) => { const e = new w.Event(type); e.touches = [{ clientX: x, clientY: y }]; el.dispatchEvent(e); };
const m2 = byId(2);
touch(m2, 'touchstart', 20, 100); touch(m2, 'touchmove', 60, 102); touch(m2, 'touchmove', 100, 104);
check('the message follows the finger', m2.parentElement.style.transform === 'translateX(80px)', m2.parentElement.style.transform);
check('the reply arrow fades in while sliding', parseFloat(m2.parentElement.style.getPropertyValue('--sw')) > 0.9, m2.parentElement.style.getPropertyValue('--sw'));
touch(m2, 'touchend', 100, 104);
check('a long slide opens the reply bar', !$('replybar').hidden && $('replyWho').textContent === 'Replying to @James');
check('the message slides back', m2.parentElement.style.transform === '');
clearReplyViaCancel();
function clearReplyViaCancel() { $('replyCancel').click(); }

const m1 = byId(1);
touch(m1, 'touchstart', 20, 100); touch(m1, 'touchmove', 50, 101); touch(m1, 'touchend', 50, 101);
check('a short slide does nothing', $('replybar').hidden);
touch(m1, 'touchstart', 20, 100); touch(m1, 'touchmove', 40, 160); touch(m1, 'touchmove', 120, 200); touch(m1, 'touchend', 120, 200);
check('scrolling downward never triggers a reply', $('replybar').hidden && m1.parentElement.style.transform === '');
touch(m1, 'touchstart', 100, 100); touch(m1, 'touchmove', 20, 100); touch(m1, 'touchend', 20, 100);
check('sliding left does nothing', $('replybar').hidden);

// jump to the original
byId(2).querySelector('.quote').click();
check('tapping a quote scrolls to the original and flashes it', byId(1).scrolled === true && byId(1).classList.contains('flash'));
byId(3).querySelector('.quote').click();
check('quote of a message not on screen gives a notice', [...d.querySelectorAll('#log .m.sys')].pop().textContent === 'That message is no longer on your screen.');

// server says too old
byId(1).querySelector('.replybtn').click();
s.recv({ t: 'err', c: 'reply_gone' });
check('reply_gone closes the bar and explains', $('replybar').hidden && [...d.querySelectorAll('#log .m.sys')].pop().textContent === 'That message is too old to reply to.');

// live reply to me while the tab is hidden
Object.defineProperty(d, 'visibilityState', { configurable: true, get: () => 'hidden' });
s.recv({ t: 'm', i: 6, h: 'James', b: 'good point', ts: 6, r: 0, q: 4, qh: 'Eldorado', qb: 'replying to James' });
check('a live reply to me counts in the tab title', d.title === '(1) STREETSYNC' && byId(6).classList.contains('mention'), d.title);

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail ? 1 : 0);
