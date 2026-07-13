# Root review round 4 — Windows AppContainer bridge

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Verdict: rejected. Rev 4 closes the earlier outline gaps, but several exact
contracts would still produce wrong samples, leaked handles, or unsafe teardown.

## Blocking corrections

1. **Correct PCM valid-bit alignment, scaling, and test vectors.**
   Microsoft documents that when `wValidBitsPerSample < wBitsPerSample`, valid
   PCM bits are **left-aligned** in the container and unused least-significant
   bits are zero. Rev 4 says 24-in-32 uses the low 24 bits and gives
   `0x007FFFFF`/`0x00400000` vectors, yet then shifts right by 8; those vectors
   cannot yield the claimed values. The positive full-scale vector must occupy
   the high 24 bits (for example `0x7FFFFF00`), and signed scaling is by
   `2^(validBits-1)`, not `1 << validBits`. Fix the table, prose, formula, and
   vectors. Avoid signed-left-shift overflow/undefined behavior in packed
   24-bit sign extension: assemble in an unsigned type and convert with an
   explicit, tested sign-extension procedure. Cite the left-alignment rule and
   add boundary vectors for minimum, maximum, -1 LSB, +1 LSB, and silence.

2. **Remove the still-contradictory forced `CapDestroy` path.**
   The same zero-argument export is specified both to return
   `E_ILLEGAL_METHOD_CALL` with active operations and, under `WM_ENDSESSION`, to
   cancel them, return without waiting, and tolerate late callbacks. There is
   no ABI input that selects “force,” and an atomic destroyed flag cannot be
   read after the storage containing it has been freed. Choose one implementable
   rule. The minimal approved rule is: `CapDestroy` never frees with any active
   operation/subscription/callback; it returns the state error. On imminent
   process termination, request stop and return from the wndproc without
   calling destroy—the OS reclaims process resources. Otherwise define an
   explicit ref-counted asynchronous shutdown operation whose completion, not
   its initiation, permits final free. Include `AccessChanged` subscriptions
   and all WinRT completion handlers in the lifetime proof.

3. **Make event and WASAPI packet draining exact.**
   Auto-reset events do not wake “once per signal”: `SetEvent` calls can
   coalesce while already signaled. Treat notification as a level/hint and
   define the race-safe drain loop. For capture, each data wake must repeatedly
   call `GetNextPacketSize` and then `GetBuffer`/`ReleaseBuffer` on the same
   capture thread until packet size is zero; one `GetBuffer` per event can leave
   packets behind. Go must drain `CaptureRead` until `S_FALSE`, and query every
   known operation plus permission status after a coalesced notification.
   Specify error cleanup for a packet already acquired, overflow, stop racing a
   packet, and every failure point so no buffer remains unreleased.

4. **Fix picker result ownership when output buffers are insufficient.**
   The note says too-small UTF-16 buffers return `E_NOT_SUFFICIENT_BUFFER`, but
   also says `PickerGetResult` returns `S_OK`, truncates the name, and transfers
   the only file handle anyway. A caller following the general error rule can
   discard the outputs and leak the handle. The result also overloads the
   operation HRESULT with `E_HANDLE` after a successful take. Freeze a clean
   two-step/take API or an explicit `requiredNameChars` + `handleTaken` result:
   size discovery and insufficient-buffer calls must not transfer; a successful
   take transfers exactly once; operation outcome remains distinct from
   transfer state; every validation/error path either retains the helper-owned
   handle for a later take or closes it. Cover null/zero buffers, repeat take,
   release-before-take, and `PickerRelease` closing an untaken handle.

5. **Choose one valid, bounded streaming draft representation.**
   The note alternates between writing native capture rate/channels and using
   existing `toEngineFormat`. The current function in `voice.go` consumes a
   complete in-memory clip, allocates a whole stereo output, and resets its
   interpolation state per call; it is not a streaming recorder/resampler and
   it does not produce the spec's mono capture. A fixed 44-byte IEEE-float WAV
   also loses `WAVEFORMATEXTENSIBLE` channel layout for multichannel native
   input and needs its validity/interoperability contract proven. Freeze either
   (a) a new stateful streaming downmix/resampler and one canonical mono WAV
   format, or (b) a correctly described native-format WAV variant with all
   required chunks/layout metadata. Apply the real product bounds—180 seconds
   default and 50 MiB upload limit—not the rev-4 “e.g. 60 seconds”; stop at the
   first bound reached using checked actual bytes. Make startup repair use the
   exact chosen header, physically truncate incomplete frames before computing
   sizes, and prove the recovered result with the same parser/decoder used by
   ingest.

6. **Use non-colliding failure codes and real checked allocation bounds.**
   The proposed helper error `0x88890001` is already
   `AUDCLNT_E_NOT_INITIALIZED`; it cannot mean overflow. Either use the existing
   appropriate WASAPI code without redefining it or allocate a documented
   private `FACILITY_ITF` HRESULT plus a distinct terminal-reason enum. The
   assertion that channels are at most 255 is not derived from
   `WAVEFORMATEX` (`nChannels` is a 16-bit field). Freeze supported maxima for
   channel count, sample rate, block alignment, WASAPI buffer frames, ring bytes,
   and caller `maxFrames`; validate all multiplication/addition in a wide type
   before C allocation, Go slice creation, or ABI copy. On overflow failure,
   release the currently held WASAPI packet, stop/release COM on the capture
   thread, signal terminal state, and ensure Go never promotes the partial.

7. **Remove remaining lifecycle/evidence contradictions.**
   The AppCapability fallback is called merely “degraded but acceptable” in one
   section although the accepted rule is conditional: it is acceptable only
   after real hardware proves bounded deterministic WASAPI revoke failure.
   Likewise, draft invariants alternately count deliberate discard as a pass
   and require finalized/recoverable media. Freeze the matrix by reason and
   duration: valid user media requires finalized `.wav` or proven-recoverable
   `.partial`; permission revoke/explicit cancel/too-short capture may be an
   evidenced deliberate discard; queued stop alone is never a pass. Use the
   same wording in state machine, scenarios, unresolved proofs, and final answer.

## Resubmission

- Amend and reattach the one authoritative note byte-identically; product
  source remains untouched.
- Preserve the accepted package posture, `GetMixFormat`-before-`Initialize`
  order, MTA handoff, initiate/event/query direction, visible picker owner,
  package-bound loader, signed-hardware no-fallback gates, Go-only draft
  ownership, and terminal recording-overflow policy.
- Add corrected PCM vectors, the exact packet/event drain loop, picker take
  state table, teardown reference graph, canonical streaming-file decision,
  checked numeric bounds, and corresponding negative tests.
- Return to `to-review`, never `done`.
