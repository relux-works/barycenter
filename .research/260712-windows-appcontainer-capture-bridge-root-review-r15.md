# Root review R15 — rejected after complete Rev 15 read

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Verdict: **REJECTED / changes required**. This remains a research-only task;
do not edit product source and do not start implementation.

## Review evidence

- Read the complete authoritative note, lines 1–3812, rather than accepting
  the agent summary or its checker.
- Authoritative and canonical `research.md` are byte-identical:
  SHA-256 `7299ddfb4891fe2cb26670ab9517739e7372b5086f313e388547a0ed2bc7aa45`.
- A stale second outcome remains at SHA-256
  `344e59ca165fa69ce74783ab3e1c630f90f042e6a35c6e3699834fd2fc027300`;
  it is not canonical and must be removed.
- Product paths `coordinator`, `pulsar-win`, `node-app`, and `.github` are
  clean.
- The agent checker reports ten passes, but the independent root checker
  `.research/root-checks/windows-r15-contract-check.sh` exits 1 with fourteen
  concrete contradictions. Do not modify root-check fixtures.
- The concrete duplicate-key parser itself passed the independent eight-case
  harness in `.research/root-checks/windows-r15-json-parser/main.go`: valid
  whitespace EOF, top-level duplicates in both orders, nested object,
  object-in-array, concatenated objects, and trailing garbage. Preserve that
  implementation; the remaining sidecar blocker is the reason/HRESULT
  contract, not duplicate detection.
- The independent late-completion model
  `.research/root-checks/windows-r15-quit-model/main.go` proves the stale
  timeout policy violates the ownership invariant at 6, 15, and 29 seconds:
  it posts cleanup with one registry entry and no live waiter. The required
  keep-waiting policy satisfies the invariant in all three schedules.

## Blocking corrections

### 1. Replace every stale quit algorithm with the already-selected policy

The correct high-level diagram at lines 1210–1266 keeps the sole-owner waiter
alive until every operation is terminal, queried, and released. The note then
reintroduces the rejected algorithm:

- line 292 promises bounded five-second operation termination;
- line 1327 times out each operation/quiescence and still proceeds to
  `CLEANUP_READY`;
- lines 1338–1346 claim `Cancel` produces `Canceled`, dismisses prompts, and
  let the waiter proceed/exit while an operation remains live;
- tests 7–9 at lines 1463–1465 explicitly post `CLEANUP_READY` with a pending
  picker/registry;
- unresolved proof 22 and final summary lines 3585–3589 repeat the stale
  bounded policy;
- the required late-terminal tests at 6, 15, and 29 seconds are absent.

`IAsyncInfo::Cancel` only **requests** cancellation; success does not prove a
particular final status or bounded completion. Freeze one policy everywhere:

1. Five seconds exposes/logs Force Quit only; it never releases ownership,
   posts `CLEANUP_READY`, or exits the waiter.
2. The waiter remains alive, keeps every event valid, and handles terminal
   `cancelled`, `completed`, or `failed` results whenever they arrive.
3. It queries/takes/drains and releases each operation exactly once.
4. It posts `CLEANUP_READY` only with an empty registry, an unwound
   subscription, no capture thread, and `CapIsQuiescent()==S_OK`.
5. A never-terminal operation produces no `CLEANUP_READY`; only the explicit
   or automatic 30-second forced fallback may end the process.

Add deterministic 6-, 15-, and 29-second completion tests, the never-terminal
test, and UI-pump responsiveness tests. Do not describe a cooperative cancel
request as guaranteed prompt dismissal or guaranteed `AsyncStatus::Canceled`.

### 2. Make watchdog defeat part of the executable success path

`tryCapDestroyOnce` at lines 1414–1419 kills only the wndproc retry timer and
posts `WM_QUIT`. It never atomically cancels/defeats the independent 30-second
watchdog. The prose at line 1450 assumes a success condition exists but defines
neither its state nor its race ordering.

Specify one atomic exit-state/CAS or equivalent synchronization. On
`CapDestroy==S_OK`, the UI path must win/mark graceful completion and cancel or
defeat the watchdog **before** `PostQuitMessage`. If the watchdog wins first,
the forced exit is final. Test success at 29 seconds racing the watchdog; a
successful graceful destroy must never be followed by `os.Exit(1)`.

### 3. Finish notification-handle ownership in every executable path

The selected ownership paragraph at lines 842–852 is directionally correct,
but the executable descriptions still contradict it:

- lines 504–505 and final summary line 3589 lazily duplicate the capture
  handle "at session start", instead of `CapturePrepare` pre-creating it
  before operation/thread publication;
- the branch table at lines 704–718 has no `CapturePrepare DuplicateHandle`
  row and no `CaptureActivate DuplicateHandle` row;
- its `CoInitializeEx` row names both original `notifyEvent` and
  `localNotify` as wakes (line 710);
- synchronous launch, async failure, normal handoff, and both cancel branches
  do not consistently state callback-duplicate close ownership/order;
- the normative pre-handoff exception at line 749 says the capture thread
  exits after steps 5/6 and omits closing its own duplicate, contradicting the
  exact close at line 817;
- callback orderings at lines 887–914 omit the callback duplicate entirely;
- test 6 at line 1005 says the handler signals a handle that is "now closed as
  part of ... destruction". A handler cannot safely signal an already-closed
  handle; destruction/close happens only after its final strong ref returns.

Generate the diagram, branch table, cleanup exception, callback lifetime
section, tests, and final summary from one ownership table. Every duplicate
must have one creation point, one owner, and one signal/close result on every
branch. No worker may signal Go's original handle.

For `CaptureActivate`, freeze the race-safe linearization explicitly: create
the callback duplicate while the operation is still `PREPARED`, attempt the
`PREPARED→ACTIVATING` CAS, close the duplicate if the CAS loses to stop, then
launch. A duplicate failure leaves `PREPARED`; no rollback from `ACTIVATING`
may overwrite a concurrent stop.

### 4. Repair the private FSM and observable `ready` contract

- The packed layout declares `TERMINAL=6` at lines 2705–2706 and the private
  table declares `TERMINAL (6)` at line 2724, but line 2742 calls the same
  private state `TERMINAL(5)`. Private 5 is already `SEALED`; this is not an
  editorial typo an implementation can safely infer around.
- `ready` is said to be release-stored in `format->ready` at lines 526 and
  2728, which reads as concurrent mutation of caller-owned query memory.
  Continuation review required a session-owned atomic that
  `CaptureGetResult` acquire-loads and copies into the caller buffer.
- The table says `ready=1` in `PREPARED`, `ready=0` in `ACTIVATING`, and then
  `ready=1` in `CAPTURING` (lines 2719–2722), reusing one field for both
  "MTA-ready" and "format valid". `valid` already owns format validity.
- `CaptureRequestStop` is an `S_OK` no-op for unknown/released IDs at line
  2384 but returns `E_INVALIDARG` at line 2767. Freeze one ABI result and use
  it in comments, truth tables, and tests (`E_HANDLE` remains the established
  query result for unknown IDs unless deliberately changed).
- line 860 says every stop signals `stopEvent`, contradicting the private FSM:
  `PREPARING`, `PREPARED`, and `ACTIVATING` must signal
  `captureThreadWakeEvent`.

Keep one private enum: `PREPARING=0`, `PREPARED=1`, `ACTIVATING=2`,
`CAPTURING=3`, `STOPPING=4`, `SEALED=5`, `TERMINAL=6`. Define `ready` once as
session-owned MTA-readiness (prefer monotonic 0→1 after successful MTA init;
`valid` remains the negotiated-format flag), and derive every public mapping
and transition test from it.

### 5. Make the branch table genuinely exhaustive

The table's claim at line 700 is false. Besides both missing duplicate-failure
rows, its global rule at lines 702/1030 says every failure after `opId`
publication is asynchronous, while `CaptureActivate` deliberately returns a
direct failure for callback-handle duplication or an invalid state.

Add rows for:

- capture-thread duplicate failure before `opId` publication;
- callback duplicate failure while still `PREPARED`;
- callback duplicate created but the `PREPARED→ACTIVATING` CAS loses;
- synchronous activation-launch failure with callback duplicate closed;
- normal callback handoff and async callback failure with callback duplicate
  closed without signaling;
- both cancellation orderings with exact thread/callback duplicate counts.

For every row freeze: function HRESULT, whether `opId` already exists,
private/public state, registry membership, duplicate creation/signal/close
counts, callback expectation, wake handle, terminal publisher, and cleanup
owner. Then make the generic initiate principle explicitly acknowledge the
post-`CapturePrepare` direct-call validation/duplication exceptions.

### 6. Publish one complete sidecar reason/HRESULT table

Line 3092 still requires `hresult==0` for `cancel`, directly contradicting
lines 822 and 1010, which freeze
`HRESULT_FROM_WIN32(ERROR_CANCELLED)`. It also says merely "a valid negative
HRESULT for failure reasons" and never defines lifecycle, device-loss,
permission, overflow, discontinuity, format, or unknown-WASAPI compatibility.

Publish one table covering all `CAP_REASON_*` values 0–10. For each reason,
freeze the allowed exact HRESULT or allowed class/set and use that same table
for terminal publication, JSON validation, recovery tests, and evidence logs.
At minimum preserve the already-frozen cancel, overflow, permission,
device-invalidated, format, and unknown-WASAPI outcomes. Either validate
`reasonName` against the enum or explicitly derive/ignore it during recovery so
it cannot falsify logs.

The token-level duplicate parser passed root tests; preserve it. Add the
typed-decoder tests still required by the continuation review: missing each
required field, unknown fields, invalid enum/version/session ID, and every
reason/HRESULT row.

### 7. Remove remaining summary/test contradictions and strengthen checks

- Final summary line 3593 says the rank-4 pair is both a tie and "no ties".
  Preserve the canonical first-installed tie rule from lines 2685–2686 and
  2816, or assign distinct ranks everywhere; do not assert both.
- Adversarial test 12 says the capture thread releases a strong session ref,
  while the thread contract says it holds no such ref.
- The current checker passes because it does not assert private-enum
  uniqueness, late-terminal ownership, watchdog defeat, exhaustive duplicate
  branches, or body/final-summary agreement.

Extend the agent-owned checker without editing root fixtures. It must fail on
every stale phrase/branch found above, assert exactly one private enum and one
public enum, and verify that the final answer uses the same generated tables as
the body. Record actual commands and outputs only after the final bytes exist.

### 8. Normalize the outcome set

Delete the stale duplicate
`260712-windows-appcontainer-capture-bridge.md` outcome. Replace/update only
the canonical `research.md` after all corrections and checks pass, with an
accurate Rev 16 description and a byte-identical SHA. Attach the improved
checker as a separate outcome. Do not claim completion from checker output
alone; return to `to-review` for another complete root read.

## Delivery constraints

- Amend only the research note and agent-owned check/outcome resources.
- Do not edit product source or root-check fixtures.
- Do not mark the task `done`.
- In the handoff, list exact final line/word/byte counts, SHA-256, checker
  commands/results, and `cmp` result; these are evidence, not substitutes for
  root review.
