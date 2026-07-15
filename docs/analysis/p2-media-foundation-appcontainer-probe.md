# P2 Media Foundation AppContainer probe

Date: 2026-07-15

Task: `TASK-260712-298tyq`

Candidate: `native-canonical-aac-v1`

## Decision boundary

The candidate is fail-closed until the signed package run exists. The inbox
Media Foundation path is expected to decode the frozen MP3 and AAC fixtures and
to reject both real Ogg/Opus fixtures with recorded HRESULTs. Microsoft lists
MP3 and AAC support for Windows desktop apps, but does not list Ogg/Opus in the
desktop audio container matrix. The probe therefore tests the actual files and
never infers support from the presence of an Opus transform alone.

Even if every hosted check succeeds, this task does not make a production or
real-hardware claim. The required two-hour soak and supported Windows 10,
Windows 11 x64 and Windows 11 ARM64 matrix remains in the dedicated manual-test
epic `EPIC-260714-th54l3`. Missing manual evidence is rejection. The native
binary accepts `--soak-seconds=7200`, while hosted CI runs a 60-second accelerated
open/decode/dispose loop and records its exact duration and RSS values.

## Implemented adapter

The signed package contains one statically linked MSVC executable and the six
frozen smoke fixtures. The executable is linked with `/APPCONTAINER`; Microsoft
documents that such a PE can run only in an AppContainer. The MSIX manifest also
declares `uap10:TrustLevel="appContainer"` with `packagedClassicApp`. It has no
capabilities element, no `runFullTrust`, no development-mode registration and no
runtime executable download.

The adapter exposes a seekable, read-only `IStream` over the already-authorized
app-private prepared input. Each underlying read is capped at 1 MiB. Media
Foundation wraps that stream with `MFCreateMFByteStreamOnStreamEx`, then creates
an `IMFSourceReader` with `MFCreateSourceReaderFromByteStream`. Microsoft marks
the latter as available to UWP apps and requires `CoInitializeEx` plus
`MFStartup`, which the probe performs on its single MTA decode worker.

The decode worker owns prepare, Source Reader calls, cancellation and decoder
errors. It schedules from a monotonic target, holds pause without reading,
seeks through `IMFSourceReader::SetCurrentPosition` with a new generation,
resumes and drains to `MF_SOURCE_READERF_ENDOFSTREAM`. A separate cooperative
cancel pass stops between samples. There is deliberately no WASAPI or render
callback in this candidate probe, so network, disk, allocation and decoder work
cannot accidentally migrate into an audio callback.

## Reproducible package proof

The dedicated workflow builds x64 and ARM64 with the installed Visual Studio
toolchain, verifies the PE AppContainer flag, inventories imports, signs each
executable and both MSIX packages with an ephemeral non-exportable CI
certificate, and validates that the manifests contain no capabilities. Only
x64 is runnable on the hosted runner; ARM64 remains a signed cross-build/schema
proof until real ARM64 hardware is tested.

The x64 package is installed through `Add-AppxPackage` after temporary
`LocalMachine\\TrustedPeople` trust. The harness runs the installed-path PE and
accepts it only when the process self-reports the installed package family plus
`TokenIsAppContainer=true`; package registration and the PE flag keep this path
inside the sandbox. It then independently activates the package with
`IApplicationActivationManager::ActivateApplication`, `AO_NONE`, and never
calls package debug APIs. The process records its package identity,
`TokenIsAppContainer`, COM apartment/thread, fixture outcomes, lifecycle
timings, range-read ceiling and RSS into its own `LocalState`. The harness reads
the receipt, uninstalls the package and removes temporary trust.

## Acceptance or rejection

The hosted receipt is accepted only when all four MP3/AAC fixtures decode and
drain, both Ogg/Opus fixtures reject with exact non-success HRESULTs, lifecycle
checks pass, the token is AppContainer, each read is at most 1 MiB, peak RSS is
at most 200 MiB, scheduled skew is at most 100 ms, and seek-to-sample is at most
3 seconds. Any changed format behavior is retained as evidence and forces review
rather than silently widening the candidate.

Because the shared codec rubric requires MP3, AAC and Opus, a normal inbox
Ogg/Opus rejection means this candidate is not a universal source decoder.
Canonical server-side AAC/M4A conversion remains mandatory for unsupported
uploads if this native path is selected. Final candidate comparison and
selection belongs to `TASK-260712-ibuaxj` and the subsequent ADR task.

## Primary references

- [Microsoft: supported codecs for Windows apps](https://learn.microsoft.com/windows/apps/develop/media-authoring-processing/supported-codecs)
- [Microsoft: MFCreateSourceReaderFromByteStream](https://learn.microsoft.com/windows/win32/api/mfreadwrite/nf-mfreadwrite-mfcreatesourcereaderfrombytestream)
- [Microsoft: IMFByteStream](https://learn.microsoft.com/windows/win32/api/mfobjects/nn-mfobjects-imfbytestream)
- [Microsoft: `/APPCONTAINER`](https://learn.microsoft.com/cpp/build/reference/appcontainer)
- [Microsoft: app capability declarations](https://learn.microsoft.com/windows/apps/package-and-deploy/app-capability-declarations)
- [Microsoft: IApplicationActivationManager::ActivateApplication](https://learn.microsoft.com/windows/win32/api/shobjidl_core/nf-shobjidl_core-iapplicationactivationmanager-activateapplication)
