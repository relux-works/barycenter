# TASK-260712-1vtwkl current-build handoff

Date: 2026-07-20 (Asia/Tbilisi)

Status: IN PROGRESS — H01 PHYSICAL FAILURE REQUIRES A NEW BUILD. Ivan
Oparin confirmed `mbpro-win` as the physical Windows 10 host and authorized
this row first with maximum autonomous execution. SSH, sanitized preflight,
exact package transfer and WACK preparation are complete. Ivan Oparin then
limited this pass to the built-in microphone. Windows 11 is intentionally
deferred for this pass. The historical bundle A contains one terminal H00
`FAIL`; bundle B contains H00 `PASS` and H01 `FAIL`. The strict execution
frontier is the permission-path repair and a newly signed immutable bundle,
not H02 on the known-broken package.

## Current exact build

- Source and `main` head: `c9a925e923ff425d26eb878784cbe9ecd1403dd1`.
- GitHub Actions run: `29751994898`, all four jobs passed.
- Packaged-probe job: `88384610344`, passed.
- Artifact: `pulsar-signed-msix-probe`, artifact ID `8465025630`.
- Artifact archive digest:
  `sha256:41faa1a435d572eb732bb580b9be5470b54f76206c3f26925f880bd894585c52`.
- Artifact expiry: `2026-08-03T14:47:05Z`.
- Package: `PulsarProbe-0.1.0.0-x64-signed.msix`.
- Package SHA-256:
  `1191699da98377f559f01312ae1c12fd0d456706d30224803bc19ee4c553b413`.
- Embedded recording cue SHA-256:
  `479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd`.
- Package identity, Publisher, PFN, AUMID, AppContainer/runtime and the exact
  four-capability set match the frozen package contract.
- Hosted install and cleanup receipts agree on the package digest. Hosted
  cleanup reports package/process/run-added trust/runtime absence, hotkey
  reacquisition and exclusive picker-fixture deletion. This is tooling-only.

The package uses a short-lived non-exportable test signer. The public signer
certificate embedded in this artifact is valid until
`2026-08-19T14:45:02Z`. If either artifact or certificate is no longer usable,
regenerate one package from a newly frozen exact source head and run both OS
rows with those same bytes. Never combine evidence from different package
hashes.

## Required delta from the 2026-07-14 freeze

The original frozen artifact predates later accepted Windows changes. The
current exact build adds the capture-quality ABI and Communications audio
category request while deliberately leaving native-effects verification false
until physical evidence exists. It also embeds the canonical recording cue
and pins its digest in the package contract. Therefore the old artifact may be
kept as historical harness evidence, but it must not be used for the final
current-build application matrix.

## Access re-audit

- The executor is still x86_64 macOS with no Windows/Boot Camp volume.
- One online Windows Tailscale peer is now visible. SSH and RDP ports answer,
  but no available SSH agent identity authenticates and no physical-machine,
  OS-family, device, console-operator or lifecycle attestation is available.
- Repository and organization self-hosted-runner inventory remains
  unauthorized to the current GitHub credential.
- GitHub-hosted Windows passed the deterministic evidence-kit contract but is
  explicitly inadmissible as physical audio evidence.

The discovered peer is a possible operator route, not a passed or qualified
host. Do not install the package there until its owner confirms that it is a
physical machine in the approved matrix and provides a physical-console
window.

### Second access audit

The strict continuation repeated the access audit on 2026-07-20 without
changing the peer:

- Tailscale identifies the peer as Windows and associates it with Ivan Oparin,
  but exposes no edition/build, hardware model, VM/physical status, audio
  endpoints or console attestation.
- TCP 22, 135, 445 and 3389 answer. WinRM 5985/5986 does not answer; anonymous
  SMB authentication is rejected.
- OpenSSH reports `OpenSSH_for_Windows_9.5`. Ten available agent public keys
  were tried non-interactively against the five plausible local/Microsoft
  account names; none was accepted. Password and interactive guessing were not
  attempted.
- Taildrop rejects the transfer because the local node and peer are owned by
  different Tailscale users. No artifact reached the Windows peer.
- RDP answering is not sufficient: it still needs an authorized credential and
  a physical-console operator, and an RDP session cannot attest the required
  sleep, lock, device-removal, microphone and audible-output behavior.

The owner confirmation makes the peer admissible for Windows 10 preflight, but
the existing agent keys still cannot authenticate. A reviewed one-time
physical-console bootstrap is therefore prepared in
`pulsar-win/probe-msix/bootstrap-hardware-host.ps1`. It adds only the pinned
`ivan@relux.works` Ed25519 public key to the administrators OpenSSH key file,
preserves unrelated keys, records a sanitized preflight receipt, and does not
change sshd, firewall, password, package or H00-H17 state.

From an elevated PowerShell at the physical console, in a current repository
checkout, run:

```powershell
Set-ExecutionPolicy -Scope Process Bypass -Force
.\pulsar-win\probe-msix\bootstrap-hardware-host.ps1 `
  -Mode Install `
  -PhysicalMachineAttested `
  -ConsoleOperatorAttested
```

Return only the printed `SSH_ACCOUNT` value. The executor can then transfer
the exact package, collect preflight, and automate every non-gesture step. The
key can be removed after bundle sealing with `-Mode Remove`.

## Windows 10 physical preflight checkpoint

The reviewed key bootstrap succeeded and noninteractive SSH now authenticates
as the dedicated local `admin` account. The sanitized receipt records:

- physical Apple `MacBookPro13,2`, with no hypervisor reported;
- Windows 10 Pro 22H2, build `19045.6456`, x64 client installation;
- Developer Mode off;
- built-in `Internal Microphone (Cirrus Logic CS8409 (AB 54))` and physical
  Cirrus Logic speakers present and healthy;
- no second removable/selectable physical input; the owner explicitly chose
  the built-in-only profile for this pass.

The owner-selected host is recorded as a test-only `ApprovedException` posture
with decision reference `Ivan Oparin 2026-07-20: mbpro-win Windows 10 host
approved for this pass`; it is not a product-support promise for EOL Windows
10 Pro.

The latest stable Microsoft Windows SDK `10.1.28000.2270` was installed from
the official Microsoft download. Setup returned `0`, required no reboot and
left Developer Mode off. `appcert.exe` version `10.0.28000.2270` is present,
has valid Microsoft Authenticode and SHA-256
`36deb040365311f884aae28bea130eb5aae598471d41afb9a6cbaa16adb243aa`.

The repaired exact MSIX and metadata are staged on the physical host. The
Windows-side MSIX SHA-256 matches `1191699d...b413`. The package remains
uninstalled and its test signer remains untrusted so H00 can begin from the
required clean state.
Evidence-kit initialization uses the recorded single-input exception with the
built-in input as both default and selected. The harness forbids `PASS` for
H04, H08 and H12 under this profile; those rows must be `BLOCKED` with the
distinct/removable-device next action. All other applicable scenarios proceed
in strict order beginning with clean H00.

The historical immutable run directory `win10-single-input-physical-a` was
initialized from the old package and retains the failed H00 diagnostic. A
pre-run hotkey probe from the
OpenSSH service session (session 0) found `Ctrl+Shift+R` unavailable while the
physical console is session 1; H05 therefore remains undecided until exercised
inside the active console session. This does not block H00-H04 ordering.

## First H00 result and repair checkpoint

The frozen MSIX installed with its exact expected identity and was activated
through its AUMID in interactive session 1. It did not create the main probe
window. A same-session UI Automation diagnostic captured the terminal startup
dialog `required startup evidence is unavailable`; the runtime JSONL contained
only `helper_load` attempts and no microphone action. Windows identified the
active desktop as `rdp-tcp#3`, not a local `console` session. H00 is therefore
immutably recorded `FAIL`, H01 was not started, and no permission prompt was
triggered.

The physical run exposed three deterministic gaps that hosted packaging had
not exercised:

- `io.MultiWriter(logFile, os.Stderr)` wrote the primary JSONL row but returned
  the packaged GUI's invalid stderr error, making startup fail closed;
- the app appended `Packages/<PFN>/LocalState` below an already virtualized
  AppContainer `LOCALAPPDATA`, while host tools inspected the conventional
  LocalState path;
- the evidence sanitizer classified the semantic field `selectedApiPath` as a
  filesystem path and rejected a valid runtime log.

The repair makes the evidence file the sole authoritative logger, writes below
the app's virtualized `LOCALAPPDATA\\PulsarProbe`, points host install/snapshot/
cleanup tools at `Packages/<PFN>/AC/PulsarProbe`, and explicitly distinguishes
`selectedApiPath` from a filesystem location. Local Go tests and Windows amd64
cross-build pass. The full PowerShell evidence contract suite also passed from
the active interactive Windows session. The failed package, run-added signer
trust and package data were removed; the failed immutable bundle remains for
diagnostic review. Strict execution stays at H00 and must start a new immutable
run from the next CI-built signed package. An H00 pass also requires the active
desktop to be transitioned or confirmed as the local physical console rather
than RDP.

## Repaired post-merge artifact and bundle B checkpoint

PR #304 merged the repair to `main` at
`c9a925e923ff425d26eb878784cbe9ecd1403dd1`. Post-merge run `29751994898`
passed all four jobs. Artifact `8465025630` was downloaded and independently
re-hashed before transfer; the Windows host reports the same package SHA-256
`1191699da98377f559f01312ae1c12fd0d456706d30224803bc19ee4c553b413`.

An engineering-only launch smoke installed this package in the existing
interactive RDP session, activated its AUMID and observed the visible top-level
window `Pulsar packaged Windows probe`. Runtime evidence contained successful
`helper_load` and `controls_ready` events at the corrected
`Packages/<PFN>/AC/PulsarProbe` root. The same smoke also recorded RDP/AppContainer
blocks for tray, global hotkey and repeated permission-status queries; these
are preserved as findings for their strict scenarios and are not counted as an
H00 result. Interactive cleanup then proved process/package/run-added signer
trust/runtime absence, hotkey reacquisition and picker-fixture deletion.

The new immutable directory `win10-single-input-physical-b` is initialized
from that clean state. It freezes the repaired package digest, Windows 10
ApprovedException posture, and `single-input-owner-approved` profile with
`Internal Microphone (Cirrus Logic CS8409 (AB 54))` as both default and
selected input.

## Bundle B H00 physical-console result

Session 1 was transitioned from `rdp-tcp#3` to the local physical `console`.
The operator was physically present, used a one-shot desktop launcher for the
exact AUMID, confirmed that no microphone permission prompt appeared at
launch, and closed the visible window after observation. Retry 2 captured the
visible `Pulsar packaged Windows probe` window, required controls, selected
built-in input, screenshot SHA-256
`dc7240d44eef8fd652ef5d3af1b045359fe394828585c02c78bf56f7e7a38813`,
successful `helper_load` and `controls_ready`, and a stable runtime snapshot.

H00 is `PASS` with 11 immutable evidence references. This verdict is strictly
limited to exact signed install, package identity, exact AUMID launch and the
visible control surface without Developer Mode or family replacement. It gives
no credit to permission, tray, hotkey, lifecycle, capture or cleanup. The
close control hid the window but did not terminate the process because tray
registration was blocked; the process was explicitly terminated only for the
stable snapshot, and that limitation is attached. Repeated
`permission_status_query` failures and tray/hotkey `Access denied` remain open
findings for their strict scenarios.

The unverdict first retry process remained hidden overnight and amplified the
repeated permission failure into a 2,741,466,867-byte diagnostic log. Its
SHA-256 `a30d2b1ed8a14c9b691e00de7a6ee60d4a16ee36ef317a29bfc76099572bb549`,
size, reset boundary, first startup observation and screenshot are retained in
compact evidence; the repetitive 2.74 GB raw file was deliberately excluded
and deleted before bundle attachment.

## Bundle B H01 physical-console result

The package-specific microphone consent entry was removed before launch and
the probe started in local console session 1. Baseline screenshot SHA-256 is
`1ccf84345fb39081b627087288baec3faf725186355b738a1133a21270e3270d`.
Ivan Oparin pressed `Record default` exactly once and observed that nothing
happened: no Windows microphone prompt appeared. The intent was logged at
`2026-07-21T09:40:44.129238Z`; the post-action screenshot SHA-256 is
`1b1b669bca9eaa2fb36db97793b23c396fa8434dcd1df0fb4afaeefbeb2c3eea`
and UI Automation found only the visible Pulsar window.

H01 is immutably `FAIL` with seven evidence references. The run produced zero
permission-request events, zero `capture_started` events and zero promotable
WAV files. Instead, the waiter-owned `CapPermissionCheck` repeatedly returned
`0x8001010e` (`RPC_E_WRONG_THREAD`): 268,515 such failures among 625,425
events. The probe was force-stopped only to bound the runaway logger; normal
lifecycle credit is explicitly excluded. The 323,535,561-byte raw log has
SHA-256 `df9bb218e44f05ae40b162b65acab705a083c93a1b5d421efdef07597fd5d941`;
its hash, counts and representative event extract are retained while the
repetitive raw file was excluded and deleted.

The concrete next action is to repair the native AppCapability apartment/thread
ownership and bound repeated permission-query failure evidence, add regression
coverage, build and sign a new exact MSIX, and restart at H00 in a new
immutable bundle. H02 is not run against the known-broken package.

## Exact manual execution request

The active pass now requires the confirmed Windows 10 row first; the complete
task still requires both admissible rows:

1. physical x64 Windows 10 Enterprise LTSC 2021 build 19044, fully patched and
   licensed, or an explicit approved lifecycle exception;
2. physical Windows-11-compatible x64 machine on a currently serviced build;
3. on this owner-approved Windows 10 pass, a dedicated admin-capable test
   account, audible output, the built-in physical microphone, WACK, screenshot
   capture, sleep/wake, lock/unlock, sign-out and privacy control access; H04,
   H08 and H12 remain `BLOCKED` until a second removable/selectable microphone
   is later supplied;
4. permission to install/remove the signed test MSIX and its embedded public
   signer, plus a safe route to return sanitized sealed bundles.

Download the exact artifact on Windows before expiry:

```powershell
gh run download 29751994898 `
  --name pulsar-signed-msix-probe `
  --dir .\dist\windows-probe
Get-FileHash `
  .\dist\windows-probe\PulsarProbe-0.1.0.0-x64-signed.msix `
  -Algorithm SHA256
```

The printed digest must equal
`1191699da98377f559f01312ae1c12fd0d456706d30224803bc19ee4c553b413`.
Then follow `pulsar-win/probe-msix/README.md` and the frozen H00-H17 order in
`TASK-260712-1vtwkl_hardware-readiness-audit.md`: complete Windows 10 first,
seal its immutable bundle, then complete Windows 11 with the same MSIX bytes.

## Progress and stop condition

- Physical rows accepted: `1/36`. Current bundle B is `2/18` terminal with
  H00 `PASS`, H01 `FAIL` and seven H01 evidence references. Historical bundle
  A retains its separate H00 `FAIL`. The next executable step is a repaired
  signed build and a fresh bundle beginning at H00.
- Task checklist accepted: `0/4`.
- Overall epic progress: `186/205` accepted (`90.7%`).
- Engineering progress: `186/186` accepted (`100%`).
- Manual epic progress: `0/19` accepted (`0%`).

Strict ordering prevents starting `TASK-260712-2hodti` or any later manual
task. The Windows 10 row may proceed and be sealed independently, but this task
cannot close until the later Windows 11 bundle is also attached and reviewed.
A failed row must retain FAIL/BLOCKED plus its concrete next action.
