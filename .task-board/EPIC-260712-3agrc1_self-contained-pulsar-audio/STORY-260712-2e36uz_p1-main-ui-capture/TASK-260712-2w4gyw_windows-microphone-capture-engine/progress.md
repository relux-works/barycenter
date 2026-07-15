## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:40:50Z

## Last Update
2026-07-15T01:17:32Z

## Blocked By
- (none)

## Blocks
- TASK-260712-25at8b
- TASK-260712-c7dmv8
- TASK-260712-1p8ykc
- TASK-260712-ezdhpf

## Checklist
- [x] Implement the selected AppContainer-safe capture bridge behind a platform service boundary
- [x] Cover explicit permission, default and selected devices, metering and hard limits
- [x] Prove cleanup for cancel, device loss, revoke, lock, suspend and quit without sample export
- [x] Finalize exactly one durable draft and prove cue audio is excluded

## Notes
2026-07-15 strict sequential kickoff from synchronized main merge c52012b84d8c80a0ff8ccbbe445a778f381e65b3 after PR #53. Implementing the Windows capture service inline by promoting the signed-probe AppCapability/WASAPI bridge behind a bounded Go lifecycle and the accepted private-draft/cue contract. Real microphone, signed Windows 10/11, hidden-capture and physical device/lifecycle observations remain manual EPIC-260714-th54l3 evidence.
2026-07-15 accepted engineering head b40bd16c378e2843ebe16ed38cdb8e076de4de43 in PR #54. Added the production AppCapability/WASAPI backend over the selected pulsar-capture.dll ABI; explicit-Record permission, default/selected input, 48 kHz mono PCM16 private WAV, local RMS, cue exclusion, ducking, 180-second/50-MiB limits, typed lifecycle cleanup and exactly-one draft semantics. Local go test -race ./..., Windows amd64 build/vet and board validation passed. GitHub Actions run 29381000568 passed all four jobs on rerun, including native C++ tests, signed MSIX build/install/cleanup and artifact upload; the first attempt was an unrelated existing overlay callback timing flake (96/100) and passed unchanged on rerun. Real microphone/permission UI/hidden capture/device and lifecycle observations remain EPIC-260714-th54l3.

## Precondition Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-2w4gyw/p1-main-ui-capture-components.puml) — Task seam and platform service context

## Outcome Resources
- [p1-windows-capture-engine-components.puml](file://TASK-260712-2w4gyw/p1-windows-capture-engine-components.puml) — Production AppCapability/WASAPI capture ownership, privacy and lifecycle boundaries
