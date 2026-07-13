# TASK-260712-dib11l — root review round 1 rework outcome

Date: 2026-07-13
Role handoff target: `to-review`
Native helper ABI: 1
`CaptureFormat` version / size: 2 / 52 bytes

## Outcome

The draft remains the dedicated `ReluxWorksLLC.PulsarProbe` validation package: x64 `packagedClassicApp` + `appContainer`, the reviewed three network capabilities plus `microphone`, no `runFullTrust`, and no broad/library filesystem capability. It is not the Store product identity and is not evidence of Partner Center certification.

The actual C++/WinRT helper and Go probe shell implement explicit Record permission, deterministic default/selected input capture, event-driven WASAPI drain, tray/`RegisterHotKey` control, hidden-window recording, visible-HWND `FileOpenPicker`, take-once readable handle transfer, fail-closed evidence artifacts, and structured scenario logs. Lifecycle stop evidence remains in the downstream lifecycle task as required.

## Round-1 corrections

1. `capture_started=pass` now requires a positively observed `CAP_STATE_CAPTURING`; terminal-first frames are drained and discarded without creating an artifact.
2. The ten-second evidence bound clips each crossing packet at the last whole-frame boundary, requests stop once, and drains/discards later buffered packets.
3. Promotion requires a live `AppCapability.AccessChanged` monitor. The gate runs before a promotable sidecar is created, closing the crash/recovery promotion gap. Periodic checks are logged as diagnostic polling, not as access-change events.
4. Stop reason/HRESULT publication is a packed lock-free CAS and never takes the activation handoff mutex. A production-export barrier test stalls that mutex and proves `CaptureRequestStop` returns.
5. Native tests now cover PREPARING/PREPARED cancellation, activation launch/cancel order, duplicate/thread launch failures, priority/seal races, zero-delay release after terminal notification, quiescence, in-flight permission handlers, picker ownership truth rows, and acquired-packet cleanup branches.
6. Cleanup preserves the primary terminal cause while recording `ReleaseBuffer` and `IAudioClient::Stop` failures; service, mix format, and client releases still run in reviewed order.
7. Every normal and recovered `.wav` is parsed and length-checked after rename before pass; invalid output is removed.
8. Partial startup initialization uses ordered rollback: helper destroy before Go-owned events, then windows and log. Destroy refusal leaves notification handles to process teardown.
9. All required child controls are mandatory, hotkey receipt is an attempt, hotkey pass requires an accepted record/stop action, and button/close/tray/picker-restore hide paths record capture-hidden evidence.
10. PowerShell wraps every native command with an explicit `$LASTEXITCODE` check, with a deliberate nonzero-command regression in Windows CI.

Additional hardening detects `WAIT_FAILED`, `GetMessageW == -1`, critical `PostMessageW` and timer failures; validates the exact capability set; reports individual device-info failures; and removes generated `.gocache`/`.build` trees before handoff.

## Task-scoped implementation files

- `.github/workflows/ci.yml`
- `pulsar-win/.gitignore`
- `pulsar-win/cmd/pulsar-win-probe/{main_stub.go,main_windows.go,startup_cleanup.go,startup_cleanup_test.go,window_windows.go,window_windows_test.go}`
- `pulsar-win/internal/winprobe/{artifact.go,artifact_test.go,capture_policy.go,capture_policy_test.go,helper_windows.go,helper_windows_test.go,loader_policy.go,loader_policy_test.go,log.go,manifest.go,manifest_test.go,native_contract_test.go,picker_read.go,picker_read_test.go,recovery.go,recovery_test.go,selection.go,selection_test.go,sidecar.go,sidecar_test.go,types.go,types_test.go}`
- `pulsar-win/native/pulsar-capture/{CMakeLists.txt,pulsar_capture.cpp,pulsar_capture.h,pulsar_capture_internal.h,tests/pulsar_capture_tests.cpp}`
- `pulsar-win/probe-msix/{AppxManifest.xml.in,README.md,build-probe.ps1,native-command.ps1,native-command.Tests.ps1}`

Unrelated pre-existing workspace changes were not edited.

## Checks run on macOS and passing

- `GOCACHE=/tmp/TASK-260712-dib11l-gocache go test -count=1 ./...`
- `GOCACHE=/tmp/TASK-260712-dib11l-gocache go test -race -count=1 ./internal/winprobe ./cmd/pulsar-win-probe`
- `GOCACHE=/tmp/TASK-260712-dib11l-gocache go vet ./...`
- `GOCACHE=/tmp/TASK-260712-dib11l-gocache CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`
- `GOCACHE=/tmp/TASK-260712-dib11l-gocache GOOS=windows GOARCH=amd64 go vet ./...`
- Windows/amd64 `go test -c` for `./internal/winprobe` and `./cmd/pulsar-win-probe`
- `go test -count=1 -run TestNativeFloatWAVIndependentDecoderGate -v ./internal/winprobe` (mono, stereo, 4-channel, 8-channel all passed through the available independent decoder)
- clean `gofmt -d`, `git diff --check`, `xmllint --noout` for the manifest, and Ruby YAML parse for CI
- generated-tree assertion: neither `pulsar-win/.gocache` nor `pulsar-win/.build` remains

## Explicitly unexecuted / unclaimed gates

This macOS host has no `pwsh`, `powershell`, `cmake`, `clang-cl`, or MinGW C++ compiler. Therefore the MSVC native compilation/CTest suite, PowerShell native-exit regression, staging, and MakeAppx package creation were not run locally. They are wired into the `windows-latest` CI job but no CI run is claimed here.

Signing, installed signed-MSIX execution, WACK, and real Windows 10 (19041+) / Windows 11 hardware scenarios were not run. Runtime permission, device, hotkey, picker, hidden-window, revoke, and signed-package evidence must remain fail/blocked until those gates execute; local cross-builds do not establish Store eligibility.
