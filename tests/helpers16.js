// Shared by the step 16 page tests: boots the built page in jsdom with a fake WebSocket.
// Node's WebCrypto is given to the page, because jsdom has none, and an in-memory IndexedDB stand-in is added by the test.
const { JSDOM } = require('jsdom');
const fs = require('fs');
const path = require('path');
const { webcrypto } = require('crypto');
const html = fs.readFileSync(path.join(__dirname, 'build', 'page.html'), 'utf8');
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
      try { Object.defineProperty(w, 'crypto', { value: webcrypto, configurable: true }); } catch (e) { /* jsdom without it */ }
      if (opts.setup) opts.setup(w);
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
const sub = (t, el) => el.dispatchEvent(new t.w.Event('submit', { cancelable: true }));
const typeInto = (t, el, v) => { el.value = v; el.dispatchEvent(new t.w.Event('input', { bubbles: true })); };
const signed = (opts = {}, extra = {}) => { const t = boot(opts); t.s.open(); t.s.recv(okf({ c: 20, g: true, ...extra })); t.s.recv({ t: 'rooms', r: ROOMS }); return t; };
const sleep = ms => new Promise(r => setTimeout(r, ms));
const sha256hex = buf => require('crypto').createHash('sha256').update(buf).digest('hex');
// Lets the page finish its encryption setup for a new account: it asks for its key, makes one, registers it.
// Returns this account's public key and fingerprint, as the server would hold them.
async function handshake(t, handle) {
  await sleep(40);
  t.s.recv({ t: 'ekeyis', h: handle });
  await sleep(120);
  const reg = t.s.sent.filter(x => x.t === 'ekey').pop();
  const fp = sha256hex(Buffer.from(reg.pub, 'base64'));
  t.s.recv({ t: 'ekeyok', fp, ver: 1, new: true });
  await sleep(40);
  return { pub: reg.pub, fp, ver: 1 };
}
const counts = () => ({ pass, fail });
module.exports = { sleep, sha256hex, handshake, boot, check, ROOMS, okf, sub, typeInto, signed, counts, webcrypto, done: () => { console.log(`\n${pass} passed, ${fail} failed`); process.exit(fail ? 1 : 0); } };
