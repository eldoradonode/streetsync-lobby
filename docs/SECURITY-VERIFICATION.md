# Step 16 security and privacy verification

Every item names the automated test that checks it. "Verified" means verified by these tests in a local environment (Go 1.22, PostgreSQL 16, Node 22 WebCrypto, jsdom). Nothing here is an independent audit.

| Guarantee | Test |
|---|---|
| Plaintext DM frames are refused and never stored or logged | `TestE2EENoPlaintextAnywhere` |
| Database rows hold ciphertext, `body` is empty; marker strings appear in no table (full dump of every table) | same |
| WebSocket frames (relay, thread, inbox, notifications) carry no plaintext | same |
| No private key (hex or base64) appears in any table or log | same |
| The server, using its own key pair or only stored data, cannot open a message | same (step 7) |
| A ciphertext moved to another sender or recipient does not decrypt | same (step 8), `e2ee_interop.test.js` |
| Browser and Go implementations interoperate in both directions | `TestE2EEInteropWithBrowser`, `tests/e2ee_interop.test.js` |
| Nobody can set another account's key; invalid or off-curve keys refused; replacement needs the password; history kept; lookups rate limited | `TestE2EEKeyRegistrationAndAbuse` |
| Sends fail without keys; stale or forged key fingerprints refused; replayed envelope refused and not charged twice | `TestE2EEKeyGuardsOnSend` |
| A report includes only the ticked messages; unticked text is absent from the reports table, the staff list, all storage and logs; wrong or zero key refused; strangers and the sender cannot report | `TestE2EEReportDisclosesOnlySelectedMessages`, page test in `page_dm.test.js` |
| Contact key change blocks sending, hides messages under the unaccepted key, and needs explicit acceptance | `page_dm.test.js` |
| The page sends no plaintext in any frame | `page_dm.test.js`, `page_step14.test.js` |
| Legacy plaintext follows the 7 day window; encrypted rows are deleted on the same schedule | `TestE2EELegacyPlaintextStaysUntilRetention`, `retention_test.go` |
| Password change: wrong or empty current password refused, weak or same new password refused, hashed storage, old password stops working, other logins ended including live connections, rate limited, no password in logs | `TestPasswordChangeFlow` |
| Admin wallet cannot move; moderator wallet intact through promotion and demotion; moderators denied admin-only actions | `step16_policy_test.go` |

## Not verified
- Real browser behaviour: no Chromium was available. IndexedDB persistence of non-extractable keys, and the layout on phones, were not tested in a real browser. WebCrypto was tested through Node, which implements the same API.
- Hosted Supabase and Render: no credentials. Nothing was tested there.
- Staff complaints about the only admin have no independent reviewer. This is an unresolved risk, not a solved one.
- Room probing and report-by-ID were checked only through the existing Step 15 tests (which pass) and the new report tests. No separate new audit was done.
