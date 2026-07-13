# Implementation handoff — packaged Windows platform probe

Task: `TASK-260712-dib11l`  
Use only after `TASK-260712-6kba80` has a root-approved outcome. The approved
bridge decision and `docs/analysis/p1-root-review-amendments.md` override all
older bridge revisions.

## Scope

Implement the smallest Windows-only signed-package probe for explicit
microphone permission, default/selected capture, tray/hotkey/window-hide
behavior, and brokered file picking. Keep lifecycle scenario implementation in
its dedicated downstream task. Do not implement coordinator identity/media APIs,
full upload/transmission UI, macOS changes, or production claims unsupported by
this probe.

## Non-negotiable invariants

- Preserve `packagedClassicApp` + `appContainer`; add only the reviewed
  microphone capability. Never add `runFullTrust`, broad/library filesystem
  capabilities, undocumented APIs, or developer-mode-only behavior.
- Implement the final reviewed native helper ABI exactly, including versioned
  structs, fixed-width values, package-bound loading, HRESULT conversion,
  explicit operation/release ownership, strong callback references/completion
  fences, and same-MTA capture handoff/release rules.
- `GetMixFormat`/validate precedes shared-mode `Initialize`, then
  `SetEventHandle`, `GetService`, `Start`. Every error path frees the mix format,
  releases any acquired packet, stops/releases COM on its owning thread, and
  signals one terminal result.
- Notification events are readiness hints; all WinRT operations and permission
  state are drained/rechecked, WASAPI packets loop until
  `GetNextPacketSize == 0`, and Go drains capture frames until `S_FALSE`.
- Recording-ring overflow is terminal and never produces a successful artifact;
  VU metering is a separate lossy path. Use the reviewed standard error plus
  terminal-reason contract and exact numeric/allocation bounds.
- Picker ownership is the visible main HWND. The result API discovers metadata
  without transfer, transfers a readable handle exactly once, and closes every
  untaken/error handle. Enforce file limit against actual bytes read.
- Load the packaged helper only through `LoadPackagedLibrary`; the unpackaged
  fallback is allowed solely for the reviewed no-package result and an absolute
  executable-directory `LoadLibraryExW` path.
- A failed AppContainer API is logged as fail/blocked evidence. Never substitute
  a silent tray/manual/full-trust path or claim signed Windows 10/11 proof from
  local cross-builds.
- Keep any native-format probe recording explicitly disposable evidence. Do not
  present it as the later production mono/upload-ready user draft.

## Required tests and evidence hooks

- Native ABI/version/struct-size/null/bounds tests and callback-barrier tests
  that release/unsubscribe immediately after notification without UAF.
- Bit-accurate PCM16, packed-24, left-aligned 24-in-32, PCM32 and float32 vectors,
  including min/max/±1 LSB/silence and unsupported formats.
- Event coalescing, packet-drain, stop-during-packet, activation-cancel, device
  error, acquired-buffer cleanup, ring overflow, and operation-ID tests.
- Picker metadata-only/take-once/repeat/release-before-take/null/short-name/
  actual-byte-limit/close-on-error tests with mockable handle ownership.
- Go `HResult` low-32-bit truncation and loader selection tests.
- Manifest/package payload assertions: helper DLL present, microphone declared,
  prohibited capabilities absent, correct x64 architecture.
- `go test ./...` in `pulsar-win`, Windows amd64 cross-build, native MSVC test
  build, package creation validation available in CI. Real signed MSIX/WACK/
  Windows 10+11 hardware scenarios remain explicit evidence tasks, not waived.

## Delivery discipline

- Touch only the probe/helper/MSIX/build/test files required by this task. Do not
  edit planning, research, task-board, coordinator, macOS, or unrelated code.
  Do not commit.
- Record changed files, ABI version, exact commands/results and all unexecuted
  hardware gates in the task outcome. Return to `to-review`, never `done`.
