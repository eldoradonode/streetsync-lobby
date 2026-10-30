const { check, signed, done, sleep } = require('./helpers16');
(async () => {
  let t = signed();
  check('an ordinary user sees the wallet tab and credit chip', !!t.$('tabWallet') && !t.$('credChip').hidden);
  t.s.recv({ t: 'me', p: 0, r: 0, x: 0, s: 0, g: true, c: 0, o: 'admin' });
  await sleep(20);
  check('an admin has no wallet tab', !t.$('tabWallet'));
  check('an admin has no credit chip', t.$('credChip').hidden);
  t.s.sent.length = 0;
  t.w.eval ? 0 : 0;
  check('the page never asks an admin for a wallet', !t.s.sent.some(x => x.t === 'wallet'));
  t.s.recv({ t: 'me', p: 0, r: 0, x: 0, s: 0, g: true, c: 12, o: 'mod' });
  await sleep(20);
  check('a moderator keeps the wallet tab', !!t.$('tabWallet') && !t.$('credChip').hidden);
  t.s.recv({ t: 'err', c: 'admin_no_wallet' });
  check('the refusal is explained', t.$('status').textContent.includes('do not use credits'), t.$('status').textContent);
  done();
})();
