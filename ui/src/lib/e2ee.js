// End-to-end encryption for direct messages, in the browser. Mirrors e2ee.go on the server.
//
//   identity     ECDH P-256 key pair made here. The private key is created NON-EXTRACTABLE and stored in IndexedDB
//                as a CryptoKey object: this code (and any script on the page) can use it but can not read its bytes,
//                and it is never sent anywhere. Only the public key goes to the server.
//   secret       ECDH(my private, their public), 32 bytes, kept in memory only while a message is handled.
//   message key  HKDF-SHA256(secret, salt "streetsync-dm-v1", info "msgkey|<fpLow>|<fpHigh>|<mid>"), 32 bytes.
//   cipher       AES-256-GCM, random 96 bit nonce, AAD = "streetsync-dm-v1|mid|sender|recipient|sfp|rfp".
//
// A message key opens exactly one message. A reporter can reveal it (revealKey) without exposing anything else.
// What is NOT protected: see docs/E2EE.md (compromised device, malicious page script, key directory trust on
// first contact, message metadata, message length, deletion or reordering by the server).

const te = new TextEncoder();
const td = new TextDecoder();
const SALT = te.encode('streetsync-dm-v1');
const CURVE = { name: 'ECDH', namedCurve: 'P-256' };
export const VERSION = 1;

export const supported = () => !!(globalThis.crypto && globalThis.crypto.subtle && globalThis.crypto.getRandomValues);
const subtle = () => globalThis.crypto.subtle;

export function toB64(buf) {
  const b = new Uint8Array(buf);
  let s = '';
  for (let i = 0; i < b.length; i++) s += String.fromCharCode(b[i]);
  return btoa(s);
}
export const fromB64 = str => Uint8Array.from(atob(str), c => c.charCodeAt(0));
const toB64Url = buf => toB64(buf).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
const toHex = buf => Array.from(new Uint8Array(buf), x => x.toString(16).padStart(2, '0')).join('');
const rand = n => globalThis.crypto.getRandomValues(new Uint8Array(n));

export async function fingerprintOf(pubRaw) {
  return toHex(await subtle().digest('SHA-256', pubRaw));
}

// ---- key storage: IndexedDB when the browser has it, memory otherwise ----
const DB_NAME = 'streetsync-e2ee';
const STORE = 'kv';
let mem = new Map();
let dbp = null;
let persistent = false;

function openDB() {
  if (dbp) return dbp;
  dbp = new Promise(resolve => {
    try {
      const idb = globalThis.indexedDB;
      if (!idb) return resolve(null);
      const req = idb.open(DB_NAME, 1);
      req.onupgradeneeded = () => req.result.createObjectStore(STORE);
      req.onsuccess = () => { persistent = true; resolve(req.result); };
      req.onerror = () => resolve(null);
      req.onblocked = () => resolve(null);
    } catch (e) { resolve(null); }
  });
  return dbp;
}
async function kvGet(key) {
  const db = await openDB();
  if (!db) return mem.get(key);
  return new Promise(resolve => {
    try {
      const r = db.transaction(STORE, 'readonly').objectStore(STORE).get(key);
      r.onsuccess = () => resolve(r.result);
      r.onerror = () => resolve(undefined);
    } catch (e) { resolve(mem.get(key)); }
  });
}
async function kvPut(key, value) {
  const db = await openDB();
  if (!db) { mem.set(key, value); return; }
  return new Promise(resolve => {
    try {
      const tx = db.transaction(STORE, 'readwrite');
      tx.objectStore(STORE).put(value, key);
      tx.oncomplete = () => resolve();
      tx.onerror = tx.onabort = () => { mem.set(key, value); resolve(); };
    } catch (e) { mem.set(key, value); resolve(); }
  });
}
// True when keys survive a reload. The page tells the person when they do not.
export async function keysArePersistent() { await openDB(); return persistent; }
export function _resetForTests() { mem = new Map(); dbp = null; persistent = false; }

const lc = s => String(s).toLowerCase();

// ---- my identity ----
export async function loadIdentity(me) {
  const fp = await kvGet('cur:' + lc(me));
  if (!fp) return null;
  const rec = await kvGet('priv:' + lc(me) + ':' + fp);
  return rec ? { fp, pub: rec.pub, priv: rec.priv } : null;
}

// Makes a new key pair for this account on this device. Older private keys stay (under their fingerprint) so
// messages made for them can still be read on this device. The caller registers the public key with the server.
export async function createIdentity(me) {
  const kp = await subtle().generateKey(CURVE, false, ['deriveBits']);
  const raw = new Uint8Array(await subtle().exportKey('raw', kp.publicKey));
  const fp = await fingerprintOf(raw);
  const pub = toB64(raw);
  await kvPut('priv:' + lc(me) + ':' + fp, { priv: kp.privateKey, pub });
  await kvPut('cur:' + lc(me), fp);
  return { fp, pub, priv: kp.privateKey };
}

async function privFor(me, fp) {
  const rec = await kvGet('priv:' + lc(me) + ':' + fp);
  return rec ? rec.priv : null;
}

// ---- contacts: pinned keys and verification ----
// The first key seen for a contact is remembered (trust on first use). A different key later is never used
// silently: peerCheck reports 'changed' and the page blocks sending until the person accepts it.
export async function peerCheck(me, peer, served) {
  const pin = await kvGet('peer:' + lc(me) + ':' + lc(peer));
  if (!served || !served.fp) return { state: 'none', pin };
  if (!pin) return { state: 'new', pin: null };
  if (pin.fp === served.fp) return { state: 'ok', pin, verified: !!pin.verified };
  return { state: 'changed', pin, oldFp: pin.fp };
}
export async function pinPeer(me, peer, served, keepVerified) {
  const prev = await kvGet('peer:' + lc(me) + ':' + lc(peer));
  const keys = Object.assign({}, prev ? prev.keys : {}, { [served.fp]: served.pub });
  const rec = { fp: served.fp, pub: served.pub, keys, verified: !!(keepVerified && prev && prev.fp === served.fp && prev.verified), since: Date.now() };
  await kvPut('peer:' + lc(me) + ':' + lc(peer), rec);
  return rec;
}
export async function markVerified(me, peer) {
  const pin = await kvGet('peer:' + lc(me) + ':' + lc(peer));
  if (!pin) return false;
  pin.verified = true;
  await kvPut('peer:' + lc(me) + ':' + lc(peer), pin);
  return true;
}
export async function pinnedKeys(me, peer) {
  const pin = await kvGet('peer:' + lc(me) + ':' + lc(peer));
  return pin ? pin.keys : {};
}

// Both people compute the same code from the two key fingerprints. Compare it over another channel
// (in person, a call) to rule out a swapped key. 12 groups of 5 digits.
export async function safetyCode(fpA, fpB) {
  const [lo, hi] = fpA < fpB ? [fpA, fpB] : [fpB, fpA];
  const h = new Uint8Array(await subtle().digest('SHA-256', te.encode(lo + '|' + hi)));
  const groups = [];
  for (let i = 0; i < 12; i++) groups.push(String((h[2 * i] << 8) | h[2 * i + 1]).padStart(5, '0'));
  return groups.join(' ');
}

// ---- messages ----
async function sharedSecret(priv, pubB64) {
  const pub = await subtle().importKey('raw', fromB64(pubB64), CURVE, false, []);
  return new Uint8Array(await subtle().deriveBits({ name: 'ECDH', public: pub }, priv, 256));
}
async function messageKeyBytes(secret, fpA, fpB, mid) {
  const [lo, hi] = fpA < fpB ? [fpA, fpB] : [fpB, fpA];
  const ikm = await subtle().importKey('raw', secret, 'HKDF', false, ['deriveBits']);
  return new Uint8Array(await subtle().deriveBits({ name: 'HKDF', hash: 'SHA-256', salt: SALT, info: te.encode(`msgkey|${lo}|${hi}|${mid}`) }, ikm, 256));
}
const aadOf = (mid, sender, recipient, sfp, rfp) => te.encode(`streetsync-dm-v1|${mid}|${lc(sender)}|${lc(recipient)}|${sfp}|${rfp}`);
const aesKey = (bytes, usage) => subtle().importKey('raw', bytes, 'AES-GCM', false, [usage]);

// Encrypts text for a contact. peerKey is the contact's public key as the server has it ({pub, fp}); the caller
// has already checked it with peerCheck. Returns the envelope to send. The text itself never leaves this function.
export async function encryptMessage(me, peer, mine, peerKey, text) {
  const secret = await sharedSecret(mine.priv, peerKey.pub);
  const mid = toB64Url(rand(16));
  const nonce = rand(12);
  const key = await aesKey(await messageKeyBytes(secret, mine.fp, peerKey.fp, mid), 'encrypt');
  const ct = await subtle().encrypt({ name: 'AES-GCM', iv: nonce, additionalData: aadOf(mid, me, peer, mine.fp, peerKey.fp) }, key, te.encode(text));
  return { v: VERSION, n: toB64(nonce), c: toB64(ct), mid, sf: mine.fp, rf: peerKey.fp };
}

// Which of my key and which of their keys this message used, from my point of view.
function sides(me, from, env) {
  return lc(me) === lc(from) ? { mine: env.sf, theirs: env.rf } : { mine: env.rf, theirs: env.sf };
}

// Reads a message. keys is the contact's known public keys by fingerprint. Rejects with an Error whose message is
//   'no_private'  this device does not hold the private key that message was made for (another device, or lost)
//   'no_peer_key' the contact's key for that message is not known here
//   'bad'         the message does not authenticate (damaged, moved to another conversation, or forged)
export async function openMessage(me, peer, env, from, to, keys) {
  if (!env || env.v !== VERSION) throw new Error('bad');
  const { mine, theirs } = sides(me, from, env);
  const priv = await privFor(me, mine);
  if (!priv) throw new Error('no_private');
  const pub = keys && keys[theirs];
  if (!pub) throw new Error('no_peer_key');
  try {
    const secret = await sharedSecret(priv, pub);
    const key = await aesKey(await messageKeyBytes(secret, env.sf, env.rf, env.mid), 'decrypt');
    const pt = await subtle().decrypt({ name: 'AES-GCM', iv: fromB64(env.n), additionalData: aadOf(env.mid, from, to, env.sf, env.rf) }, key, fromB64(env.c));
    return td.decode(pt);
  } catch (e) { throw new Error('bad'); }
}

// The key that opens this one message, as base64. Used only when the person reports the message on purpose.
export async function revealKey(me, peer, env, from, to, keys) {
  const { mine, theirs } = sides(me, from, env);
  const priv = await privFor(me, mine);
  const pub = keys && keys[theirs];
  if (!priv || !pub) throw new Error('no_key');
  return toB64(await messageKeyBytes(await sharedSecret(priv, pub), env.sf, env.rf, env.mid));
}

// Test hook: load a fixed key pair (synthetic test keys only) so tests can check the same bytes against the Go server.
export async function _importForTests(me, dB64, pubB64) {
  const raw = fromB64(pubB64);
  const jwk = { kty: 'EC', crv: 'P-256', d: toB64Url(fromB64(dB64)), x: toB64Url(raw.slice(1, 33)), y: toB64Url(raw.slice(33, 65)), ext: true };
  const priv = await subtle().importKey('jwk', jwk, CURVE, false, ['deriveBits']);
  const fp = await fingerprintOf(raw);
  await kvPut('priv:' + lc(me) + ':' + fp, { priv, pub: pubB64 });
  await kvPut('cur:' + lc(me), fp);
  return { fp, pub: pubB64, priv };
}
