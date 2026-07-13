# Root review round 3 — TASK-260712-dib11l

Verdict: **rejected for rework**. Return to `to-dev`; do not mark `done`.

The root reviewer read the complete task-scoped Go shell/evidence code, C++/WinRT
helper, native tests, MSIX and CI after the second Sol Max rework, compared them
with the authoritative `windows-capture-bridge-r16.md` and both earlier root
reviews, and independently reran every check available on this macOS host. The
round-two blockers are materially improved, but the following durability and
contract gaps still make the evidence path capable of overstating success.

## Blocking findings

1. **The production writer never performs the required periodic durable flush.**
   Rev16 line 3281 requires `FlushFileBuffers` about once per second / every
   `sampleRate` frames while capture is active. `ArtifactWriter.Sync` exists at
   `pulsar-win/internal/winprobe/artifact.go:75`, but no production call site
   exists: the only calls are tests. `drainCapture` appends frames at
   `main_windows.go:687` and returns for nonterminal capture without syncing.
   A crash or forced process termination can therefore lose the entire buffered
   draft since creation rather than a bounded tail. Implement the periodic
   production policy and deterministic boundary/error tests; a finalization-only
   `Sync` is not a substitute for the interrupted-capture contract.

2. **Fail-closed deletion and verification can report an outcome while leaving
   the forbidden files behind.** There are several concrete variants:

   - `ArtifactWriter.Abort` returns immediately when `Close` fails
     (`artifact.go:87-92`), so it never even attempts to remove `.partial`,
     sidecar or final paths although the capture-query failure log says the
     artifact was discarded.
   - `discardAs` ignores the close error and ignores sidecar deletion failure
     (`artifact.go:171-184`), so a deliberate discard can be reported without
     verifying deletion of both required files.
   - Normal finalization ignores sidecar-removal failure and, after failed WAV
     verification, ignores failure to remove the invalid final file
     (`artifact.go:154-160`). Startup recovery has the same ignored invalid-final
     and sidecar deletion paths (`recovery.go:73-88`), while orphan-sidecar
     deletion is also silent (`recovery.go:90-97`). This violates the frozen
     invariants that every `.wav` in the draft directory is valid and deliberate
     discard verifies removal of both `.partial` and `.partial.reason`.

   Cleanup must be best-effort across every owned path even after an earlier
   error, aggregate/report all failures, and never emit `pass`/successful
   `discard` if the asserted filesystem postcondition is false. Add injectable,
   deterministic close/remove/sync/verify failure tests rather than relying on
   host permissions to make these branches happen.

3. **The production-required diagnostic extension is not explicitly versioned,
   while the handoff claims the frozen helper contract is unchanged.**
   `CaptureGetDiagnostics` is an exported DLL symbol and is mandatory in Go's
   `helperProcNames`, but it is absent from the authoritative Rev16 exported ABI
   and `pulsar_capture.h`; `CapGetVersion` still returns core ABI 1. Moving the
   declaration to `pulsar_capture_diagnostics.h` does not by itself create a
   negotiated contract: a valid core-v1 Rev16 DLL passes `CapGetVersion` and is
   then rejected by the loader for the extra mandatory symbol.

   The narrow private evidence extension is acceptable in principle, but it
   must be unmistakably separate and version-negotiated (for example a
   probe-prefixed, versioned symbol/extension contract validated independently
   from core ABI v1). Do not silently bump or modify the frozen public core ABI,
   and do not claim that core version negotiation covers the private extension.
   Add loader/contract tests for missing, wrong-version and matching extension.

## Round-two findings now accepted

- Result-query call failures are classified before zeroed output state and use
  operation-specific fail-closed stop/cancel/release ownership.
- Hidden/capture evidence now requires positive `CAPTURING` plus a hidden frame
  overlap; the picker restore latch covers synchronous initiation failures.
- Native timestamp and cleanup evidence reaches JSONL without replacing the
  sealed terminal cause.
- Activation Diagram A/B and permission-unsubscribe tests now drive the real
  production exports/registry/handlers through deterministic OS seams. The
  AccessChanged token is synthetic at the seam, which is correctly still not a
  real Windows hardware claim.

## Independent root checks

Passed on macOS:

- `git diff --check` and clean `gofmt -d ./cmd ./internal`
- `go test -count=1 ./...`
- `go test -race -count=1 ./internal/winprobe ./cmd/pulsar-win-probe`
- `go vet ./...`
- Windows/amd64 cross-build and `go vet`
- Windows/amd64 test-binary compilation for helper wrapper and shell
- independent decoder gate for mono, stereo, four- and eight-channel float WAV
- manifest `xmllint`, CI YAML parse, Rev15/Rev16 consistency/model/sidecar checks
- `task-board validate`; no repo-local generated build/cache/binary output found

Still unexecuted and unclaimed because this host has no Windows native toolchain:
MSVC compilation/CTest, PowerShell regression, MakeAppx/staging, signing, WACK,
installed MSIX, and Windows 10/11 packaged hardware scenarios.

## Rework handoff

Fix all three findings without weakening AppContainer or the frozen public core
ABI. Preserve unrelated user changes, add deterministic regression coverage,
rerun the strongest available checks, attach an honest outcome, return only to
`to-review`, and do not commit. Root review remains mandatory before acceptance.
