# Root review round 15 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: **approved**.

I reviewed every Rev 15 amendment against the previously full-read Rev 13 and
the complete R14 correction set. The authoritative note and the single
canonical task outcome are byte-identical: 4,181 lines, 32,752 words, 252,149
bytes, SHA-256
`ba049711820d42cab102a51aa91316613dd8183fc1ebb109ec6eefd1f068ba0a`.
Product source remains untouched.

## Accepted corrections

1. The `orbits.status` rebuild now classifies owned indexes/triggers strictly
   by `tbl_name`, captures and drops **all** user-defined views and **all**
   external triggers, and recreates their exact DDL inside the same transaction.
   No SQL-body substring/token heuristic remains.
2. The migration captures the connection's `PRAGMA foreign_keys` state before
   disabling it and freezes a rollback-before-restore defer/finally for every
   commit, rollback, SQL error, panic, and fatal intermediate-state exit.
3. A read-handle `CloseHandle` failure is consistently fatal for the recovery
   attempt and blocks the network send. The ownership table, detailed branch,
   send barrier, summary, and fault-test oracle now agree.

## Independent checks

- Authoritative/outcome SHA and byte counts match; exactly one canonical
  `research.md` outcome exists.
- `.research/root-checks/recovery-r14-foreign-keys.sql` reproduces why an
  OFF → transaction → ROLLBACK path remains `foreign_keys=0` without the
  required restoration.
- `.research/root-checks/recovery-r15-rebuild.sql` executes the selected
  conservative rebuild with owned index/trigger, dependent and unrelated
  views, and dependent and unrelated external triggers. Result:
  - all six schema objects exist after rebuild;
  - `PRAGMA foreign_keys = 1`;
  - `PRAGMA foreign_key_check` returns zero violations;
  - the rebuilt status constraint rejects `status='bogus'`.
- Static searches find no surviving policy that sends after a read-handle
  close failure and no dependency selection by SQL-body scan.
- Live `PairSlot` source confirms the already-accepted one-way rollback story:
  after limits are restored, a revoked slot letter can be explicitly re-paired
  with a newly minted token; status re-enable alone never revives it.

## Approval boundary

This approves the research/decision contract, not an implementation. Product
code must still implement the exact SQL, same-connection PRAGMA lifecycle,
DPAPI fault behavior, transactions, redaction, and full test inventory, and is
subject to separate line-by-line root review before acceptance.
