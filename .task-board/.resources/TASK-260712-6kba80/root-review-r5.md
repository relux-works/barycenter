# Root review round 5 — Windows AppContainer bridge

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Verdict: rejected. Rev 5 fixes the broad R4 findings, but its callback lifetime,
numeric vectors, HRESULT, and draft/product contracts remain unsafe or false.

## Blocking corrections

1. **Restore a real strong-reference/completion-fence lifetime for every async
   callback.**
   Strict `CapDestroy` is necessary but does not make ref-counting unnecessary.
   The activation-cancel path sets terminal state and signals Go before the
   callback has returned; Go can immediately call `CaptureRelease` and free the
   session while callback code/its COM object is still executing. The same race
   exists for picker, permission, enumeration/default-device completions, and
   `AccessChanged` racing unsubscribe. Each callback must hold a strong operation
   reference until its final instruction/return, and release/query/destroy must
   either drop only the registry reference or observe a completion fence/refcount
   proving no callback can execute DLL code afterward. Define the exact order
   of terminal publication, event signaling, registry removal, callback-ref
   release, event unsubscription, and DLL unload. Add adversarial tests that
   wake Go and call Release/Unsubscribe immediately while the callback is held
   at a test barrier.

2. **Correct all remaining conversion vectors and float expectations.**
   The int16 table labels byte sequence `01 00` (shown as `0x0100` in the table's
   byte-oriented convention) as integer 256; little-endian `01 00` is 1. Its
   negative row is likewise not -256. Use unambiguous spaced byte sequences and
   actual ±1-LSB vectors (`01 00` → 1, `FF FF` → -1), plus separate 256 vectors
   if desired. State expected **float32** results: converting `INT32_MAX` with a
   float32 divisor rounds to `1.0f`, not the displayed near-one double value.
   Make the tests bit-exact where appropriate and tolerance-based only where a
   specific floating operation requires it. Remove the C++20 guarantee claim
   from a C++17 build or implement signed right-shift extraction without relying
   on implementation-defined pre-C++20 behavior.

3. **Do not invent a globally non-colliding `FACILITY_ITF` HRESULT.**
   `0x80040200` is already Microsoft `VFW_E_INVALIDMEDIATYPE` in DirectShow;
   `FACILITY_ITF` is shared and does not guarantee global uniqueness. Keep the
   authoritative `CAP_REASON_OVERFLOW` enum and return a standard HRESULT such
   as `HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW)`/`E_FAIL`, or version a separate
   helper error namespace without claiming global uniqueness. Update every ABI,
   table, log expectation, and negative test.

4. **Remove the remaining event/subscription/ABI contradictions.**
   The notification section still says every `SetEvent` wakes once and Go wakes
   once per signal immediately before correctly explaining coalescing. Delete
   those false guarantees; an event is only a readiness hint. The Go drain list
   must include a fresh `CapPermissionCheck` for `AccessChanged`, not only the
   one-shot permission-request result. `CapPermissionSubscribe` still says
   `CapDestroy` unsubscribes, while the global contract says an active
   subscription makes destroy fail; choose the latter and require explicit
   unsubscribe/fence. General UTF-buffer rules still return
   `E_NOT_SUFFICIENT_BUFFER` for short file names while picker explicitly
   truncates and returns `S_OK`; scope the general rule away from picker and
   freeze invalid `takeHandle`, pending, repeat-take, and operation-HRESULT
   semantics without overloading successful operation outcome.

5. **Separate the native bridge/probe artifact from the production mono draft,
   or implement the production format now.**
   Rev 5 freezes native multichannel float WAV, then says the product needs a
   future mono downmixer. Applying the 50-MiB *upload* limit to this internal
   eight-channel float representation stops a recording at about 34 seconds,
   directly contradicting the text claiming the 180-second limit wins for sane
   inputs and the product's mono-capture intent. Choose one explicit boundary:
   the minimal platform probe may emit a short, clearly disposable native-format
   evidence WAV, while the later recording task must stream/downmix to a frozen
   canonical mono draft and enforce 180 s / 50 MiB on upload-ready bytes; or this
   contract must freeze and implement that canonical streaming path now. Do not
   call native probe files final user drafts. Correct the size arithmetic and
   ensure the write that crosses a limit is clipped at a whole-frame boundary,
   never allowed to overshoot.

6. **Remove stale repository claims and prove the chosen file with the right
   validator.**
   Several sections still say recording uses `toEngineFormat` despite the new
   decision correctly rejecting it. Remove all such paths from responsibility,
   scenarios, and final summary. `pulsar-win/voice.go::parseWAV` is a Windows
   playback parser, not the not-yet-built coordinator ingest validator; do not
   call it “the same parser used by ingest.” For a probe artifact, verify with
   the local parser plus an independent decoder/tool in the Windows evidence.
   For a production upload-ready file, the later ingest contract must validate
   it. If retaining 44-byte multichannel IEEE-float WAV, explicitly test the
   claimed decoder interoperability and channel-count/header behavior rather
   than asserting it from the app-private intent.

## Resubmission

- Amend and reattach the one authoritative note byte-identically; product
  source remains untouched.
- Preserve accepted package posture, WASAPI order/packet drain, MTA handoff,
  strict no-force destroy, package-bound loader, two-step picker ownership,
  left-aligned PCM conversion, checked bounds, no-fallback signed-hardware
  gates, and Go-only writing.
- Add callback barrier/refcount tests, corrected bit-level vectors, a
  non-colliding error contract, readiness-hint semantics, explicit probe-vs-
  production file boundary, and the correct validators.
- Return to `to-review`, never `done`.
