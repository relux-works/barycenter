# Finalize the independent P1 migration re-review

This is a continuation/finalization pass for `TASK-260715-unbb7c`, not a new
implementation task.

- Review exact PR head `aafcfc222518e7a32e2acaf365a1af4d247cc03c`.
- Do not modify product code or implement any migration.
- Prior tracked run `RUN-260719-260288` independently inspected the fix,
  reproduced the P1-MIG-003 failure at the pre-fix head, proved the new fixture
  passes at the reviewed head, reran the focused migration/predecessor and
  200-test acceptance suites, and observed hosted CI 4/4 green. It ended before
  its background full `go test -race ./...` result could be delivered, so it
  deliberately did not publish an approval.
- Establish the missing full-race result yourself. Do not infer it merely from
  the prior runner exit code.
- If the full race result is green and inspection finds no Critical/High issue,
  publish a final APPROVE verdict that explicitly closes P1-MIG-003, preserves
  the non-blocking MED-1 disposition, names the reviewer/model and exact
  revision, and records that no production data or manual restore was touched.
- If it is not green or another Critical/High issue exists, publish CHANGES
  REQUESTED with evidence and route the task back to development.
- On APPROVE, complete the external review task through task-board. The
  orchestrator will separately reconcile and accept original task
  `TASK-260712-1xkn75`.
