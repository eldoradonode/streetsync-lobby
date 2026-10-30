# Wallet policy (Step 16)

- Admin accounts have no wallet. `applyCreditTx` (the only place credits move) refuses any movement for an admin, in both directions. That covers tips, room creation and entry fees, grants, welcome and streak rewards and anything added later. Tips and grants return `admin_no_wallet`. Admins create rooms and enter paid rooms without paying and do not pay DM fees. The UI hides the wallet tab, credit chip and tip button for admins.
- An admin balance that already exists is not deleted and not changed. It stays frozen with its ledger. Run `scripts/admin_wallet_report.sql` (read only) before and after deploying to see it. What to do with a frozen balance is a decision for the operator.
- Moderators keep wallets, balances and history. Promotion and demotion change only the role. Ordinary users can not tip moderators or admins. Staff-owned rooms are free (staff earn no entry fees).
- Roles are read from the database inside each transaction, never from the client.
