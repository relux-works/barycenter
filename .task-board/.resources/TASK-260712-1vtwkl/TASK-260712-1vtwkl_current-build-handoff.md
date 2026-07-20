# TASK-260712-1vtwkl current-build handoff

Date: 2026-07-20 (Asia/Tbilisi)

Status: IN PROGRESS — OWNER-APPROVED BUILT-IN MICROPHONE PROFILE. Ivan
Oparin confirmed `mbpro-win` as the physical Windows 10 host and authorized
this row first with maximum autonomous execution. SSH, sanitized preflight,
exact package transfer and WACK preparation are complete. Ivan Oparin then
limited this pass to the built-in microphone. Windows 11 is intentionally
deferred for this pass. No H00-H17 verdict is claimed yet.

## Current exact build

- Source and `main` head: `f4a90f1f332bc73cac8f36f96cee6c16cc2ad7c0`.
- GitHub Actions run: `29738846385`, all four jobs passed.
- Packaged-probe job: `88340580425`, passed.
- Artifact: `pulsar-signed-msix-probe`, artifact ID `8459523515`.
- Artifact archive digest:
  `sha256:ad8ab25b544017950f62b3d9e5ea2da2c04d7c8e0eca4ab3ea05b4e9a8bcbfc7`.
- Artifact expiry: `2026-08-03T11:33:53Z`.
- Package: `PulsarProbe-0.1.0.0-x64-signed.msix`.
- Package SHA-256:
  `a53253f33c5d9acf903daa4641c254884eb3e69d497e6442bdb1cd4e85d6b7e6`.
- Embedded recording cue SHA-256:
  `479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd`.
- Package identity, Publisher, PFN, AUMID, AppContainer/runtime and the exact
  four-capability set match the frozen package contract.
- Hosted install and cleanup receipts agree on the package digest. Hosted
  cleanup reports package/process/run-added trust/runtime absence, hotkey
  reacquisition and exclusive picker-fixture deletion. This is tooling-only.

The package uses a short-lived non-exportable test signer. The public signer
certificate embedded in this artifact is valid until
`2026-08-19T11:31:53Z`. If either artifact or certificate is no longer usable,
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

The exact MSIX and metadata are staged on the physical host. The Windows-side
MSIX SHA-256 matches `a53253f3...b7e6`. The package remains uninstalled and its
test signer remains untrusted so H00 can begin from the required clean state.
Evidence-kit initialization uses the recorded single-input exception with the
built-in input as both default and selected. The harness forbids `PASS` for
H04, H08 and H12 under this profile; those rows must be `BLOCKED` with the
distinct/removable-device next action. All other applicable scenarios proceed
in strict order beginning with clean H00.

The immutable run directory `win10-single-input-physical-a` has now been
initialized successfully from the clean package/trust state. Its boundary is
initialized-only and no scenario is passed. A pre-run hotkey probe from the
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

## Exact manual execution request

The active pass now requires the confirmed Windows 10 row first; the complete
task still requires both admissible rows:

1. physical x64 Windows 10 Enterprise LTSC 2021 build 19044, fully patched and
   licensed, or an explicit approved lifecycle exception;
2. physical Windows-11-compatible x64 machine on a currently serviced build;
3. on both hosts, a dedicated admin-capable test account, audible output,
   default physical microphone, a second removable/selectable microphone,
   WACK, screenshot capture, sleep/wake, lock/unlock, sign-out and privacy
   control access;
4. permission to install/remove the signed test MSIX and its embedded public
   signer, plus a safe route to return sanitized sealed bundles.

Download the exact artifact on Windows before expiry:

```powershell
gh run download 29738846385 `
  --name pulsar-signed-msix-probe `
  --dir .\dist\windows-probe
Get-FileHash `
  .\dist\windows-probe\PulsarProbe-0.1.0.0-x64-signed.msix `
  -Algorithm SHA256
```

The printed digest must equal
`a53253f33c5d9acf903daa4641c254884eb3e69d497e6442bdb1cd4e85d6b7e6`.
Then follow `pulsar-win/probe-msix/README.md` and the frozen H00-H17 order in
`TASK-260712-1vtwkl_hardware-readiness-audit.md`: complete Windows 10 first,
seal its immutable bundle, then complete Windows 11 with the same MSIX bytes.

## Progress and stop condition

- Physical rows accepted: `0/36` (H00-H17 on two OS families).
- Task checklist accepted: `0/4`.
- Overall epic progress: `186/205` accepted (`90.7%`).
- Engineering progress: `186/186` accepted (`100%`).
- Manual epic progress: `0/19` accepted (`0%`).

Strict ordering prevents starting `TASK-260712-2hodti` or any later manual
task. The Windows 10 row may proceed and be sealed independently, but this task
cannot close until the later Windows 11 bundle is also attached and reviewed.
A failed row must retain FAIL/BLOCKED plus its concrete next action.
