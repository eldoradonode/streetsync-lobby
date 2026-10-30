// The browser crypto module must agree with the Go server: decrypt a Go message, and make one the server can verify.
const { webcrypto } = require('crypto');
const fs = require('fs');
const path = require('path');
globalThis.crypto = webcrypto;
let pass = 0, fail = 0;
const check = (n, c, x) => { if (c) { pass++; console.log('PASS', n); } else { fail++; console.log('FAIL', n, x || ''); } };

(async () => {
  const E = await import('../ui/src/lib/e2ee.js');
  const v = JSON.parse(fs.readFileSync(path.join(__dirname, 'fixtures', 'e2ee-vectors.json'), 'utf8'));
  const alice = await E._importForTests('alice', v.aliceD, v.alicePub);
  const bob = await E._importForTests('bob', v.bobD, v.bobPub);
  check('fingerprints match what Go computed', alice.fp === v.env.sf && bob.fp === v.env.rf, [alice.fp, v.env.sf]);
  const pt = await E.openMessage('bob', 'alice', v.env, 'alice', 'bob', { [alice.fp]: alice.pub });
  check('the browser code decrypts a message made by the Go code', pt === v.plain, pt);
  const own = await E.openMessage('alice', 'bob', v.env, 'alice', 'bob', { [bob.fp]: bob.pub });
  check('the sender can read their own message too', own === v.plain);
  let err = '';
  try { await E.openMessage('bob', 'alice', v.env, 'alice', 'carol', { [alice.fp]: alice.pub }); } catch (e) { err = e.message; }
  check('a changed recipient name breaks decryption', err === 'bad', err);
  const tampered = Object.assign({}, v.env, { c: v.env.c.slice(0, -4) + (v.env.c.slice(-4) === 'AAAA' ? 'BBBB' : 'AAAA') });
  err = '';
  try { await E.openMessage('bob', 'alice', tampered, 'alice', 'bob', { [alice.fp]: alice.pub }); } catch (e) { err = e.message; }
  check('a changed ciphertext is rejected', err === 'bad', err);

  const plain = 'Browser vector: grüße ✓ ' + Date.now();
  const env = await E.encryptMessage('alice', 'bob', alice, { pub: bob.pub, fp: bob.fp }, plain);
  const key = await E.revealKey('alice', 'bob', env, 'alice', 'bob', { [bob.fp]: bob.pub });
  const key2 = await E.revealKey('bob', 'alice', env, 'alice', 'bob', { [alice.fp]: alice.pub });
  check('both sides derive the same message key', key === key2);
  fs.writeFileSync(path.join(__dirname, 'fixtures', 'e2ee-js-vector.json'), JSON.stringify({ plain, env, key }, null, 2));
  check('envelope has exactly the expected fields', JSON.stringify(Object.keys(env).sort()) === JSON.stringify(['c', 'mid', 'n', 'rf', 'sf', 'v']));
  const again = await E.encryptMessage('alice', 'bob', alice, { pub: bob.pub, fp: bob.fp }, plain);
  check('same text gives a different nonce, message id and ciphertext', again.n !== env.n && again.mid !== env.mid && again.c !== env.c);
  check('the plaintext is not in the envelope', !JSON.stringify(env).includes('grüße') && !Buffer.from(env.c, 'base64').toString('utf8').includes('grüße'));
  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail ? 1 : 0);
})();
