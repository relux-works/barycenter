# Root review round 3 — Windows AppContainer bridge

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Verdict: close, but the rev-3 contract still contains runtime and data-integrity
contradictions.

## Blocking corrections

1. **Fix WASAPI initialization order.**
   `GetMixFormat()` must run before `IAudioClient::Initialize`, because its
   returned format is the format passed to shared-mode `Initialize`. The current
   sequence has them reversed in the handoff diagram and scenario text. Freeze:
   activation → `GetMixFormat`/validate → `Initialize` → `SetEventHandle` →
   `GetService` → `Start`, with same-thread cleanup for every failure point.

2. **Complete async-operation lifetime management.**
   Add release/take semantics for every initiated operation; permission request
   currently has no Release. Define auto-reset/manual-reset notification event
   behavior and the drain loop after a coalesced signal. `CapDestroy` cannot
   synchronously free state while picker/permission/activation callbacks are
   un-cancellable and pending. Either require zero active operations and return
   `E_NOT_VALID_STATE`, or make global shutdown ref-counted/two-phase. No callback
   may touch freed context. A picked HANDLE must transfer exactly once; repeated
   `PickerGetResult` calls must not return the same owned handle twice.

3. **Choose one draft writer and make the file format valid.**
   The note says the helper writes `.partial`, but the ABI gives it no file and
   the responsibility table says Go writes drafts. Freeze Go as the only draft
   writer: it continuously drains float32 frames and writes app-private partial
   data. A RIFF/WAV `data` size of `0xFFFFFFFF` is an RF64 marker and is not a
   valid ordinary WAV placeholder without RF64 metadata. Use a clearly invalid
   private partial header with stored format metadata (for example zero sizes)
   and repair from actual file length, or implement valid RF64. Finalization is:
   terminal capture → drain all remaining frames → rewrite header →
   `FlushFileBuffers`/close → atomic rename. Startup recovery uses the exact same
   validated metadata and never treats truncated frames as valid.

4. **Do not hide recording data loss.**
   Dropping oldest unread samples on ring overflow and then finalizing a draft
   as successful produces corrupted, discontinuous audio. For the recording
   stream, overflow must transition to terminal failure and mark/delete the
   partial draft; evidence records the overflow. A separate lossy meter buffer
   may drop samples, but it cannot be the recording source. Define ring capacity,
   checked frame/channel arithmetic, and maximum capture packet size.

5. **Make native PCM conversion exact.**
   Cover both `WAVEFORMATEX` and `WAVEFORMATEXTENSIBLE`, including packed 24-bit
   versus 24 valid bits in a 32-bit container, `nBlockAlign`, sign extension,
   `wValidBitsPerSample`, channel count/mask, and unsupported formats. A generic
   “int24” row is not sufficient. Add deterministic conversion vectors to the
   required probe/unit evidence.

6. **Tighten picked-file result semantics.**
   Treat `IStorageItemHandleAccess::Create` inside this exact signed AppContainer
   as a mandatory probe hypothesis, not a fully proven fact. Distinguish unknown
   size from a real zero-byte file, enforce the maximum against actual bytes
   while reading (not only `GetFileSizeEx`, which is racy/optional), and specify
   take-once handle transfer plus close-on-error. A too-small name buffer must
   not accidentally transfer or leak the file handle.

7. **Finish ABI/error details.**
   Map the complete `AppCapabilityAccessStatus` enum without collapsing cases.
   When Go receives an HRESULT in a `uintptr`, explicitly truncate to the low
   32 bits before converting to signed `int32`; testing `uintptr < 0` is never
   valid. Use versioned result structs or explicit validity flags so a failed
   activation does not claim `CaptureFormat` is populated. Correct pseudo-code
   to `WaitForMultipleObjects` for data/stop events and define operation-ID
   wrap/exhaustion behavior.

8. **Align lifecycle evidence with asynchronous reality.**
   Shutdown/suspend/lock only pass after either a finalized `.wav` exists or a
   deliberately retained partial is proven recoverable on next launch. A queued
   stop is not success. `AppCapability` fallback is acceptable only if the
   mandatory real-hardware revoke test proves deterministic WASAPI failure;
   otherwise the probe is blocked as already stated.

## Resubmission

- Amend and reattach the single authoritative note; source code remains
  untouched.
- Preserve the accepted async ABI direction, two-phase capture lifetime, MTA
  proof, visible picker owner, package-bound loader, no-fallback platform gates,
  signed-hardware evidence matrix, and AppContainer posture.
- Return to `to-review`, never `done`.
