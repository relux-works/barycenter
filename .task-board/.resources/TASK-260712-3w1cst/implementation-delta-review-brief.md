# Independent delta review: E2EE schema and epoch foundation

Review `TASK-260712-3w1cst` as an implementation-independent security,
persistence and migration reviewer. Do not modify production or test code.

The producer claims only a dormant additive foundation. Production E2EE must
remain disabled and unadvertised; no library, suite, container, serialization,
real-app, signed-package or hardware evidence may be inferred. EPC-001,
EPC-002, EPC-004, EPC-005 and external review `TASK-260712-1ulshp` remain open.

Review the exact Git HEAD and working tree. Recompute every SHA-256 pin in
`acceptance/phase3/e2ee-schema-epoch-foundation-v1.json`, then inspect at least:

- every `e2ee_*` table, CHECK/index/trigger, startup order and foreign key;
- repository validation and transaction boundaries for commit, fork, replay,
  protected-object finalize/revoke, history-grant revoke, transfer and report;
- fresh, faulted migration, generation-skip, rollback-era legacy write,
  concurrency, restart, immutability and on-disk sentinel tests;
- absence of secret/plaintext/decrypted-evidence storage and any runtime or
  capability-advertisement callsite;
- the protocol delta that addresses IDR-001, IDR-002 and IDR-003: canonical
  failure precedence, sequence/generation vectors on all three platforms and
  strict public-envelope decoders;
- legacy plaintext compatibility only for explicitly non-E2EE rows;
- exact no-claim/manual-evidence posture.

Run fresh focused checks and, if time allows, the full relevant suites. The
producer evidence is: coordinator full suite green, Store + E2EE race green
(`store 475.523s`), Windows full suite green, macOS full suite green (308
tests), and both Python acceptance modules green (6 tests). Do not accept
producer output without independent reproduction or source inspection.

Record one terminal outcome resource with exact reviewed commit/hashes,
commands/results, findings by severity, owners/retests and one explicit verdict:

- APPROVE: zero open Critical/High implementation or delta-design finding;
  set the task `done` and check the reviewer checklist items.
- REJECT_FIX: any producer-fixable Critical/High; set `development` and leave
  exact findings.
- BLOCK_EXTERNAL: only an external/manual dependency blocks the task's stated
  dormant engineering scope; set `blocked` and identify it.

Low and informational findings may be accepted only when explicitly tracked.
Any protocol-affecting change after the reviewed commit requires another delta
review. Never mark or imply the production E2EE capability accepted.
