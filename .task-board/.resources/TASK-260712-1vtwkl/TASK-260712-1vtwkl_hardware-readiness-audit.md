# TASK-260712-1vtwkl hardware readiness and frozen evidence matrix

Date: 2026-07-14 (Asia/Tbilisi)

Status: BLOCKED ON EXTERNAL PHYSICAL HOSTS. This document freezes the run
contract; it is not Windows 10/11 evidence and must not be used to mark any
matrix row as passed.

## Accepted input artifact

- Source commit: `f5b73f06a9e06f71c6193d982e6138e5bec68247`.
- GitHub Actions run: `29292631211`, completed successfully.
- Artifact: `pulsar-signed-msix-probe`, artifact ID `8295657025`.
- GitHub artifact archive digest:
  `sha256:5d5d6714096ec1a917e34c6826569c1c4735e2e56b38fb9307d786d5e1f1e555`.
- Package: `PulsarProbe-0.1.0.0-x64-signed.msix`.
- Package SHA-256:
  `a0c3022b69c68f140969a7d7bef4cd0904f1b2872960e7a4511bea9462749be7`.
- Package identity: `ReluxWorksLLC.PulsarBarycenter`, Publisher
  `CN=60105954-A0D9-4E89-B32D-18AF2F423ABE`, x64, version `0.1.0.0`.
- PFN: `ReluxWorksLLC.PulsarBarycenter_q036g2bzd7ngc`.
- AUMID: `ReluxWorksLLC.PulsarBarycenter_q036g2bzd7ngc!PulsarProbe`.
- Runtime contract: `appContainer` / `packagedClassicApp`; exactly the three
  existing network capabilities plus `microphone`; no extension, full-trust,
  broad-filesystem, or developer-mode dependency.
- Local cache was re-hashed on 2026-07-14 and matched the package digest. The
  GitHub artifact is scheduled to expire on 2026-07-27. If it is unavailable,
  regenerate a package from the frozen source and run both OS rows with the
  same newly recorded bytes; never combine evidence from two package hashes.

The self-signed certificate route is admissible for this controlled hardware
matrix because the package is signed, the installer validates the frozen
product identity before adding only the embedded public signer to Trusted
People, and developer mode is not used. It is not Store certification.

## Evidence-kit regression checkpoint

Commit `12d563897507406af92705502c8a1ee56b490ad4` adds the operator-side
collector, cleanup verifier and negative contract suite. GitHub Actions run
`29295222623` passed all four jobs on 2026-07-14. The Windows packaged-probe job
proved only the tooling boundary: strict H00-H17 ordering, privacy rejection,
serviced-build/physical-host preflight, evidence hash-tamper rejection, real
`RegisterHotKey` conflict/release, signed install receipt and exact cleanup.
It is hosted execution and does not pass any H-row.

The inspected regression artifact is `pulsar-signed-msix-probe`, artifact ID
`8296494980`, archive digest
`sha256:9c8897a7bcb3c48c27220a1adcca82dc1410263dd570746ff081763845185121`,
expiring 2026-07-28. It contained exactly the MSIX, digest/build metadata,
schema-v2 install receipt and cleanup receipt; no certificate or private-key
export. The generated package SHA-256 was
`1b9a6e0d3b76578638956791b3bec7cff77cde4682d0e49f20a9558a9d86c344`.
Install and cleanup receipts agreed on that digest and the frozen PFN/AUMID;
cleanup recorded process/package/run-added trust/runtime absence, zero partials,
hotkey reacquisition, and exclusive picker open/rename/delete.

This newer artifact is not silently substituted for the accepted input above.
At physical-run start, freeze one still-available package and use its exact
bytes for both OS rows. If the accepted artifact has expired, record the newly
selected source/run/artifact/hash before H00 and never combine histories from
different package hashes.

## Access audit

The active executor has no admissible physical Windows path:

- the current machine is x86_64 macOS and has no Boot Camp or Windows volume;
- no Windows peer is present in the accessible Tailscale network;
- no Windows SSH target is configured;
- repository workflows expose only GitHub-hosted Windows execution, which is
  explicitly outside the real-hardware/audio boundary;
- repository and organization self-hosted-runner inventory is not authorized
  to the current GitHub credential, and no usable runner endpoint was supplied;
- no physical-console operator or removable microphone was supplied.

No VM, cloud desktop, GitHub-hosted runner, unpackaged executable, developer
mode, synthetic device, or fabricated screenshot may satisfy a row.

## Admissible machines

Use two distinct physical x64 machines or two physical boot installations. A
machine must have a dedicated local test account, administrative authority for
the controlled signer-trust/install step, an audible output, a default physical
microphone, and a second distinct removable/selectable input such as a USB or
wired microphone. The operator must be able to use the physical console,
sleep/wake, lock/unlock, sign out, change microphone privacy, disconnect the
selected device, and capture screenshots.

### Windows 10 row

At the current date, ordinary Windows 10 22H2 Home/Pro/Enterprise/Education
reached end of support on 2025-10-14. ESU supplies security updates but does
not extend the product lifecycle or technical support. Therefore an ordinary
22H2 machine is not silently accepted as the task's "supported Windows 10"
host.

The strict admissible row is physical Windows 10 Enterprise LTSC 2021, build
19044, fully patched and licensed; Microsoft lists mainstream support through
2027-01-12. A 22H2+ESU row requires a recorded product decision changing the
meaning of "supported" before execution.

Official lifecycle references:

- https://learn.microsoft.com/en-us/lifecycle/announcements/windows-10-22h2-end-of-support-update
- https://learn.microsoft.com/en-us/lifecycle/products/windows-10-enterprise-ltsc-2021
- https://learn.microsoft.com/en-us/lifecycle/faq/extended-security-updates

### Windows 11 row

Use a physical, Windows-11-compatible machine on a currently serviced x64
release with the latest cumulative update. Windows 11 24H2 remains serviced on
2026-07-14 (Home/Pro through 2026-10-13 and Enterprise/Education through
2027-10-12); 25H2 or a hardware-shipped 26H1 release is also acceptable when
currently serviced. Record edition, display version, full build and UBR.

Official lifecycle references:

- https://learn.microsoft.com/en-us/windows/release-health/windows11-release-information
- https://learn.microsoft.com/en-us/lifecycle/products/windows-11-enterprise-and-education
- https://learn.microsoft.com/en-gb/lifecycle/products/windows-11-home-and-pro

## Per-host preflight record

The evidence bundle must record, without hostname, username, serial number or
absolute user path:

1. physical-machine attestation and console operator;
2. edition, DisplayVersion, architecture, OS build plus UBR, install type and
   last cumulative update;
3. lifecycle eligibility for that exact edition/version;
4. Developer Mode off;
5. physical output plus default and second selectable input: friendly name,
   driver provider/version/date and probe-emitted MMDevice ID;
6. exact MSIX SHA-256, embedded signing subject/thumbprint/validity, signature
   status, identity, Publisher, version, architecture, PFN, AUMID, trust level,
   runtime behavior and exact capability set;
7. absence of an existing package in the real Pulsar product family before
   installation;
8. install receipt and the fact that only the embedded public self-signed
   certificate was temporarily trusted;
9. system clock/timezone and UTC start/end timestamps;
10. Windows App Certification Kit version.

## Frozen scenario order

Execute every scenario from top to bottom on Windows 10, then repeat the same
order on Windows 11 using the same MSIX bytes. Keep one complete, unspliced
`scenarios.jsonl` history per host and mark every row PASS, FAIL or BLOCKED.
Every non-PASS row must include the exact HRESULT/GetLastError, observed order,
sanitized screenshot/log slice and a concrete next action.

| ID | Scenario | Required pass evidence |
| --- | --- | --- |
| H00 | Clean signed install | Frozen manifest/signature/identity match; install and exact AUMID launch succeed without developer mode or replacing another real-family package. |
| H01 | Cold permission deny | No prompt at launch; explicit Record causes the system microphone prompt; denial is typed and produces no promotable WAV while picker and non-capture UI remain usable. Record prompt latency. |
| H02 | Cold permission allow | From a genuinely reset/clean permission state, explicit Record causes the system prompt; Allow leads to an allowed status and capture preparation. Record prompt and activation latency. |
| H03 | Default capture | Default physical input is the logged MMDevice; ten-second capture has supported native/mix format, frames after activation, exactly one valid WAV, audible content and independent decoder metadata. |
| H04 | Selected capture | A distinct second physical input is selected and logged; capture comes from that device, finalizes one valid WAV and remains distinguishable from default input. |
| H05 | Hotkey and conflict | `Ctrl+Shift+R` starts/stops from the hidden lifecycle window with no repeat double-toggle. A separate process first owning the chord produces an honest blocked result/GetLastError; after release and clean relaunch, registration and toggle pass. |
| H06 | Brokered picker | Visible-owner picker reads and hashes fixture bytes through the brokered handle without broad filesystem access. Repeat after hiding then restoring the owner and record focus/modality. After release/exit the fixture can be exclusively renamed and deleted. |
| H07 | Hidden-window recording | Start visibly, hide the main window, prove post-hide frames while still hidden, then stop from tray/hotkey. Exactly one valid WAV is finalized and capture does not depend on a visible main window. |
| H08 | Repeated cycles | Ten alternating default/selected start-stop cycles complete without stale generation, leaked process/device/handle, duplicate artifact, stuck hotkey or inaccessible picker fixture. |
| H09 | Quit during capture | Tray Quit during active capture records the ordered graceful cleanup through synced `lifecycle_process_exit_ready`; process exits inside the bound, hotkey/WTS/tray/helper ownership is released and no orphan partial remains. |
| H10 | Suspend/resume | During active capture, real system sleep emits `PBT_APMSUSPEND`, stops and disposes the exact generation, unregisters hotkey, and never auto-restarts. Resume is observed, rearm succeeds, and a new explicit capture works. |
| H11 | Session lock/unlock | `WTSRegisterSessionNotification` succeeds. Physical-console lock emits `WTS_SESSION_LOCK`, stops/disposes exact capture and unregisters hotkey; unlock is observed, does not auto-restart, and permits a new explicit capture. |
| H12 | Device removal | Unplug the selected removable input during active capture. A typed device-loss/WASAPI terminal result appears, non-promotable partial state is cleaned, no old generation resumes, reconnect/discovery and a new capture work. |
| H13 | Permission revoke/restore | Disable microphone access in Settings during active capture. `AccessChanged`, deterministic WASAPI failure, or both must arrive in bounded time with exact status/HRESULT/order. Audio is discarded fail-closed, hotkey remains unavailable, restore does not auto-restart, and a new explicit capture works. No signal is BLOCKED/no-go. |
| H14 | Abrupt kill and recovery | Force-kill during capture, retain the safe partial, relaunch, and prove startup recovery creates a playable independently decoded WAV or discards a too-short/corrupt partial according to policy. No unreconciled sidecar/partial remains. |
| H15 | Sign-out/shutdown boundary | During active capture observe actual `WM_QUERYENDSESSION` and confirmed `WM_ENDSESSION`; do not claim post-latch cleanup. On next launch, startup recovery reconciles the OS-owned boundary and leaves no unreconciled artifact. |
| H16 | WACK | Run the current Windows App Certification Kit on the exact installed/package bytes. Save tool version and complete report; any failure or API warning is FAIL with exact next action. |
| H17 | Final cleanup | Copy evidence first; then stop process, remove package and only trust added by this run. Process/package/trust are absent, privacy capture indicator is off, the hotkey can be acquired by the conflict helper, picker fixture can be exclusively renamed/deleted, and no inaccessible or unreconciled partial remains. |

## Bundle contract

For each OS, preserve:

- `machine.json` with only the preflight fields above;
- package build metadata, install receipt and SHA-256 verification output;
- the complete sanitized `scenarios.jsonl` and the whole package-private
  `evidence` directory copied only after the probe exits;
- independent WAV decoder output and SHA-256 for every retained WAV;
- UTC-indexed screenshots for prompts, controls, Settings transitions,
  device selection, WACK and every failure/blocked state;
- `matrix.md` mapping H00-H17 to exact log line/time, screenshot, artifact,
  verdict and next action;
- `cleanup.json` proving process/package/trust/hotkey/picker cleanup;
- WACK report and tool version.

Do not include microphone payload beyond the approved short fixture recordings,
certificate exports, private keys, passwords, tokens, usernames, hostnames,
serial numbers or absolute user paths. Before attachment, scan the bundle for
those values and record the scan result.

## Exact unblock request

Provide both of the following, plus a physical-console operator window:

1. one physical x64 Windows 10 Enterprise LTSC 2021 machine, patched and
   licensed, or an explicit product decision authorizing a different Windows
   10 lifecycle posture;
2. one physical Windows-11-compatible x64 machine on a currently serviced
   release;
3. on each, a dedicated admin-capable local test account, audible output,
   default mic, second removable/selectable mic, WACK, screenshot capture and
   permission to install/remove the signed test MSIX and its embedded public
   certificate;
4. a safe transfer route for the exact package and sanitized evidence bundle.

Until all four are available, `TASK-260712-1vtwkl` remains blocked, accepted
progress stays 11/205, and no later task may start.
