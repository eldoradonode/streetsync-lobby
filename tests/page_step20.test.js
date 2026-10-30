// Step 20 page tests: favourites, recent rooms, directory order, the member list. The server decides everything; these
// check what the page shows and sends.
const { boot, check, ROOMS, okf, sub, typeInto, sleep, done } = require('./helpers16.js');

const enter = (extra = {}, joined = {}) => {
  const t = boot(); t.s.open(); t.s.recv(okf({ c: 20, g: true, ...extra })); t.s.recv({ t: 'rooms', r: ROOMS });
  t.s.recv({ t: 'joined', r: 'lobby', n: 'General Chat', ...joined });
  return t;
};
const sent = (t, type) => t.s.sent.filter(m => m.t === type);
const R = (s, o = {}) => ({ s, n: 'Room ' + s, c: 'Community', g: [], u: 1, h: 0, k: 'public', o: 'Host', ...o });
const slugsIn = (t, id) => [...t.d.querySelectorAll('#' + id + ' .room')].map(b => b.dataset.slug);

(async () => {
  // ---- favourites and recent rooms
  let t = enter();
  check('no Favourites section when there are none', !t.$('rdFavs'));
  t.s.recv({ t: 'mr', fs: ['a'], f: [R('a')], v: [R('b'), R('a')] });
  check('favourites are shown in their own section', slugsIn(t, 'rdFavs').join() === 'a');
  check('recent rooms are shown in their own section, newest first', slugsIn(t, 'rdRecent').join() === 'b,a');
  const heart = slug => t.d.querySelector('#rdFavs [data-fav="' + slug + '"]');
  check('a favourite room shows a pressed star with a clear name', heart('a').getAttribute('aria-pressed') === 'true' && /Remove Room a from favourites/.test(heart('a').getAttribute('aria-label')));
  heart('a').click();
  check('tapping the star of a favourite sends a removal', JSON.stringify(sent(t, 'fav').pop()) === JSON.stringify({ t: 'fav', r: 'a', fav: false }));
  t.d.querySelector('#rdRecent [data-fav="b"]').click();
  check('tapping the star of another room sends an add', JSON.stringify(sent(t, 'fav').pop()) === JSON.stringify({ t: 'fav', r: 'b', fav: true }));
  t.s.recv({ t: 'mr', fs: ['b'], f: [R('b')], v: [R('b')] });
  check('the lists follow what the server says', slugsIn(t, 'rdFavs').join() === 'b' && heart('b').getAttribute('aria-pressed') === 'true');
  t.s.recv({ t: 'err', c: 'fav_full' });
  check('a full list is explained', t.$('log').textContent.includes('50 favourite rooms') || document_text(t).includes('50 favourite rooms'));
  function document_text(tt) { return tt.d.body.textContent; }
  typeInto(t, t.$('rdSearch'), 'gist');
  check('while searching the Favourites and Recently visited sections step aside', !t.$('rdFavs') && !t.$('rdRecent'));

  // ---- directory order
  t = enter();
  t.s.recv({ t: 'dir', r: [R('c', { u: 0 }), R('a', { u: 0 }), R('b', { u: 0 })], o: 'new' });
  check('rooms appear in exactly the order the server sent', slugsIn(t, 'rdList').filter(s => /^[abc]$/.test(s)).join() === 'c,a,b', slugsIn(t, 'rdList'));
  const chips = () => [...t.d.querySelectorAll('#rdSort [data-sort]')];
  check('three sort choices: most active, recently active, newest', chips().map(b => b.textContent).join() === 'Most active,Recently active,Newest');
  check('the default is most active and the choice is announced as pressed', chips()[0].getAttribute('aria-pressed') === 'true' && chips()[1].getAttribute('aria-pressed') === 'false');
  chips()[2].click();
  check('choosing Newest asks the server for that order', JSON.stringify(sent(t, 'dir').pop()) === JSON.stringify({ t: 'dir', sort: 'new' }));
  check('the choice shows as pressed', chips()[2].getAttribute('aria-pressed') === 'true');
  t.s.recv({ t: 'dir', r: [R('b', { u: 5, h: 5 }), R('c', { u: 0 }), R('a', { u: 3, h: 3 })], o: 'new' });
  check('the new order is shown as sent, not re-sorted by the page', slugsIn(t, 'rdList').filter(s => /^[abc]$/.test(s)).join() === 'b,c,a', slugsIn(t, 'rdList'));
  check('with a chosen order the busiest rooms are not pulled out into Trending', !t.$('rdTrending'));

  // ---- the member list
  t = enter({}, { k: 'public', o: 'Host' });
  check('the list is closed at first', !t.$('members'));
  t.$('roomPeople').click();
  check('People opens the list and asks the server', !!t.$('members') && JSON.stringify(sent(t, 'members')[0]) === JSON.stringify({ t: 'members', off: 0 }));
  t.s.recv({ t: 'ml', r: 'lobby', off: 0, tot: 4, on: 2, more: false, m: [
    { h: 'Host', s: 'on', x: true }, { h: 'Amy', s: 'on' }, { h: 'Bob', s: 'off' }, { h: 'Cy', s: 'hid' }] });
  const text = () => t.$('members').textContent;
  check('the count says how many are online and how many are members', /2 online/.test(text()) && /4 members/.test(text()));
  const sec = id => t.$('members').querySelector('[data-sec="' + id + '"]');
  check('Online, Status hidden and Offline are separate headed groups', !!sec('on') && !!sec('hid') && !!sec('off') && sec('on').textContent.includes('Online'));
  const row = h => t.$('members').querySelector('.mrow[data-handle="' + h + '"]');
  check('every person has a written status', /Online/.test(row('Amy').textContent) && /Offline/.test(row('Bob').textContent) && /Status hidden/.test(row('Cy').textContent));
  const mark = h => row(h).querySelector('.mmark').textContent;
  check('the three states differ by shape, not only by colour', new Set([mark('Amy'), mark('Bob'), mark('Cy')]).size === 3);
  check('the host is labelled', /HOST/.test(row('Host').textContent));
  t.s.recv({ t: 'mp', h: 'Bob', s: 'on' });
  check('a live update moves Bob to Online and updates the count', row('Bob').dataset.status === 'on' && /3 online/.test(text()));
  t.s.recv({ t: 'mp', h: 'Dee', s: 'on', a: 'f3' });
  check('a newcomer appears', !!row('Dee') && /5 members/.test(text()));
  t.s.recv({ t: 'mp', h: 'Dee', d: 1 });
  check('someone who left the room disappears', !row('Dee') && /4 members/.test(text()));
  t.s.recv({ t: 'mp', h: 'Amy', s: 'off' });
  check('going offline is shown', row('Amy').dataset.status === 'off');
  t.s.recv({ t: 'ml', r: 'abuja', off: 0, tot: 1, on: 1, more: false, m: [{ h: 'Stranger', s: 'on' }] });
  check('an answer for another room is ignored', !row('Stranger'));
  // paging
  t.s.recv({ t: 'ml', r: 'lobby', off: 0, tot: 60, on: 2, more: true, m: [{ h: 'Host', s: 'on', x: true }, { h: 'Amy', s: 'on' }] });
  const more = () => t.$('membersMore');
  check('a long list offers Show more', !!more());
  more().click();
  check('Show more asks for the next page', JSON.stringify(sent(t, 'members').pop()) === JSON.stringify({ t: 'members', off: 2 }));
  t.s.recv({ t: 'ml', r: 'lobby', off: 2, tot: 60, on: 2, more: false, m: [{ h: 'Amy', s: 'on' }, { h: 'Zed', s: 'off' }] });
  check('the next page is added without repeating anyone', t.$('members').querySelectorAll('.mrow[data-handle="Amy"]').length === 1 && !!row('Zed') && !more());
  // closing
  t.$('membersClose').click();
  check('closing hides the list and tells the server', !t.$('members') && sent(t, 'mclose').length === 1);
  t.s.recv({ t: 'mp', h: 'Late', s: 'on' });
  check('updates after closing do nothing', !t.$('members'));
  t.$('roomPeople').click();
  t.s.recv({ t: 'ml', r: 'lobby', off: 0, tot: 1, on: 1, more: false, m: [{ h: 'Amy', s: 'on' }] });
  t.d.dispatchEvent(new t.w.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  t.w.dispatchEvent(new t.w.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  check('Escape closes it', !t.$('members'));
  // changing room closes the list
  t.$('roomPeople').click();
  t.s.recv({ t: 'joined', r: 'abuja', n: 'Abuja' });
  check('entering another room closes the list', !t.$('members'));
  // favourite from inside the room
  t = enter();
  t.s.recv({ t: 'mr', fs: ['lobby'], f: [], v: [] });
  check('the room shows whether it is a favourite', t.$('roomFav').getAttribute('aria-pressed') === 'true' && /Favourite/.test(t.$('roomFav').textContent));
  t.$('roomFav').click();
  check('and the button removes it', JSON.stringify(sent(t, 'fav').pop()) === JSON.stringify({ t: 'fav', r: 'lobby', fav: false }));
  done();
})();
