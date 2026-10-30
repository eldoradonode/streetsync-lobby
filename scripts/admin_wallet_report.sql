-- Read-only report for the Step 16 admin wallet policy. Run it before deploying and after.
-- Admin accounts can not receive, spend or earn credits any more. A balance that already exists is NOT deleted:
-- it stays frozen exactly as it was, and every ledger row stays. Nothing here changes data.
--
-- 1. Admin accounts that hold a balance or have ledger history.
SELECT u.id, u.handle, u.credits AS frozen_balance,
       (SELECT count(*) FROM credit_ledger l WHERE l.user_id = u.id)            AS ledger_rows,
       (SELECT coalesce(sum(l.amount), 0) FROM credit_ledger l WHERE l.user_id = u.id) AS ledger_sum
FROM users u
WHERE u.role = 'admin' AND u.deleted_at IS NULL
ORDER BY u.id;
-- 2. Consistency for those accounts: frozen_balance should equal ledger_sum. A difference means the balance
--    was changed outside the ledger and needs a person to look at it before anyone decides what to do.
SELECT u.handle, u.credits, coalesce(s.total, 0) AS ledger_sum, u.credits - coalesce(s.total, 0) AS difference
FROM users u LEFT JOIN (SELECT user_id, sum(amount) AS total FROM credit_ledger GROUP BY user_id) s ON s.user_id = u.id
WHERE u.role = 'admin' AND u.deleted_at IS NULL AND u.credits <> coalesce(s.total, 0);
-- 3. Optional, run by a person after deciding what should happen to a frozen balance (the app never does this):
--    INSERT INTO credit_ledger ... is NOT recommended. Leave the balance frozen, or move it to the operator's
--    records with a documented ledger entry made under a separate maintenance process.
