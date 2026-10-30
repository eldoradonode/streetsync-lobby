// Step 18 page tests: presence dot, profile photo avatars with fallback, photo editor visibility, room notice setting.
const { boot, check, ROOMS, okf, sub, sleep, done } = require('./helpers16.js');
const signed = () => { const t = boot(); t.s.open(); t.s.recv(okf({ c: 20, g: true })); t.s.recv({ t: 'rooms', r: ROOMS }); return t; };
const sentOf = (t, type) => t.s.sent.filter(m => m.t === type);

(async () => {
  let t = signed();
  t.$('tabInbox').click();
  t.$('dmWho').value = '@Ann'; sub(t, t.$('dmFind'));
  t.s.recv({ t: 'pres', h: 'Ann', o: '0' });
  t.s.recv({ t: 'thread', h: 'Ann', s: 'none', c: 1, m: [] });
  let p = t.$('presence');
  check('an offline person shows a grey dot and Offline under the handle', !!p && p.textContent.trim() === 'Offline' && p.classList.contains('off') && !!p.querySelector('.pdot'));
  check('the status sits inside the header, after the handle', t.$('title').parentElement.contains(p));
  t.s.recv({ t: 'pres', h: 'Ann', o: '1' });
  p = t.$('presence');
  check('a live update turns it to a green dot and Online', p.textContent.trim() === 'Online' && p.classList.contains('on'));
  t.s.recv({ t: 'pres', h: 'someone_else', o: '1' });
  check('another person\'s status does not change this header', t.$('presence').textContent.trim() === 'Online');
  t.s.drop();
  await sleep(20);
  check('when the connection drops the dot is not left green', !t.$('presence') || t.$('presence').textContent.trim() !== 'Online');

  // closing the conversation tells the server to stop sending presence
  t = signed(); t.$('tabInbox').click();
  t.$('dmWho').value = '@Ann'; sub(t, t.$('dmFind'));
  t.s.recv({ t: 'thread', h: 'Ann', s: 'none', c: 1, m: [] });
  t.$('back').click();
  check('leaving the conversation sends unwatch', sentOf(t, 'unwatch').length === 1);

  // photo avatars: a version in the look shows the picture, never the full size in a list, and falls back when it fails
  t = signed();
  t.s.recv({ t: 'hist', m: [{ i: 1, h: 'Zed', b: 'hi', ts: Date.now(), av: 'f3~4' }] });
  await sleep(30);
  const img = t.d.querySelector('img.avphoto');
  check('a message from a person with a photo shows their picture', !!img);
  check('it asks for the small picture, with the version for caching, and loads lazily', img && img.getAttribute('src') === '/photo/Zed?v=4' && img.getAttribute('loading') === 'lazy');
  if (img) img.dispatchEvent(new t.w.Event('error'));
  await sleep(30);
  check('if the picture cannot load the illustrated avatar is shown instead', !t.d.querySelector('img.avphoto') && !!t.d.querySelector('svg[aria-label="Avatar of Zed"]') && !t.d.querySelector('svg[aria-label="Avatar of Zed"]').hasAttribute('hidden'));

  // the photo editor only exists when the server can store photos
  t = signed(); t.$('tabProfile') && t.$('tabProfile').click();
  await sleep(30);
  check('with photos off nothing is offered', !t.$('photoEditor') && !t.$('photoChoose'));
  done();
})();
