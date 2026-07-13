# Root review round 13 — Windows AppContainer capture bridge

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Verdict: rejected. I read all 3,508 lines of Rev 13 and verified the
authoritative and task-board outcome copies are byte-identical (3,508 lines,
46,643 words, 363,669 bytes, SHA-256
`a7d112eaac2be00ec0718ac3844e19328f5d1cd65693891ea2452e755596d4a8`).
Product source remains untouched. Rev 13 makes `CapturePrepare` nonblocking
and selects Go as the sole sidecar writer, but the resulting contract still
cannot be implemented safely as written. The failures below come from the
normative ABI/state diagrams and independently executed checks, not from the
revision summary.

## Blocking corrections

1. **Publish one non-overlapping public capture ABI, including an observable
   MTA-ready state.** The ABI at lines 2053–2068 freezes
   `0=preparing, 1=activating, 2=capturing, 3=stopped, 4=failed,
   5=cancelled`. The packed-state section at lines 2497–2503 and the final
   summary at line 3289 instead freeze `stopped=2, failed=3, cancelled=4`.
   In that second mapping, a normally stopped operation is numerically
   indistinguishable from an actively capturing operation. The cancellation
   diagrams use `cancelled(4)` (lines 678 and 694), while the ABI says 4 is
   failed and 5 is cancelled. Test 26 then says a readiness-timeout cancel
   becomes `FAILED` (line 3266), contradicting the cancel mapping again.

   The two-step readiness handshake is not representable either. Lines
   2057–2060 say the waiter observes an internal `readyFlag` field in
   `CaptureFormat`, but the complete versioned struct at lines 1601–1615 has
   no such field; `readyFlag` appears nowhere else in the note. `state=0` and
   `format.valid=0` therefore mean both “still initializing” and “MTA ready.”

   Define public constants once and generate the ABI comments, packed-state
   mapping, diagrams, Go query switch, and tests from them. Add a real
   versioned readiness output (`prepared` public state or a documented
   `ready` field included in `CaptureFormat.structSize`) with exact memory
   ordering. Terminal values must be disjoint from every nonterminal value.

2. **Close and signal every notification duplicate on every cancellation
   ordering; never use Go's original handle from worker threads.** The capture
   thread duplicates `notifyEvent` at session start (lines 409–410). In the
   typical pre-handoff cancel path it calls `CoUninitialize`, sets
   `threadDone`, and exits without steps 7–11 (lines 648 and 671–680). It never
   closes its own duplicate. The callback separately duplicates the handle at
   registration (line 698); in Diagram B it allegedly “does NOT hold
   `localNotify`” and returns without closing it (lines 686–694), even though
   creation happened before the ordering was known. Both schedules leak a
   kernel handle.

   The main handoff diagram also calls `SetEvent(notifyEvent)` directly for
   capture-ready/data notifications (lines 521–522 and 542), while the frozen
   ownership rule says neither entity ever signals Go's original handle
   (line 698). The CoInitialize path alternates among undefined `readyEvent`,
   original `notifyEvent`, and `localNotify` (lines 420, 427, 430, and 609).

   Give each duplicate a creation owner, a single close owner, and a close-
   without-signal branch. Diagram A must close the thread's unused duplicate;
   Diagram B must close the callback's unused duplicate. All capture-thread
   readiness/data/terminal signals must use the thread-owned duplicate. Add
   exact `DuplicateHandle`/`SetEvent`/`CloseHandle` counts for every ordering,
   including duplicate failure before thread start/handler registration.

3. **Keep a live owner for timed-out operations until terminal/release, and
   make UI-thread retries asynchronous.** The quit algorithm waits five
   seconds, proceeds past a still-pending operation, posts
   `WM_APP+CLEANUP_READY`, and exits the waiter (lines 1087–1105 and 1184).
   But terminal operations remain in the helper registry until their explicit
   `*Release` call (lines 1173–1180 and 1364). If a picker callback completes
   after the waiter exits, no owner remains to query its terminal result and
   call `PickerRelease`; the registry can therefore never become empty and
   `CapDestroy` can never succeed. Line 1293 nevertheless says event handles
   close only after the waiter exits **and all operations are released**, which
   is impossible on this path.

   The UI handler then calls `Sleep(100)` up to 100 times inside the message
   handler (lines 1241–1261), freezing the message pump for roughly ten
   seconds despite the repeated promise that the pump remains responsive. At
   the end it stops retrying. Even if some independent state could become
   destroyable at second 15, there is no subsequent `CapDestroy` call; only the
   30-second `os.Exit(1)` remains.

   On a graceful path, keep the waiter/event handles alive and continue the
   query/release loop for every late terminal operation. Schedule each
   `CapDestroy` retry with a timer/message so the wndproc returns immediately,
   and continue until success or a separately named forced-exit policy fires.
   A bounded timeout may expose “Force Quit”; it cannot abandon the only
   release owner and still claim eventual graceful teardown. Model and test a
   callback completing at 6, 15, and 29 seconds, plus a callback that never
   completes.

4. **State the real cooperative-cancellation guarantee.** Lines 1171 and 1182
   call every operation's termination bounded and claim the WinRT contract
   says an operation transitions to `Canceled` and its handler fires after
   `IAsyncInfo::Cancel`. The cited API's normative guarantee is only that
   `Cancel` *requests* cancellation; it does not promise prompt dismissal or a
   five-second terminal bound. The revision's own never-completes tests at
   lines 1283–1284 acknowledge this.

   Remove the stronger documentation claim. Treat `Cancel` as cooperative,
   keep the operation live until actual terminal callback, and define the
   30-second policy honestly as an automatic forced fallback if that is the
   selected product behavior. Lines 29–30/188 call ForceQuit a separate
   explicit path, while lines 1079, 1143, 1165, 1269, and 3281 start its
   watchdog on every ordinary Quit/Ctrl-C/SIGTERM; those policies are not
   mutually exclusive as line 1272 claims. Freeze one truthful naming and
   state machine.

5. **Make the Go-only reason sidecar contract self-consistent and reject
   duplicate JSON keys explicitly.** Rev 13 correctly says Go is the sole
   writer after terminal publication at lines 2841–2857 and 3296. Surviving
   normative text instead says the sidecar is durable before terminal
   notification (lines 135, 199, and 2819), and says the native capture thread
   writes it before `threadDone` during shutdown (line 2921). The helper has no
   filesystem ABI, so those guarantees are impossible. Delete them and keep
   the accepted fail-closed gap: death before Go's durable write means discard.

   Line 2836 also says `json.Decoder` plus `DisallowUnknownFields` rejects
   duplicate fields. It does not. I executed the standard library decoder
   against `{"reason":0,"reason":1}` with `DisallowUnknownFields`; it
   returned `error=<nil> reason=1`. The reproducible root check is
   `.research/root-checks/windows-r13-json-duplicates/main.go`. A crafted
   sidecar can therefore silently replace a promotable reason with a
   non-promotable reason or vice versa depending on ordering.

   Implement explicit duplicate-key detection at the token/object layer before
   decoding typed fields, then enforce exactly one occurrence of every required
   key, no unknown keys, one trailing EOF, bounded size, known version/reason,
   session-ID equality, and reason/HRESULT compatibility. Add both duplicate
   orders for every security-relevant key to recovery tests.

6. **Run the promised static consistency checks against the actual note and
   fail resubmission when they disagree.** Lines 656–659 assert
   `grep -c 'cancelled bit' note.md` is zero. Against the authoritative Rev 13,
   the exact command returns **4**, and a case-insensitive contextual search
   finds additional prose/history occurrences. The sidecar contradictions and
   state-number conflicts are similarly machine-detectable. A self-check that
   is merely written into the note but not executed is not evidence.

   Provide an executable checker with robust regexes and explicit expected
   canonical declarations: one public state table, one sidecar writer/order,
   one stop-priority table, no direct worker use of original `notifyEvent`, and
   no finite retry loop described as continuing indefinitely. Include its
   actual output and nonzero failure behavior.

7. **Repair the stop-priority inventory and generated tests.** The canonical
   table assigns overflow=1, discontinuity=2, permission-revoke=3 (lines
   2470–2480). Test 6 says permission-revoke is priority 2, while test 7 says it
   is priority 3 (lines 2554–2556). The header history repeats priority 2
   (lines 69–70). The packed CAS may still be sound, but the test oracle is not.

   Generate the enum/rank function and every race expectation from one table.
   Test both schedules for overflow vs discontinuity, overflow vs permission,
   permission vs user stop, and equal-ranked WASAPI vs format error; freeze an
   explicit tie rule for the equal-ranked pair.

## Resubmission

- Amend one authoritative note, attach a byte-identical outcome, keep product
  source untouched, and return to `to-review`, never `done`.
- Preserve the nonblocking `CapturePrepare` direction, MTA handoff, packed
  reason seal, release-before-ring-commit loop, visible picker owner,
  process-lifetime DLL load, and Go-only fail-closed sidecar writer.
- Execute the public-state/readiness model, both cancellation handle schedules,
  late-terminal graceful-quit model, duplicate-JSON fixture, and static note
  checker. A revision summary or a future test list is not acceptance evidence.
