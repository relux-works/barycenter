# Root acceptance R16 — Windows AppContainer capture bridge

Date: 2026-07-13  
Task: `TASK-260712-6kba80`  
Verdict: **ACCEPTED as the implementation gate**. This accepts research and
contracts only; no product implementation was made by this task.

## Review performed

- Root read the complete 3,812-line Rev 15, rejected it in
  `260712-windows-appcontainer-capture-bridge-root-review-r15.md`, then reviewed
  the complete Rev15→Rev16 diff and every amended contract in final context.
- No agent-produced change was accepted on summary. The stopped Rev16 agent
  produced no file; all accepted Rev16 changes and executable fixtures were
  authored and reviewed by root.
- Official Microsoft documentation was rechecked for `IAsyncInfo::Cancel`,
  synchronous `MediaDevice.GetDefaultAudioCaptureId`, `DuplicateHandle`,
  `CreateThread` early execution, `CloseHandle`, and CRT-safe `_beginthreadex`.
- Product paths `coordinator`, `pulsar-win`, `node-app`, and `.github` remained
  clean throughout.

## Additional root findings fixed during acceptance

The final pass caught issues not covered by the R15 rejection list: stale
discontinuity=`ERROR_BUFFER_OVERFLOW`, a direct `CAPTURING→SEALED` conversion
path, an impossible claim that HRESULT occupied the published 64-bit packed
layout, incomplete callback-duplicate closes in cycle-breaking prose, stale
11/12 test counts, an unowned thread launch handle, and raw `CreateThread` for
a CRT-using `/MT` helper. Rev16 now uses `_beginthreadex`, a creator-held
early-start lifetime fence, exact launch-handle close ownership, and a frozen
CRT-error→HRESULT mapping.

## Final artifact identity

- Decision note: 4,192 lines; 51,119 words; 398,825 bytes.
- Decision note SHA-256:
  `a969885686814b44c2b7a7aaef4fcdbc3cf05b90f044a942c0eba92524ae0847`.
- Consistency checker: 238 lines; 1,376 words; 11,450 bytes.
- Checker SHA-256:
  `dc4dc5f4c2291d27ae604e99d727c812b697b28009332e481db4102d3fbbfdda`.

## Executed evidence

All commands exited 0 against the final bytes:

```text
bash .research/root-checks/windows-consistency-check.sh .research/260712-windows-appcontainer-capture-bridge.md
bash .research/root-checks/windows-r15-contract-check.sh .research/260712-windows-appcontainer-capture-bridge.md
go run .research/root-checks/windows-r15-json-parser/main.go
go run .research/root-checks/windows-r15-quit-model/main.go
go run .research/root-checks/windows-r16-fsm-model/main.go
go run .research/root-checks/windows-r16-sidecar-contract/main.go
bash -n .research/root-checks/windows-consistency-check.sh .research/root-checks/windows-r15-contract-check.sh
git diff --check -- .research
```

The Markdown structure check found 50 code fences (even/closed), and every
`[MS-*]` citation used by the body has a source definition. Canonical task-board
resources must be compared byte-for-byte before the task moves to `done`.
