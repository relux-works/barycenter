# Packaged Windows probe

This Windows-only probe packages `pulsar-win-probe-amd64.exe` and the actual
`pulsar-capture.dll` helper in an x64 `packagedClassicApp` + `appContainer`
MSIX. The only added device capability is `microphone`; file access is through
`FileOpenPicker` and a take-once brokered read handle.

The probe is frozen to the current Partner Center product identity, so the
same package can take either of two explicit signing routes:

| Field | Frozen value |
| --- | --- |
| Partner Center product | `9P26FDCWV1GC` (`Pulsar Barycenter`) |
| Package identity | `ReluxWorksLLC.PulsarBarycenter` |
| Publisher | `CN=60105954-A0D9-4E89-B32D-18AF2F423ABE` |
| Package family | `ReluxWorksLLC.PulsarBarycenter_q036g2bzd7ngc` |
| Architecture | `x64` |
| Application ID | `PulsarProbe` |
| AUMID | `ReluxWorksLLC.PulsarBarycenter_q036g2bzd7ngc!PulsarProbe` |
| Trust/runtime | `appContainer` / `packagedClassicApp` |
| Capabilities | `internetClient`, `internetClientServer`, `privateNetworkClientServer`, `microphone` |

Because package family is derived from the identity and Publisher, this probe
belongs to the real Pulsar product family. `build-probe.ps1` derives the exact
family with `PackageFamilyNameFromId`; `install-probe.ps1` verifies and prints
the family and full AUMID instead of asking an operator to guess a publisher
hash. Installation deliberately refuses to replace an existing package in
that family. Use a dedicated Windows test account or host with Store Pulsar
absent.

## Obtain a signed package

Every successful `pulsar-win-packaged-probe` CI job creates the
`pulsar-signed-msix-probe` artifact. It contains:

- `PulsarProbe-0.1.0.0-x64-signed.msix` — signed package, with no exported
  private key;
- `.msix.sha256` and `.msix.json` — digest and frozen build contract;
- `.msix.install.json` — hosted-Windows install receipt with the resolved
  package digest, family, AUMID, public signer provenance, trust ownership and
  relative evidence locations;
- `.msix.cleanup.json` — hosted-Windows proof that the exact package,
  run-added trust, runtime root, process, picker handle and hotkey ownership
  were released. Its boundary is cleanup-contract-only, not hardware evidence.

For example, with GitHub CLI and a completed CI run ID:

```powershell
gh run download <run-id> `
  --name pulsar-signed-msix-probe `
  --dir .\dist\windows-probe
```

To reproduce the signed package locally, use a Visual Studio 2022 developer
shell with the x64 MSVC toolchain and Windows SDK 10.0.19041 or later:

```powershell
$thumbprint = & .\pulsar-win\probe-msix\new-test-signing-certificate.ps1
try {
  .\pulsar-win\probe-msix\build-probe.ps1 `
    -Version 0.1.0.0 `
    -SigningCertificateThumbprint $thumbprint
} finally {
  Remove-Item -Force "Cert:\CurrentUser\My\$thumbprint" `
    -ErrorAction SilentlyContinue
}
```

The generator creates a short-lived, non-exportable test key in the current
user certificate store. Its Subject is read from and must exactly match the
manifest Publisher. The build runs native CMake/CTest, Go vet/tests/build,
MakeAppx, SignTool with SHA-256, embedded-signer verification, and writes the
package to `dist\windows-probe`. It never writes a PFX, certificate password,
or private key to the repository or artifact.

The self-signed route is for controlled hardware testing only. Microsoft
requires its public signer to be explicitly trusted on each test host. The
Store route below is the production distribution path and does not share that
trust step.

## Install, launch, and collect artifacts

Open an elevated PowerShell terminal on a dedicated Windows 10 or Windows 11
test host, then run:

```powershell
$package = Resolve-Path `
  .\dist\windows-probe\PulsarProbe-0.1.0.0-x64-signed.msix
$install = & .\pulsar-win\probe-msix\install-probe.ps1 `
  -Package $package `
  -TrustLocalTestSigner `
  -Launch
```

Before changing trust, the installer reads `AppxManifest.xml` directly from the
MSIX and rejects any identity, target-family, application, extension,
trust/runtime, executable, or capability declaration outside the frozen
contract. `-TrustLocalTestSigner` then accepts only a self-signed Code Signing
certificate whose Subject exactly equals the frozen Publisher. It extracts
only the public certificate embedded in the MSIX and adds it to Local Computer
→ Trusted People, then revalidates the package signature before
`Add-AppxPackage`.
Omit this flag for a Store-signed package.

The script validates identity, Publisher, x64 architecture, package family,
AppContainer/runtime attributes, and the exact four-capability set. It then
launches `shell:AppsFolder\<package-family>!PulsarProbe`, verifies that the
probe process appeared, and prints the resolved paths. The same values are in
the adjacent schema-v2 `.msix.install.json` receipt, including the exact
package full name, public signer identity/validity/thumbprint, whether this run
added signer trust, and the relative runtime evidence root. It contains no
private signing material or absolute user path.

The visible window provides separate `Record default` and `Record selected`
actions, `Stop`, brokered picker, and hide controls. The tray duplicates those
controls. `Ctrl+Shift+R` toggles the currently selected capture mode. Run only
the scenario sequence assigned by the Windows 10/11 evidence matrix; do not
merge evidence from two hosts.

Runtime output is package-private and resolves exactly as follows:

```text
%LOCALAPPDATA%\Packages\<package-family>\LocalState\PulsarProbe\
  scenarios.jsonl
  evidence\
    *.wav
    *.partial
    *.partial.reason
```

Do not copy or splice the runtime directory by hand for the hardware matrix.
Use the evidence kit below after the probe has exited. Never attach certificate
exports, local usernames/paths, microphone content outside the task-approved
short fixtures, or a private key to task results.

## Physical Windows 10/11 evidence kit

`hardware-evidence.ps1` creates one fail-closed bundle per physical host. It
does not have a hosted-runner/VM override and never promotes a scenario from a
test command. It records each H00-H17 operator verdict as `unreviewed`; task
acceptance still requires inspection of the referenced bytes.

Before installing the MSIX, list the exact endpoint friendly names and create a
new bundle. The strict Windows 10 row is Enterprise LTSC 2021 build 19044;
`ApprovedException` requires an explicit product-decision reference.

```powershell
Get-PnpDevice -Class AudioEndpoint -PresentOnly |
  Select-Object Status, FriendlyName, InstanceId

$package = Resolve-Path `
  .\dist\windows-probe\PulsarProbe-0.1.0.0-x64-signed.msix
$bundle = Join-Path $PWD "win10-ltsc-physical-a"
$staging = Join-Path $env:TEMP "pulsar-evidence-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $staging | Out-Null
$pickerFixture = Join-Path $staging "picker-fixture.bin"
[IO.File]::WriteAllText(
  $pickerFixture,
  "pulsar-picker-fixture-v1",
  [Text.UTF8Encoding]::new($false)
)

.\pulsar-win\probe-msix\hardware-evidence.ps1 `
  -Mode Initialize `
  -OutputDirectory $bundle `
  -RunID win10-ltsc-physical-a `
  -OSFamily windows10 `
  -Package $package `
  -PhysicalMachineAttested `
  -ConsoleOperatorAttested `
  -OutputEndpointName "<exact physical output friendly name>" `
  -DefaultInputName "<exact default physical input friendly name>" `
  -SelectedInputName "<exact second removable input friendly name>"

$installReceipt = Join-Path $staging "install.json"
$install = .\pulsar-win\probe-msix\install-probe.ps1 `
  -Package $package `
  -TrustLocalTestSigner `
  -ReceiptPath $installReceipt `
  -Launch
```

Use the frozen H00-H17 order in
`TASK-260712-1vtwkl_hardware-readiness-audit.md`. Exit the probe before every
stable runtime snapshot. Attach screenshots, independently decoded WAV
metadata, sanitized WACK output, prompt timing, and other evidence separately;
then record an honest terminal operator verdict. A `FAIL` or `BLOCKED` verdict
requires a concrete next action. Evidence and verdicts cannot be overwritten.

```powershell
.\pulsar-win\probe-msix\hardware-evidence.ps1 `
  -Mode Snapshot -OutputDirectory $bundle -Scenario H03

$decoderReport = Join-Path $staging "H03-independent-decoder.json"
.\pulsar-win\probe-msix\hardware-evidence.ps1 `
  -Mode Attach -OutputDirectory $bundle -Scenario H03 `
  -Attachment $decoderReport

.\pulsar-win\probe-msix\hardware-evidence.ps1 `
  -Mode Verdict -OutputDirectory $bundle -Scenario H03 -Verdict PASS `
  -Observation "default physical input, valid ten-second WAV, independent decoder pass"
```

For H05, run the holder in a separate PowerShell process before launching the
probe. Its ready file contains no host data. The signed probe must report the
real conflict/GetLastError; after stopping the holder and relaunching, the
probe must acquire and exercise the chord.

```powershell
$hotkeyReady = Join-Path $staging "hotkey-holder-ready.json"
$holder = Start-Process pwsh -PassThru -ArgumentList @(
  "-NoProfile", "-File",
  (Resolve-Path .\pulsar-win\probe-msix\hotkey-conflict.ps1),
  "-Mode", "Hold", "-HoldSeconds", "120", "-ReadyPath", $hotkeyReady
)
while (-not (Test-Path $hotkeyReady) -and -not $holder.HasExited) {
  Start-Sleep -Milliseconds 100
}
# Launch the signed probe, capture the blocked registration, and exit it.
Stop-Process -Id $holder.Id -Force
.\pulsar-win\probe-msix\hotkey-conflict.ps1 -Mode Probe
```

After H00-H16 evidence is copied, clean the exact package, runtime root and only
the signer trust added by this run. The cleanup script refuses to run without
`-EvidenceCopied`, validates the receipt before removal, proves exclusive
picker-fixture access by an open+rename+delete round trip, and reacquires
`Ctrl+Shift+R`. Attach that receipt as H17, record the H17 verdict, and seal only
after every row has an evidence reference and terminal verdict.

```powershell
$cleanupReceipt = Join-Path $staging "cleanup.json"
.\pulsar-win\probe-msix\uninstall-probe.ps1 `
  -ReceiptPath $installReceipt `
  -PickerFixture $pickerFixture `
  -CleanupReceiptPath $cleanupReceipt `
  -EvidenceCopied

.\pulsar-win\probe-msix\hardware-evidence.ps1 `
  -Mode Attach -OutputDirectory $bundle -Scenario H17 `
  -Attachment $cleanupReceipt
.\pulsar-win\probe-msix\hardware-evidence.ps1 `
  -Mode Verdict -OutputDirectory $bundle -Scenario H17 -Verdict PASS `
  -Observation "process/package/run-added trust/runtime root absent; hotkey and picker released"
.\pulsar-win\probe-msix\hardware-evidence.ps1 `
  -Mode Seal -OutputDirectory $bundle
```

The kit enforces H00-H17 order and rechecks each referenced file and nested
runtime-snapshot hash before sealing. The seal is a hash index plus an honest
aggregate of unreviewed operator verdicts. `all-operator-pass-unreviewed` is
deliberately not a task pass. Run the Windows 10 row first and Windows 11 second
with the exact same package SHA-256; never merge two bundle histories or reuse
an output directory.

The following manual commands are emergency cleanup only. They do not produce
an admissible H17 receipt:

```powershell
Get-Process pulsar-win-probe-amd64 -ErrorAction SilentlyContinue |
  Stop-Process
Get-AppxPackage -Name ReluxWorksLLC.PulsarBarycenter |
  Remove-AppxPackage
if ($install.SignerTrustAdded) {
  Remove-Item -Force `
    "Cert:\LocalMachine\TrustedPeople\$($install.SignerThumbprint)"
}
```

## Store-distributed route

For a Store private flight, create the unsigned upload candidate by omitting
the thumbprint:

```powershell
.\pulsar-win\probe-msix\build-probe.ps1 `
  -Version <monotonic-four-part-version>
```

The resulting `*-unsigned.msix` keeps the product identity and Publisher shown
above. Submit it only through an explicitly authorized Partner Center flight
for product `9P26FDCWV1GC`; Microsoft signs the certified MSIX. This task does
not publish or modify a Partner Center submission. A Store-installed package
is launched with `install-probe.ps1 -Package <path> -Launch` and needs no local
certificate trust.

Microsoft's current contracts are documented in
[Create a certificate for package signing](https://learn.microsoft.com/windows/msix/package/create-certificate-package-signing),
[Sign an app package using SignTool](https://learn.microsoft.com/windows/msix/package/sign-app-package-using-signtool),
[MSIX signing end-to-end](https://learn.microsoft.com/windows/msix/package/sign-msix-package-guide),
and [package identity](https://learn.microsoft.com/windows/apps/desktop/modernize/package-identity-overview).

CI proves build, signature creation, trusted-signature validation, package
registration, and contract receipt creation on hosted Windows. It does not
prove WACK, Store certification, microphone behavior, lifecycle event delivery,
or real Windows 10/11 hardware behavior. Those remain the strict downstream
matrix gate.

## Lifecycle cleanup and evidence

The hidden top-level window is the lifecycle owner. Each observed edge receives
a monotonic `cleanupId`; `scenarios.jsonl` records `cleanupOrder`,
`cleanupStage`, `lifecycleEdge`, `lifecycleMode`, `stopReason`, and the exact
latest `observedOSSignal` plus the ordered repeated-signal history. Capture and
permission continuations carry a monotonic capture generation. Closing a
lifecycle gate and binding that exact generation is one synchronized
transition, so an older or later generation cannot satisfy its cleanup run.
Terminal, temporary-artifact, and native-release observations are retained as
independent monotonic facts for that generation. If a callback wins the race
before stop publication, the facts replay only after the run reaches the stop
stage; release is not treated as proof that artifact cleanup succeeded.
The selected signal paths are:

| Edge | Selected packaged-probe signal | Behavior |
| --- | --- | --- |
| Explicit quit | tray `Quit`, hidden-window `WM_CLOSE`, or `Ctrl-C`/`SIGTERM` | Asynchronous graceful stop and drain; permission callback, hotkey, WTS registration, helper, tray, and log are released in order before `WM_QUIT`. |
| Suspend | `WM_POWERBROADCAST/PBT_APMSUSPEND` | Nonblocking capture stop with `CAP_REASON_SUSPEND`; hotkey stays unregistered until a resume message is observed. Capture is not restarted automatically. |
| Session lock | `WTSRegisterSessionNotification` then `WM_WTSSESSION_CHANGE/WTS_SESSION_LOCK` | Nonblocking capture stop with `CAP_REASON_LOCK`; hotkey stays unregistered until `WTS_SESSION_UNLOCK`. |
| Permission revoke | `AppCapability.AccessChanged` plus `CheckAccess`; the existing bounded permission poll is a deterministic defensive signal | Nonblocking stop with `CAP_REASON_PERMISSION_REVOKE`; evidence is discarded fail-closed and the hotkey remains unavailable until access is allowed again. |
| Windows sign-out/shutdown | `WM_QUERYENDSESSION`, followed by `WM_ENDSESSION` | Requests stop without blocking the window procedure. A cancelled shutdown returns through ordinary idle cleanup; confirmed shutdown performs no post-latch cleanup/evidence and hands remaining ownership to Windows/startup recovery. |

For graceful quit with an active capture, the asserted order is signal, stop
request, capture settlement, temporary-artifact disposition, capture release,
permission callback unsubscribe, hotkey unregister, WTS unregister,
`CapDestroy`, tray deletion, evidence-log sync, and process-exit-ready. Tray
ownership is retained when `Shell_NotifyIconW(NIM_DELETE)` fails so the UI
thread can retry it. The exit-ready record is itself synced before
`PostQuitMessage`. A capture result-query failure is logged as
`terminalObserved=false`; it is not presented as a native terminal result.
Owned temporary paths are retried through the production
`ArtifactWriter.Abort` postcondition before later cleanup stages may advance.

Quit intent does not share the bounded ordinary command queue. The waiter
observes it on every bounded poll, so a full queue or failed wake cannot lose
terminal cleanup. Permission-ready, capture-ready, discovery, picker, and
rearm starts validate the current lifecycle gate; stale capture continuations
are logged no-ops and never call prepare or activate.

Idle cleanup and permission-rearm callbacks are durable waiter-owned intents.
The UI acknowledges an exact intent ID while `PostMessageW` is in flight or
after it succeeds; post completion observes synchronous consumption instead of
recreating the intent. A failed post is retried by the bounded waiter poll and
repeated failures escalate to graceful exit. Rearm itself remains a capture
start gate until the current
waiter permission result, discovery initiation, and UI hotkey registration have
been accepted as one transition. An `AccessChanged` query failure, or any
runtime permission-query failure while a capture generation is owned, closes
the permission gate and starts the same fail-closed stop path before diagnostic
logging. Failed explicit-record and lifecycle-rearm permission queries use that
same ordering: requested work is settled or stopped, the rearm token is closed,
and no permission-ready, discovery, hotkey, prepare, or activation continuation
is published.

Evidence sync after helper destruction uses a bounded retry so a storage fault
cannot leave a hidden hung process. The 30-second process deadline stays armed
through helper destruction and evidence sync until posting `WM_QUIT` is
irrevocably committed. The watchdog is logged when armed. Its hard-exit
callback, evidence-retry exhaustion, and the user-visible `Force Quit` path do
not put another potentially blocking log or filesystem sync in front of the
sole exit action. Missing process-exit-ready evidence therefore makes the run a
failure and startup recovery remains the next signed-run check; it is never a
passing clean-shutdown result.

JSONL writes and syncs run through one ordered, bounded evidence coordinator.
The first short write, error, queue saturation, or acknowledgement timeout is
sticky: the worker discards every queued successor without invoking the logger
or sync callback, and synchronous successors receive the same non-nil failure.
Later code cannot emit `evidence_log_synced` or a passing process-exit claim,
and bounded retry exhaustion takes the nonblocking hard-exit path. The evidence
worker also shares the confirmed-shutdown ordinary-work gate: an already
admitted callback may finish, but enqueue attempts and queued logger/sync
callbacks are suppressed without sanitization or physical I/O after the latch.
Before serialization, nested typed fields are cloned and scrubbed with bounded
cycle depth. Absolute Windows, UNC, POSIX (including root-level files), and
`file:///` paths trigger whole-value redaction, as do original picker names,
usernames, auth/credential/token/password values, and audio/payload content.
Evidence keeps generated session IDs, hashes, sizes, reasons, and result codes
instead of local artifact names. The top-level `DeviceID` has a narrower,
explicit trust boundary: it is populated only by the Windows default-device or
enumeration APIs, so recognized `\\?\SWD#MMDEVAPI#...` and ordinary MMDevice ID
forms remain available as exact hardware evidence. Credential text and actual
filesystem paths in that field are still rejected, and nested fields never use
the device-ID exception.

Suspend, lock, and permission-revoke paths return idle after capture/artifact
cleanup and hotkey unregistration. Resume or restored permission may re-register
the hotkey, but never restarts the previous capture. This makes repeated
start/stop and lifecycle cycles explicit rather than leaving a hidden active
session.

There are two real platform limits that cannot be resolved on a non-Windows
host or inferred from an unpackaged build:

- AppContainer delivery of power and WTS session notifications must be
  exercised with the signed MSIX on the Windows 10/11 hardware matrix. A failed
  WTS registration is logged as `blocked`, including `GetLastError` and the next
  action; without registration the probe cannot directly observe session lock.
- Once Windows confirms `WM_ENDSESSION`, Windows owns the remaining lifetime.
  The current native capture generation and operation are published together
  as one immutable atomic owner snapshot. The window procedure closes the
  abrupt start gate, claims that exact snapshot without entering the ordinary
  lifecycle mutex, requests its one-shot stop, publishes the monotonic
  confirmation latch, and wakes the waiter. It then returns without unregistering the hotkey,
  advancing lifecycle evidence, or releasing any resource. Confirmation wins
  over every coalesced ordinary wait event. The waiter may append at most eight
  already-buffered 4096-frame reads to an existing safe `.partial`, without
  logging, sync, finalization, cleanup, result-take, release, UI publication, or
  helper destruction, and then exits. While the stop call is between the closed
  gate and confirmation latch, the waiter suppresses ordinary drains but stays
  alive; it exits only after observing confirmation and claiming the bounded
  abrupt drain. An outer waiter-drain admission is not cleanup authority: each
  query/read, permission or helper call, artifact write/sync/finalize/abort,
  exact-owner release, UI post, lifecycle settlement, and evidence operation
  acquires its own atomic pre-close permit immediately before the callback. A
  callback holding that permit may return after confirmation, but its result
  cannot start a separately gated successor. In particular, an admitted Stop
  cannot authorize a late Finalize or Release, and an admitted Release may
  publish its one-shot result but cannot clear the owner or assert passing
  lifecycle/evidence state after the latch. Both wndprocs also guard their
  entry from gate closure: the
  still-running message pump returns protocol-required values directly and
  suppresses every queued app, timer, hotkey, command, close, cancel/resume, and
  repeated-shutdown callback during and after confirmation. A helper prepare or
  activation callback admitted before confirmation may finish, but it observes
  the abrupt gate on return. A successful prepare that returns only after the
  latch is neither published nor Stop-called; Windows/process teardown and
  startup recovery own that late registry operation. Its lifecycle-result
  commit also requires a fresh permit, so the pre-latch prepare-in-flight state
  is not advanced after confirmation. Every ordinary open-gate
  successful prepare owns an exact immutable generation/operation snapshot even
  if publication conflicts with the active owner. The unpublished loser is
  one-shot cancel-stopped at the helper-result seam only after the Stop callback
  acquires its own fresh pre-close permit. A same-generation duplicate readiness
  message is rejected by capture phase before a second helper call, so it
  creates no registry entry and cannot disturb the incumbent. If a real
  successful prepare nevertheless
  loses publication to a distinct/stale atomic incumbent, the lifecycle tracker
  keeps that loser native-owned. Its unique one-shot Stop callback is atomically
  claimed and stored before the exact orphan obligation becomes waiter-visible;
  only then is native Stop considered for invocation. A waiter at that
  publication-to-invocation seam therefore observes a live pending producer,
  never a structural gap. If confirmed shutdown closes the gate at that seam,
  both the invocation and lifecycle-result permits are rejected and
  OS/startup recovery keeps ownership. The
  loser is never activated and owns no
  artifact or ordinary UI/evidence successor. On the ordinary open-gate path,
  only the waiter may query its exact ID to terminal and invoke `CaptureRelease`
  through the loser's own release gate. A failed release retains the obligation
  for exact retry; success removes only that obligation and settles only its
  rejected generation, never the distinct active owner. New recording,
  `CLEANUP_READY`, and `CapDestroy` remain blocked while either an active or
  orphan owner exists. Confirmed shutdown never waits for an orphan Stop/query
  and admits zero orphan query/release work; Windows reclaims that boundary.
  Post-helper result evidence has a separate
  admission from successful-owner successors: an ordinary open-gate
  `CapturePrepare` failure records its HRESULT once without creating or
  activating an owner, while abrupt gate closure suppresses that pending row.
  A duplicate attempt preserves the incumbent generation/operation and invokes
  no second helper operation or result row. A queued duplicate refused before the helper
  after suspend/lock has already bound native A is diagnostic-only: it cannot
  fabricate terminal, artifact, release, or idle-cleanup progress for A. A
  genuinely pre-native invalidated generation retains suppressed settlement.
  For a published success, exact app-side operation/generation state is made
  visible before the potentially blocking result-evidence write, so an immediate
  auto-reset readiness signal cannot be consumed while the waiter still sees
  operation zero. Evidence failure then claims the exact owner's one-shot cancel
  stop before quit escalation; abrupt stop cannot claim it again, and graceful
  terminal cleanup reuses the recorded HRESULT instead of calling native stop a
  second time. Queued readiness must claim activation intent on that exact owner
  before activation-intent evidence. Stop, intent admission, and native
  admission are atomic: a stop-first owner emits no activation evidence and
  performs no native activation; a stop that arrives during intent evidence
  prevents native admission; and on an ordinary open-gate path an
  already-admitted native callback is followed by the same one-shot exact-owner
  stop. Capture drain, hotkey, lifecycle, quit,
  and shutdown stop paths reuse that claim instead of issuing a second native
  stop. Reuse exposes explicit pending versus completed state: an in-flight stop
  is never logged as `S_OK`, and query-failure cleanup cannot abort an artifact,
  release native ownership, clear the exact owner, or finalize lifecycle release
  until the recorded stop result is visible or independent native-terminal
  evidence authorizes the normal terminal path. Confirmed shutdown never waits
  for the in-flight result and admits no later ordinary cleanup retry. Native
  activation admission owns the helper-call interval: a later query, hotkey, or
  lifecycle stop is retained as pending and, while the ordinary gate remains
  open, invoked once only after the admitted `CaptureActivate` callback returns.
  Confirmation may claim the same exact Stop before its latch, but if activation
  returns only after the latch the distinct invocation is suppressed and handed
  to Windows/process teardown. This prevents both stop-before-activate and
  release-before-stop while keeping confirmed shutdown nonblocking and starting
  no post-latch helper callback. The completion/abandon defer is armed
  immediately after successful native admission, before the second shutdown
  check, so close-after-admission
  cannot strand a deferred stop on an ordinary path even when the external
  activation call is correctly suppressed; confirmed shutdown deliberately
  abandons it to OS handoff. Every later stop request carries both generation and
  operation identity. If authoritative terminal handling already released and
  cleared that exact owner, the late request is a not-requested no-op—there is
  no direct native fallback, and a reused operation ID in another generation is
  never stopped. Stop admission, native Stop result publication, terminal
  observation, and the actual waiter-owned `CaptureRelease` call now share that
  immutable `(generation, operation ID, owner)` state. While ordinary cleanup is
  admitted, a pending result always has either the immediate Stop callback or the
  admitted activation/deferred Stop callback as its live producer. Confirmed
  handoff performs no Release and intentionally needs no later producer. Release
  cannot overtake either ordinary producer;
  the release-admitted bit covers the helper call itself, and only exact `S_OK`
  marks the owner released and permits the same pointer to be cleared. A failed
  or unexpected success HRESULT retains the owner for retry and bounded
  fail-closed exit. Query-failure cleanup requires exact `S_OK` from Stop before
  artifact abort or release; a failed Stop is structural evidence, not terminal
  evidence. Independently observed native terminal state may authorize its
  normal terminal path only after any already-admitted Stop result is visible.
  Because Stop is nonblocking, an immediate query-failure `CaptureRelease` may
  honestly return `E_ILLEGAL_METHOD_CALL` until native terminal cleanup catches
  up. That result is recorded as waiting, not success or structural terminal
  proof; finalized retry derives authority only from the exact completed
  `S_OK` Stop (or separately observed terminal), retries the same owner/ID, and
  clears/settles once only after exact `S_OK` Release.
  `CapturePrepare` returning `S_OK` with operation ID zero is classified before
  publication as an ABI failure: no native API receives zero, the generation is
  settled, required redacted evidence is acknowledged first, and only then is
  the already-bounded graceful-exit path armed. Every task-owned structural
  ownership/ABI path uses that same explicit evidence-before-escalation API;
  evidence failure suppresses the successor and leaves sticky evidence-failure
  cleanup authoritative, while confirmed shutdown suppresses both callbacks.
  Required structural evidence and graceful escalation therefore have separate
  permits: an evidence callback admitted before confirmation may return, but it
  cannot arm escalation afterward. The same rule applies after an orphan
  Release: orphan removal, passing evidence, lifecycle settlement, and UI wake
  each require a fresh permit and remain suppressed when Release returns after
  the latch.
  A first result query that rejects a published
  nonzero operation as an invalid handle similarly retains ownership and enters
  bounded fail-closed exit without fabricating terminal or release evidence.
  Non-window SIGTERM/evidence-failure entry, the graceful watchdog callback,
  post-message-pump error logging/UI, and deferred local cleanup use the same
  gate, so they also add no post-latch work.
  Windows still cannot guarantee a terminal callback or durable file/log sync
  before process termination, so the hardware run must inspect fail-closed
  partial recovery on the next launch.

Portable Go tests drive the production generation ledger, exact atomic capture
owner, prepare/activation continuation, durable UI-intent,
bounded evidence, lifecycle, permission-query, rearm, resource-ownership,
timer-fallback, and process-exit coordinators. They validate the message
decision table, deterministic pre/post-publication race schedules, injected
post/write/sync/stall failures, cleanup ordering, privacy redaction,
idempotency, stop-to-confirmed waiter/message barriers, and 100 repeated cycles.
They are host-verifiable evidence, not a
substitute for the signed-MSIX lifecycle and hardware gate.
