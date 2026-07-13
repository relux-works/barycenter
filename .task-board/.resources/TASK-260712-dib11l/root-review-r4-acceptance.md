# Root review round 4 — TASK-260712-dib11l

Verdict: **accepted for the implemented probe scope** after independent root
review. The task may move through `reviewing` to `done`.

## Findings closed

1. Active evidence drafts now perform a production `Sync` at each crossed
   `sampleRate` frame threshold. Sync failures latch, stop further writes, and
   cannot finalize as pass. The Windows log calls the count `framesWritten`,
   not a false durability metric.
2. Abort, discard, final verification, normal sidecar cleanup, and recovery
   aggregate close/remove/verify failures and fail closed when their asserted
   filesystem postcondition is false.
3. The private diagnostics ABI is separately named and negotiated through
   `PulsarProbeDiagnosticsGetVersion` plus the versioned
   `PulsarProbeCaptureGetDiagnosticsV1` wire contract. The frozen public core
   header and core ABI v1 remain unchanged.
4. A live writer tracks only paths it actually created or renamed, records
   their file identity, uses exclusive creation and atomic no-replace promotion,
   and refuses stable replacement identities. Existing final, sidecar, temp,
   and partial paths are not overwritten or deleted by the writer. Recovery
   also refuses to replace a pre-existing final.

The identity checks plus no-replace operations are accepted for the probe's
private evidence directory and non-adversarial filesystem threat model. They
are not represented as a general security boundary against an attacker that
can continuously exchange directory entries inside the package data directory.

## Independent root validation

Passed on macOS after the delegated process exited:

- complete root read of the round-three durability, cleanup, ownership,
  recovery, diagnostics-extension, native contract, and Windows logging changes
- `git diff --check` and clean `gofmt -d pulsar-win/cmd pulsar-win/internal`
- `go test -count=1 ./...`
- `go test -race -count=1 ./...`
- `go vet ./...`
- Windows/amd64 cross-build and `go vet`
- Windows/amd64 test-binary compilation for `internal/winprobe` and the probe shell
- independent 1/2/4/8-channel float-WAV decoder gate
- both manifest XML parses and CI YAML parse
- Rev15/Rev16 consistency and contract checks
- Rev16 FSM, sidecar, JSON duplicate/parser, and graceful-quit models
- `task-board validate`
- no repository-local generated executable, test binary, CMake build, or cache output

## Explicitly unexecuted

This macOS host cannot establish the remaining native/package/hardware claims:
MSVC compilation and CTest, PowerShell native-command regression, MakeAppx and
staging, signing, WACK, installed MSIX execution, or real Windows 10/11 capture,
permission-revoke, picker, hotkey, lifecycle, and device scenarios. Those are
release/certification gates, not silently claimed by this acceptance.

No commit or push was performed.
