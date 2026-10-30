# End-to-end encrypted direct messages (Step 16)

Status: implemented for a single device per account. Not independently audited. Get an external cryptographic review before anyone relies on it for highly sensitive communication.

## What the server and database can see
- Ciphertext, nonce, random message id, sender and recipient accounts, time, the fingerprints of both public keys, the size of the ciphertext.
- Each account's public key and key history (`e2e_keys`, `e2e_key_log`).
- Never: message text, private keys, conversation secrets. Notifications say "Sent you an encrypted message". Inbox previews are decrypted in the browser.
- Room messages are NOT encrypted. Only direct messages are.

## Design
- Identity key: ECDH P-256, generated in the browser with WebCrypto. The private key is created non-extractable and stored in IndexedDB. Only the public key is sent (`ekey`).
- Message key: HKDF-SHA256 over the ECDH secret, salt `streetsync-dm-v1`, info `msgkey|<fpLow>|<fpHigh>|<mid>`. `mid` is 128 random bits chosen per message, so each message has its own key and nonce reuse is not a concern.
- Cipher: AES-256-GCM, random 96 bit nonce. AAD = `streetsync-dm-v1|mid|sender|recipient|sfp|rfp`, so a ciphertext moved to another sender, recipient or key pair fails to decrypt.
- The server checks only the shape of an envelope and that `sfp`/`rfp` are the keys on file for both people. Replays of a message id are refused (`dm_mid` unique index).
- Plaintext `dm` frames are refused. New messages are never stored readable.

## Key registration and trust
- A key always belongs to the authenticated connection's own account. No frame can name another account.
- First key: registered automatically. Replacing a key: needs the account password (accounts that have one; guests have no password, so a guest session alone can replace its key). Every registration is logged in `e2e_key_log`.
- Contacts: the first key seen for a contact is pinned in the browser (trust on first use). A different key later blocks sending and shows a warning with the safety code until the person accepts it. Messages under an unaccepted key are not shown as text.
- Safety code: 12 groups of 5 digits from both fingerprints. Compare in person or on a call, then press "It matches".
- First contact trusts the server's key directory. A malicious or compromised server could hand out a wrong key at first contact. Only comparing safety codes out of band rules that out.

## Devices, lost keys, resets
- One device. The key lives in that browser. Another device sees "Your key is on another device" and can make a new key (needs password). Contacts then see a key-change warning. Old messages stay readable only where the old private key is.
- Clearing browser data or losing the device loses the key and those messages. Nobody, including the operator, can recover them. No server-side recovery key exists by design.
- A password change or reset never touches keys and never gives the server access to old conversations.
- If the browser does not persist IndexedDB, Settings says so (keys vanish on reload).
- Logging out keeps keys on the device (so messages stay readable). On a shared device, clear site data.

## Existing plaintext DMs
- Old rows (`enc = 0`) are not converted and are shown labelled "Not encrypted". They stay readable on the server until the retention job deletes them (at most 7 days after they were sent). Nothing is deleted early.

## Reporting
- The reporter ticks the messages to report (up to 5). The page reveals one message key per ticked message. The server decrypts the stored ciphertext with that key, checks it authenticates, and stores only that plaintext. A key opens one message only (HKDF is one way), so unticked messages stay unreadable.
- Without a key the server refuses (`needs_disclosure`). Only the recipient can report. Staff see only the stored selected text.
- A reporter can still choose to report a message that was real but misleading in context. Staff only ever see what was selected.

## Not protected
- A compromised device or browser, malicious or injected page script (it can use the key), a malicious server serving altered page code (the web app is delivered by the server, which is a fundamental limit of browser E2EE).
- Metadata: who talks to whom and when, message sizes.
- Deletion, dropping or reordering of messages by the server. No forward secrecy or post-compromise security: one stolen private key decrypts everything encrypted for it.
- Screenshots, and the recipient keeping or forwarding messages.
