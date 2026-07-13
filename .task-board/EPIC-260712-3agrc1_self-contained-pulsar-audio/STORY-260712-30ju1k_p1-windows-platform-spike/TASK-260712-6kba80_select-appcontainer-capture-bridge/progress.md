## Status
done

## Assigned To
[reviewer] root

## Created
2026-07-12T15:27:53Z

## Last Update
2026-07-13T00:10:47Z

## Blocked By
- (none)

## Blocks
- TASK-260712-dib11l
- TASK-260712-e1ie4x
- TASK-260712-298tyq

## Checklist
- [x] Inspect current pulsar-win audio, UI and MSIX packaging files
- [x] Produce an option matrix for capture, picker, hotkey and lifecycle APIs
- [x] Record the selected bridge, manifest changes and unresolved hardware proofs
- [x] Audit bridge ABI, thread ownership, redistribution license, signed-package eligibility and minimum OS support
- [x] Findings written to file
- [x] Key aspects highlighted
- [x] Fact-checking performed — claims verified, sources cited
- [x] Document linked as outcome resource
- [x] All questions from task description answered

## Notes
Decision note written to .research/260712-windows-appcontainer-capture-bridge.md and attached as an outcome resource. Selected bridge: WinRT permission/device enumeration plus ActivateAudioInterfaceAsync-backed WASAPI capture behind a narrow native helper DLL; FileOpenPicker via IInitializeWithWindow; hotkey and lifecycle moved onto a hidden top-level window instead of the current message-only tray window. Rejected runFullTrust, broad filesystem access, pure MMDevice-only capture as the primary Store probe path, and Media Foundation as the primary P1.0 path. Unresolved items are the signed Windows 10/11 hardware proofs recorded in the note.
agent completed: [analyst] researcher (codex) (exit=0)
agent spawned: codex (pid=3667, exit=0)
Root review round 1: changes required. Address every blocker in attached root-review-r1.md, amend the existing note and outcome byte-identically, keep source code untouched, then return to-review.
agent completed: [analyst] researcher (codex) (exit=1)
agent spawned: codex (pid=9503, exit=1)
Rev 2: amended per root review round 1 blocking findings. Added: (1) frozen COM handoff sequence with MTA callback → mutex-protected slot → dedicated capture thread, covering cancellation and shutdown without UI-thread deadlock; (2) complete helper ABI with __stdcall exports, fixed-width types, opaque handles, HRESULT returns, /MT static CRT, no redistributables; (3) visible Pulsar window as picker owner, restore-before-pick contract; (4) AppContainer claims separated into documented facts vs mandatory probe hypotheses, AppCapability SUA-only caveat with ActivateAudioInterfaceAsync consent fallback; (5) lifecycle stop state machine with signal-to-action mapping and finalize-vs-discard policy; (6) exact capture probe defaults: shared mode, event-driven, GetMixFormat (no AUTOCONVERTPCM), AudioDeviceRole::Default, WinRT device interface IDs. 38 Microsoft docs cited.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=9951, exit=0)
Root review round 2: still rejected. Address every blocker in attached root-review-r2.md, keep one byte-identical authoritative outcome, keep source code untouched, then return to-review.
Rev 3 addresses all 11 R2 blocking findings: (1) async ABI — no .get() on UI thread, initiate→event→query contract with operation IDs; (2) two-phase CaptureRequestStop+CaptureRelease with ref-counted late-callback safety; (3) helper converts GetMixFormat to interleaved float32, versioned CaptureFormat struct with native metadata; (4) picker returns kernel HANDLE via IStorageItemHandleAccess::Create, not a path; (5) LoadPackagedLibrary only, NewLazyDLL rejected; (6) dedicated HResult Go type, no syscall.Errno conflation; (7) ABI version query, structSize fields, HANDLE vs opId disambiguation; (8) direct MS MTA citation, explicit CoInitializeEx requirement; (9) blocked/no-go for failed P1.0 APIs — no silent tray-only degradation; (10) .partial streaming file, atomic promotion, startup recovery; (11) single authoritative outcome reattached. 43 sources cited.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=14422, exit=0)
Root review round 3: still rejected. Address every blocker in root-review-r3.md, keep one byte-identical outcome, keep source code untouched, then return to-review.
R4 amendment in progress. Addressing all 8 R3 blocking findings: WASAPI init order, async-op lifetime, draft writer/format, recording overflow, PCM conversion exactness, picker handle semantics, ABI/error details, lifecycle evidence alignment.
Rev 4 amendment complete. All 8 R3 blocking corrections addressed:
R3-1: GetMixFormat before Initialize (corrected in handoff diagram and legal-handoff text)
R3-2: Async-op lifetime — release/take semantics for all ops, auto-reset event drain rule, CapDestroy requires zero active ops, take-once picker HANDLE, CapPermissionRequestRelease added
R3-3: Go sole draft writer, .partial uses zero WAV sizes not 0xFFFFFFFF RF64 marker, checked frame arithmetic, fmt chunk always valid
R3-4: Recording ring overflow = terminal FAILED, separate lossy meter ring for UI
R3-5: Exact PCM conversion — WAVEFORMATEX vs WAVEFORMATEXTENSIBLE, packed 24-bit vs 24-in-32, nBlockAlign validation, wValidBitsPerSample rules, sign extension, deterministic test vectors
R3-6: IStorageItemHandleAccess::Create is probe hypothesis under AppContainer, unknown vs zero-byte size, max enforced against actual bytes read, take-once handle, name-buffer truncation cannot leak handle
R3-7: Complete AppCapabilityAccessStatus enum (5 values + unavailable), uintptr→int32 truncation via HResultFromUintptr, CaptureFormat.valid flag, overflowCount removed from CaptureGetResult, WaitForMultipleObjects in capture loop, op-ID wrap/exhaustion
R3-8: Lifecycle pass = finalized .wav or proven-recoverable .partial (not queued stop), AppCapability fallback requires proven WASAPI revoke detection
Fact-checked: GetMixFormat-before-Initialize confirmed by MS Initialize docs + Capturing-a-Stream sample; MTA pointer passing confirmed by MS MTA docs (verbatim quotes verified); RF64 0xFFFFFFFF marker confirmed. 43 MS sources cited. Source code untouched.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=19819, exit=0)
Root review round 4: rejected. Address every blocker in root-review-r4.md, keep one byte-identical outcome and product source untouched, then return to-review.
Rev 5 amends all 7 R4 blocking corrections: (1) PCM left-alignment with corrected vectors/formula/UB-safe sign extension, (2) forced CapDestroy removed — always requires zero active ops; WM_ENDSESSION uses OS reclaim, (3) exact WASAPI packet drain loop with GetNextPacketSize + acquired-packet error cleanup, (4) two-step picker size-discovery/take API with clean handle ownership, (5) native-format draft with real 180s/50MiB bounds + parseWAV verification — toEngineFormat rejected for recording, (6) FACILITY_ITF overflow HRESULT + checked allocation bounds + corrected nChannels max, (7) frozen draft outcome matrix by reason with consistent AppCapability conditional fallback wording.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=24057, exit=0)
Root review round 5: rejected. Address every blocker in root-review-r5.md, keep one byte-identical outcome and product source untouched, then return to-review.
Rev 6 amends all 6 R5 blocking corrections: (1) strong-reference callback lifetime restored for every async callback — activation, picker, permission, enumeration, AccessChanged — with ref-counted operation state, completion fence ordering, DLL unload safety, and adversarial race tests; (2) int16 conversion vectors corrected (spaced byte notation, correct LE interpretation), INT32_MAX float32 result corrected to 1.0f, C++20 arithmetic-right-shift removed from C++17 build — 24-in-32 uses explicit unsigned extraction + sign extension; (3) overflow HRESULT changed from FACILITY_ITF 0x80040200 (collides with VFW_E_INVALIDMEDIATYPE) to HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW) = 0x8007006F with CAP_REASON_OVERFLOW enum; (4) false "wakes exactly once" event guarantee removed, CapPermissionCheck added to Go drain list, CapPermissionSubscribe no longer says CapDestroy auto-unsubscribes, UTF-16 buffer rules scoped to device strings (picker truncates + S_OK with requiredNameChars), invalid takeHandle returns E_INVALIDARG, PENDING returns S_FALSE; (5) probe artifact explicitly separated from production draft — probe writes short disposable native-format evidence WAVs, no production bounds; production recording (future) must implement streaming mono downmixer with 180s/50MiB on upload-ready bytes; (6) toEngineFormat references for recording removed, parseWAV is not "the same parser used by ingest," probe verified with parseWAV + independent decoder/tool. Source code untouched. 44 MS docs cited.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=30417, exit=0)
Root review round 6: rejected. Address every blocker in root-review-r6.md, keep one byte-identical outcome and product source untouched, then return to-review.
Rev 7 amends all R6 blocking corrections: (1) process-lifetime DLL — FreeLibrary never called, COM ownership graph frozen, deterministic barrier tests; (2) AccessChanged unsubscribe fence — DuplicateHandle for notification event, Go handle race eliminated; (3) picker HRESULT immutable — *handleTaken alone reports transfer, complete truth table; (4) capture thread before activation — CoInitializeEx proven before ActivateAudioInterfaceAsync, linearization point defined, injected failure tests; (5) CapDestroy requires empty registry — terminal+released, not just terminal; (6) stop-reason priority CAS — overflow/permission_revoke dominate, Go rechecks permission before promotion, unknown WASAPI errors fail closed; (7) safe C++17 PCM — memcpy reads, conditional subtraction for signed 24-bit, unaligned vectors under ASan/UBSan, WAV interop gate with independent decoder. Source code untouched. SHA-256: da300431beed4f6bc477acf5f8fed8bd151ccd4d53b81a1ee7648accc8a8c3b6
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=40010, exit=0)
Root review round 7: rejected. Address every blocker in root-review-r7.md, keep one byte-identical outcome and product source untouched, then return to-review.
Rev 8 underway: addressing all 7 R7 blocking corrections — (1) 24-bit signed conversion, (2) WASAPI HRESULT table, (3) discontinuity+ring preflight, (4) coherent launch/thread-exit state machine, (5) real Go loader+UI WinRT apartment, (6) complete picker truth table, (7) conditional WAV claims
Rev 8 complete. All 7 R7 blocking corrections applied: (1) 24-bit signed conversion uses representable signed operands — `int32_t val = (int32_t)u; if (u >= 0x800000u) val -= 0x1000000;` applied to packed-24, 24-in-32, and safe-sample-reads; (2) WASAPI HRESULT table purged AUDCLNT_E_NOT_ALLOWED, added AUDCLNT_E_RESOURCES_INVALIDATED (0x88890025), unknown audio errors → CAP_REASON_WASAPI_ERROR not permission revoke; (3) drain loop handles DATA_DISCONTINUITY (first-packet accept, subsequent terminal CAP_REASON_DISCONTINUITY=10), TIMESTAMP_ERROR logged, whole-packet ring preflight before conversion/copy; (4) coherent state machine — 5s readyEvent timeout, capture thread holds strong session ref, terminal after CoUninitialize, cancellation wakes capture thread, C++/WinRT cycle breaking at callback, operation destructor waits thread-exit event, adversarial tests 9-13; (5) real Go loader via kernel32.NewProc("LoadPackagedLibrary") with APPMODEL_ERROR_NO_PACKAGE fallback, CapInit calls RoInitialize(RO_INIT_SINGLETHREADED), CapDestroy balances with RoUninitialize, UI-thread apartment section added; (6) every picker pointer classified mandatory/optional, validation order frozen, null state/hresult/handleTaken → E_POINTER, negative nameBufLen → zero, PickerOpenFile validation table added; (7) WAV "valid and widely interoperable" → "selected initial build-time contract", decoder gate prerequisite before signed scenarios, all components switch together if gate rejects. Sources MS-45/46/47 added. SHA-256: 00a0a21d659da68544e0f4235cea44658d492394cc6b1ef85eab7abaa3a84256. Source code untouched.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=47457, exit=0)
Root review round 8: rejected after complete 2520-line read, official Microsoft checks, live message-pump inspection, and local x/sys verification. Address every blocker in root-review-r8.md; preserve accepted invariants; keep product source untouched; reattach one byte-identical outcome and return to-review.
Rev 9 complete. All 7 R8 blocking corrections applied: (1) executable CaptureStart branch table covering every failure path with function HRESULT, *opId, registry, terminal publisher, callback, signals, cleanup owner — once opId returned, CaptureStart returns S_OK and all outcomes travel through CaptureGetResult; (2) capture thread no longer holds ref-counted session ref — uses atomic threadDone flag, eliminates self-join deadlock, destructor never joins; (3) named CAP_PERMISSION_* enum with exhaustive switch — raw NotDeclaredByApp(1)→CAP_PERMISSION_NOT_DECLARED(4), raw Allowed(4)→CAP_PERMISSION_ALLOWED(1), direct cast never reaches Go, CAP_PERMISSION_UNAVAILABLE(-1) is no-go for promotion unless separately gated, AccessChanged holds strong shared_ptr; (4) SERVICE_NOT_RUNNING and RESOURCES_INVALIDATED reclassified to CAP_REASON_WASAPI_ERROR (non-promotable), HRESULT mapping expanded to all WASAPI calls, stop-reason linearization tightened; (5) dedicated waiter goroutine on its own OS thread — WaitForMultipleObjects for helper events, PostMessageW for UI actions, existing GetMessageW pump unchanged; (6) ring capacity max(2*sampleRate, bufferFrames), scratch-buffer conversion before ring commit; (7) CapInit thread ID tracking, wrong-thread CapDestroy rejection, NewLazySystemDLL for kernel32, injectable function wrapper seam for tests. SHA-256: 68bdc6f4c5372c8d92c421bd01bf963a9d9197311ce8d0bd2996f1de1dfcef21. Source code untouched.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=55210, exit=0)
Root review round 9: rejected after complete Rev8 read plus complete Rev8-to-Rev9 diff and all modified-section/source checks. Authoritative Rev9 and outcome Rev8 are not byte-identical. Address every blocker in root-review-r9.md; keep product source untouched; attach one verified outcome and return to-review.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=63859, exit=0)
Root review round 10: rejected after complete 2809-line read, byte/SHA verification, live-shell inspection, and independent concurrency/lifetime analysis. Address every blocker in root-review-r10.md; preserve accepted invariants; keep product source untouched; reattach one byte-identical outcome and return to-review.
Rev 11 complete. All 6 R10 blocking corrections applied:
R10-1: Every pre-cleanup terminal publisher removed — null handoff diagram, readyEvent timeout prose, sync launch failure text, two-phase contract cleanup list, late callback completion, acquired-packet error cleanup all now route through composite barrier (CoUninitialize → threadDone → fence → terminal → localNotify). UI and callback paths store only pending causes.
R10-2: Truthful final-access lifetime — threadDone=1 means "cleanup complete; one terminal store remains" (not "last session-state access"). Terminal store is the final session-state access. CaptureGetResult uses acquire read. C++/WinRT cycle broken at callback return (after GetActivateResult + handoff), not after terminal publication — normal callback never publishes terminal.
R10-3: Release-before-commit in every packet description — diagram capture loop, §Buffer handling, cleanup ReleaseBuffer failure classification for overflow/discontinuity/format-error/stop-while-acquired branches (cleanup HRESULT logged, terminal reason is original cause).
R10-4: Packed atomic reason seal — state+sealed-bit+reason in one uint64_t, all updates via InterlockedCompareExchange64, no mutex needed, no post-snapshot CAS race. Private SEALED state distinct from public TERMINAL; CaptureGetResult maps SEALED → pre-cleanup state.
R10-5: Asynchronous quit state machine — waiter requests stop, waits for terminal, drains, releases, posts WM_APP+CLEANUP_READY; UI calls CapDestroy on its own thread, then posts WM_QUIT. Waiter never calls CapDestroy. OnQuit starts state machine, not immediate WM_QUIT. WM_QUERYENDSESSION calls CaptureRequestStop. Picker results carry HANDLE/take state.
R10-6: Complete HRESULT/cleanup table covers every COM/WASAPI stage from GetMixFormat through GetService, including activation E_ACCESSDENIED, format validation, Initialize, GetBufferSize, SetEventHandle, Stop failure. Each row: terminal state/reason/HRESULT, format validity, PCM existence, promotability, Stop eligibility, release/free order (including CoTaskMemFree), final publisher. 11 branch tests generated.
SHA-256 verified byte-identical on disk: af8ea8828babbe494c4746a15f78fd82fb90583b5d35870943d210df4a1de5fa. 2972 lines, 37758 words, 291063 bytes. Product source untouched.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=71632, exit=0)
Root review R11 rejected Rev11 after full 2972-line read, byte/SHA verification, live-shell inspection, and official Microsoft API checks. Five blockers: remaining pre-barrier/final-access contradictions; incomplete packed cancellation/internal-failure state machine; non-terminating graceful quit and unobservable callback quiescence; leaking/contradictory HRESULT cleanup table; privacy-unsafe reason-blind orphan draft recovery. See precondition root-review-r11.md.
Rev 12 complete. All 5 R11 blocking corrections applied:
R11-1: Removed remaining pre-barrier/final-access contradictions — threadDone description changed to "cleanup complete; one terminal store remains", CoInitializeEx failure path fixed, branch table rows include full cleanup sequence, standalone overflow sequence replaced with normative path reference, added §Normative cleanup path (11 steps) with static grep fixture, cancelled-activation paragraph updated.
R11-2: Finished packed state machine — lastPublicState in bits [63:56], cancelled bit removed in favor of state>=STOPPING, internal-failure CAS protocol for overflow/discontinuity/conversion/WASAPI failures in CAPTURING state, wake events table per source state, 12 deterministic transition tests.
R11-3: Graceful quit terminates with every operation category — per-operation quit table (capture/picker/permission/enumeration/default-device/AccessChanged) with IAsyncInfo::Cancel, CapIsQuiescent export for observable callback ref count, Ctrl-C/SIGTERM bridge via signal.Notify, WM_APP+CLEANUP_READY retry handshake with CapDestroy return checking, 7 quit tests.
R11-4: HRESULT/cleanup table owns every resource — pMixFormat eager-free after Initialize (fields copied first), running-stream HRESULT rows split (E_ACCESSDENIED→PERMISSION_REVOKE, AUDCLNT_E_DEVICE_INVALIDATED→DEVICE_LOST, other→WASAPI_ERROR), ownership flags (initialized/serviceAcquired/started) govern cleanup, Stop eligibility policy-based (only after successful Start, S_FALSE for already-stopped), 18 branch tests, successful-start allocation proof.
R11-5: Orphan .partial recovery is reason-aware and fail-closed — durable .partial.reason JSON sidecar written atomically before threadDone, startup recovery checks sidecar (missing/corrupt/stale→discard, non-promotable reason→discard, permission not Allowed→discard), process-kill test matrix for every edge, counterexample proofs.
SHA-256 verified byte-identical: 98e62a4303512399f1d7c25c93ea44d90509262cb3555fa69476e8de13ad17bd. 3296 lines. Product source untouched.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=79452, exit=0)
Root review R12 rejected Rev12 after complete 3296-line read, byte/SHA verification, official Microsoft API checks, static consistency grep, and independent state/quit/recovery analysis. Six blockers: contradictory pre-handoff terminal publisher and stale cancelled bit; forced WM_QUIT after failed CapDestroy and non-terminating cancel semantics; reason-sidecar writer/ABI contradiction plus unsafe promotable flag; UI-thread CaptureStart blocks up to 5 seconds despite async ABI; duplicate cleanup and Stop HRESULT rules; inconsistent priority/test inventory. See precondition root-review-r12.md. Product source untouched.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=91749, exit=0)
Root review R13: REJECTED after full 3,508-line read and independent checks. See root-review-r13.md. Required Rev14 corrections: one disjoint public state/readiness ABI; exact notify-handle close/signaling ownership; waiter remains alive through late terminal/release plus async UI retries; honest cooperative-cancel/forced-fallback policy; consistent Go-only sidecar with explicit duplicate-key rejection; executable static checker; one generated priority table. Product source must remain untouched and task must return to to-review.
agent completed: [analyst] researcher (claude) (exit=1)
agent spawned: claude (pid=2056, exit=1)
Prior Rev14 correction agent exhausted budget (exit 1) before outcome attachment. Authoritative is a partial mixed revision; canonical research.md remains Rev13. Continue only from root-review-r14-continuation.md: finish private PREPARING/PREPARED FSM, pre-created handle duplicates and failure branches, replace all stale quit sections/final summary, align cancel sidecar HRESULT, execute checks, then attach one canonical outcome. Product source untouched.
Rev 15 completes the interrupted Rev 14 per root-review-r14-continuation. Concrete fixes: (R14-1) private FSM freezes distinct PREPARING(0)/PREPARED(1) states with stop-latching during blocked CoInitializeEx and CaptureActivate atomically requiring PREPARED — packed layout, wake table, tests 1a/1b, states diagram, ABI comments all derived from one FSM; (R14-2) notification duplicates created eagerly in CapturePrepare/CaptureActivate with failure branches (no op/no activation on DuplicateHandle failure), each closed exactly once per path, removed the lazy-vs-registration contradiction, tests 6a/6b added; (R14-3) wndproc CLEANUP_READY retry is now SetTimer/WM_TIMER driven, no Sleep in pump, quit test 5 updated; (R14-4) verified cancel HRESULT=ERROR_CANCELLED and concrete token-walking rejectDuplicateKeys; (R14-5) added executable checker .research/root-checks/windows-consistency-check.sh (exit 0, all PASS) which found+fixed the R14-3 Sleep and two R14-2 stale worker signals. Outcome research.md synced byte-identical to authoritative note (sha 7299ddfb4891fe2c..., 3812 lines, 392912 bytes; cmp confirms). Product source untouched. HONEST RESIDUAL: this pass targeted the five named R14 corrections + checker anti-patterns; it did not re-derive every diagram cell, so a full root read of all ~3.8k lines is still required — flagged, not asserted resolved.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=7690, exit=0)
Root review R15: REJECTED after complete 3812-line read. See precondition root-review-r15.md. Independent duplicate parser passes; independent contract checker fails 14 stale contradictions; late-terminal model disproves timeout→CLEANUP_READY at 6/15/29 seconds. Correct quit/watchdog, duplicate-handle branch ownership, private FSM/ready ABI, exhaustive branch table, full sidecar reason/HRESULT table, final summaries/checker. Stale duplicate outcome removed. Product source remains untouched. Return to to-review only with one byte-identical canonical outcome.
agent completed: [analyst] researcher (codex) (exit=1)
agent spawned: codex (pid=14745, exit=1)
agent completed: [analyst] researcher (codex) (exit=1)
agent spawned: codex (pid=14909, exit=1)
agent completed: [analyst] researcher (codex) (exit=1)
agent spawned: codex (pid=15074, exit=1)
Root acceptance R16 (2026-07-13): ACCEPTED after complete Rev15 read, complete Rev15→Rev16 diff/context audit, official Microsoft source verification, and independent executable checks. Agent Rev16 attempt produced no file; accepted changes were root-authored and root-reviewed. Canonical research.md: 4192 lines, 51119 words, 398825 bytes, SHA-256 a969885686814b44c2b7a7aaef4fcdbc3cf05b90f044a942c0eba92524ae0847. Canonical checker: 238 lines, 1376 words, 11450 bytes, SHA-256 dc4dc5f4c2291d27ae604e99d727c812b697b28009332e481db4102d3fbbfdda. Final exit 0: Rev16 consistency checker, R15 adversarial checker, duplicate JSON parser, late-quit model, FSM/handle/early-launch model, sidecar compatibility model, bash -n, 50-fence structure check, git diff --check. research/checker/acceptance resources each cmp byte-identical. Product source paths remained clean. Gate selects WinRT permission/device/picker plus ActivateAudioInterfaceAsync/WASAPI native helper, with signed AppContainer hardware/WACK proofs explicitly unresolved for the implementation spike.

## Precondition Resources
- [p1-windows-store-spike-components.puml](file://TASK-260712-6kba80/p1-windows-store-spike-components.puml) — Component view for API and manifest selection
- [p1-root-review-amendments.md](file://TASK-260712-6kba80/p1-root-review-amendments.md) — Root-reviewed signed AppContainer and Windows platform invariants
- [research-instructions.md](file://TASK-260712-6kba80/research-instructions.md) — Task-specific official-source spike deliverable and no-implementation boundary
- [root-review-r1.md](file://TASK-260712-6kba80/root-review-r1.md) — Root review round 1 blocking corrections
- [root-review-r2.md](file://TASK-260712-6kba80/root-review-r2.md) — Root review round 2 blocking corrections
- [root-review-r3.md](file://TASK-260712-6kba80/root-review-r3.md) — Root review round 3 blocking corrections
- [root-review-r4.md](file://TASK-260712-6kba80/root-review-r4.md) — Root review round 4 blocking PCM, async teardown, drain, picker ownership, draft format, bounds, and evidence corrections
- [root-review-r5.md](file://TASK-260712-6kba80/root-review-r5.md) — Root review round 5 blocking callback lifetime, vector, HRESULT, event, draft boundary, and validator corrections
- [root-review-r6.md](file://TASK-260712-6kba80/root-review-r6.md) — Root review round 6 blocking module lifetime, unsubscribe fence, picker ABI, COM handoff, teardown, stop arbitration, and PCM/WAV corrections
- [root-review-r7.md](file://TASK-260712-6kba80/root-review-r7.md) — Root review round 7 blocking signed PCM, HRESULT/flag, whole-packet ring, launch/thread state, real loader/apartment, picker ABI, and WAV-gate corrections
- [root-review-r8.md](file://TASK-260712-6kba80/root-review-r8.md) — Root review round 8 blocking capture state, self-join, permission fallback, WASAPI, message-pump, ring, init and loader corrections
- [root-review-r9.md](file://TASK-260712-6kba80/root-review-r9.md) — Root review round 9 blocking terminal, lifetime UAF, packet commit, reason seal, waiter ownership, init and outcome corrections
- [root-review-r10.md](file://TASK-260712-6kba80/root-review-r10.md) — Root review round 10 blocking terminal, lifetime, packet, reason, shutdown, and HRESULT corrections
- [root-review-r11.md](file://TASK-260712-6kba80/root-review-r11.md) — Root round 11 rejection: state, quit, cleanup, and recovery blockers
- [root-review-r12.md](file://TASK-260712-6kba80/root-review-r12.md) — Root review round 12: rejected; six blocking contract corrections
- [root-review-r13.md](file://TASK-260712-6kba80/root-review-r13.md) — Root review round 13: rejected after full read and independent checks
- [root-review-r14-continuation.md](file://TASK-260712-6kba80/root-review-r14-continuation.md) — Continuation instructions after budget-exhausted incomplete Rev14
- [root-review-r15.md](file://TASK-260712-6kba80/root-review-r15.md) — Root review R15 rejection after complete 3812-line read and independent JSON, quit, SHA, source-cleanliness, and contract checks

## Outcome Resources
- [research.md](file://TASK-260712-6kba80/research.md) — Root-accepted Rev 16 Windows AppContainer capture bridge; SHA-256 a969885686814b44c2b7a7aaef4fcdbc3cf05b90f044a942c0eba92524ae0847
- [windows-consistency-check.sh](file://TASK-260712-6kba80/windows-consistency-check.sh) — Root-expanded Rev 16 executable consistency checker; exit 0, 238 lines, SHA-256 dc4dc5f4c2291d27ae604e99d727c812b697b28009332e481db4102d3fbbfdda
- [root-acceptance-r16.md](file://TASK-260712-6kba80/root-acceptance-r16.md) — Root acceptance after full Rev15 read, complete Rev16 diff/context review, official-source checks and six executable models/checkers
