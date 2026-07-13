# TASK-260712-dib11l — root review round 2 rework outcome

Date: 2026-07-13
Role handoff target: `to-review`
Native helper ABI: 1 (unchanged)
`CaptureFormat` version / size: 2 / 52 bytes (unchanged)

## Outcome

All six `root-review-r2.md` findings were reworked against the actual packaged-probe production paths. The draft remains the separate `ReluxWorksLLC.PulsarProbe` x64 validation identity with `packagedClassicApp` + `appContainer`, the reviewed three network capabilities plus `microphone`, no `runFullTrust`, and no broad/library filesystem capability. It is not the Store product identity and does not establish Partner Center eligibility.

The frozen Rev16 exports and struct negotiation are unchanged. Native timestamp and secondary-cleanup evidence uses a separate additive private header/export (`CaptureGetDiagnostics`) so the shell can copy evidence into `scenarios.jsonl` without replacing the primary terminal reason/HRESULT or altering the frozen public header.

## Round-2 corrections

1. Native deterministic tests now drive the real `CaptureActivate` registration, `ActivationHandler`, operation registry, duplicated capture/callback notification handles, Diagram A/Diagram B publisher order, immediate `CaptureRelease` after notification, callback quiescence, and exact signal/close counts. Permission tests call the real `CapPermissionSubscribe`/`CapPermissionUnsubscribe` exports through an injected OS-registration seam, validate a registered synthetic event token, race an in-flight dispatched handler with token revocation, close Go's original handle immediately, and prove the duplicated handle/strong reference survives until handler return.
2. `AUDCLNT_BUFFERFLAGS_TIMESTAMP_ERROR` is counted by the production packet drain, retained atomically on the real capture session, returned by the private diagnostic export, and emitted as accepted retrievable JSONL evidence. Tests cover packet detection, the production session/export route, and JSONL serialization/deduplication.
3. Cleanup `ReleaseBuffer` and `IAudioClient::Stop` failures use the same production evidence routing functions, remain secondary to the sealed terminal cause, are retrievable through the capture operation before release, and are emitted as separate JSONL failure evidence plus terminal fields. Native tests prove the primary cancel reason/HRESULT remains unchanged while both cleanup errors are retrieved.
4. Hidden-window evidence now requires positive `CAPTURING` observation plus a post-hide-drain captured-frame overlap. The sole waiter drains possibly pre-hide frames before opening a hidden epoch and checks actual HWND visibility again before crediting frames, preventing a queued restore from producing a false positive. Tests cover hide-before-start/restore-before-start, preparing-only, hidden-through-start, hide-during-capture, and picker restore.
5. Picker window ownership is a production state machine shared by synchronous initiation failures and asynchronous terminals. A tray-hidden main window is synchronously re-hidden when owner restoration or `PickerOpen` initiation fails; picked/cancelled/failed/query-failed asynchronous paths consume the same restore latch exactly once. Shell tests cover the full visible/hidden and sync/async restoration table.
6. Permission, enumeration, default-device, picker, and capture drains classify a failed call HRESULT before reading output state. On failure they log the exact query cause, ignore zeroed outputs, issue the real operation-specific cancel/stop where available, attempt the real release export, retain pending ownership for retry, and clear terminal/unknown operations. Capture query failure also deletes untrusted artifact/sidecar/final paths. Tests prove failed+zero-state classification, cancel-before-release order, pending ownership, and artifact deletion.

## Round-2 changed files

- `pulsar-win/cmd/pulsar-win-probe/main_windows.go`
- `pulsar-win/cmd/pulsar-win-probe/window_windows.go`
- `pulsar-win/cmd/pulsar-win-probe/picker_window_state.go`
- `pulsar-win/cmd/pulsar-win-probe/picker_window_state_test.go`
- `pulsar-win/cmd/pulsar-win-probe/query_failure.go`
- `pulsar-win/cmd/pulsar-win-probe/query_failure_test.go`
- `pulsar-win/internal/winprobe/artifact.go`
- `pulsar-win/internal/winprobe/artifact_test.go`
- `pulsar-win/internal/winprobe/capture_visibility.go`
- `pulsar-win/internal/winprobe/capture_visibility_test.go`
- `pulsar-win/internal/winprobe/diagnostic_log.go`
- `pulsar-win/internal/winprobe/diagnostic_log_test.go`
- `pulsar-win/internal/winprobe/helper_windows.go`
- `pulsar-win/internal/winprobe/native_contract_test.go`
- `pulsar-win/internal/winprobe/types.go`
- `pulsar-win/native/pulsar-capture/pulsar_capture.cpp`
- `pulsar-win/native/pulsar-capture/pulsar_capture_internal.h`
- `pulsar-win/native/pulsar-capture/pulsar_capture_diagnostics.h`
- `pulsar-win/native/pulsar-capture/tests/pulsar_capture_tests.cpp`

Unrelated pre-existing workspace changes were preserved. No commit was created.

## Commands run and passing on macOS

- `GOCACHE=/tmp/TASK-260712-dib11l-r2-gocache go test -count=1 ./...`
- `GOCACHE=/tmp/TASK-260712-dib11l-r2-gocache go test -race -count=1 ./internal/winprobe ./cmd/pulsar-win-probe`
- `GOCACHE=/tmp/TASK-260712-dib11l-r2-gocache go vet ./...`
- `GOCACHE=/tmp/TASK-260712-dib11l-r2-gocache CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`
- `GOCACHE=/tmp/TASK-260712-dib11l-r2-gocache GOOS=windows GOARCH=amd64 go vet ./...`
- `GOCACHE=/tmp/TASK-260712-dib11l-r2-gocache GOOS=windows GOARCH=amd64 go test -c -o /tmp/TASK-260712-dib11l-r2-winprobe.test.exe ./internal/winprobe`
- `GOCACHE=/tmp/TASK-260712-dib11l-r2-gocache GOOS=windows GOARCH=amd64 go test -c -o /tmp/TASK-260712-dib11l-r2-shell.test.exe ./cmd/pulsar-win-probe`
- `GOCACHE=/tmp/TASK-260712-dib11l-r2-gocache go test -count=1 -run TestNativeFloatWAVIndependentDecoderGate -v ./internal/winprobe` (mono, stereo, four-channel, and eight-channel passed through the available independent decoder)
- `gofmt -d ./cmd ./internal`
- `git diff --check`
- `xmllint --noout probe-msix/AppxManifest.xml.in`
- `ruby -e 'require "yaml"; YAML.load_file(".github/workflows/ci.yml")'`
- `bash .research/root-checks/windows-consistency-check.sh`
- `bash .research/root-checks/windows-r15-contract-check.sh`
- `go run .research/root-checks/windows-r16-fsm-model/main.go`
- `go run .research/root-checks/windows-r16-sidecar-contract/main.go`
- `task-board validate`
- Generated-tree inspection found no repo-local `.gocache`, `.build`, CMake `Testing`/`CMakeFiles`, native test binaries, probe EXE, or helper DLL.

## Explicitly unexecuted / unclaimed gates

This host has no `cmake`, `pwsh`, `powershell`, `clang-cl`, or MinGW Windows C++ compiler. Therefore the MSVC native helper compilation and CTest suite (including the new real callback/registration/export barriers), the PowerShell native-exit regression, staging, and MakeAppx package creation were not executed locally. Apple `/usr/bin/clang++` cannot compile the Windows SDK/C++/WinRT paths. No GitHub Actions run is claimed.

Signing, installed signed-MSIX execution, WACK, and real Windows 10 (19041+) / Windows 11 hardware scenarios were not run. Runtime AppCapability/permission revoke, WASAPI timestamp and cleanup diagnostics, default/selected hardware capture, RegisterHotKey, hidden-window overlap, FileOpenPicker handle transfer, signing, package identity, and Store eligibility remain unclaimed until those Windows gates execute.

All code remains an untrusted draft until the root reviewer reads every changed file and independently reruns the available checks.
