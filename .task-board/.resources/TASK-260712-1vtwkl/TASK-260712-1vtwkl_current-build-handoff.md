# TASK-260712-1vtwkl current-build handoff

Date: 2026-07-20 (Asia/Tbilisi)

Status: BLOCKED ON PHYSICAL-CONSOLE EXECUTION. This handoff prepares the exact
current build for the manual Windows 10/11 matrix. It is not physical-hardware
evidence and does not pass H00-H17.

## Current exact build

- Source and `main` head: `fc6656a9f20eeba4cc1f907475598667bf556b67`.
- GitHub Actions run: `29735606631`, all four jobs passed.
- Packaged-probe job: `88330064941`, passed.
- Artifact: `pulsar-signed-msix-probe`, artifact ID `8458213808`.
- Artifact archive digest:
  `sha256:9e0fb800a132449054a059f204a5939b07a72c57b0314923c4c95c9fc0992f0c`.
- Artifact expiry: `2026-08-03T10:38:29Z`.
- Package: `PulsarProbe-0.1.0.0-x64-signed.msix`.
- Package SHA-256:
  `869f5d5613419be30c64097b8d9cf9ac62bc2277db42464b6dbf7dc973c679f5`.
- Embedded recording cue SHA-256:
  `479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd`.
- Package identity, Publisher, PFN, AUMID, AppContainer/runtime and the exact
  four-capability set match the frozen package contract.
- Hosted install and cleanup receipts agree on the package digest. Hosted
  cleanup reports package/process/run-added trust/runtime absence, hotkey
  reacquisition and exclusive picker-fixture deletion. This is tooling-only.

The package uses a short-lived non-exportable test signer. The public signer
certificate embedded in this artifact is valid until
`2026-08-19T10:36:17Z`. If either artifact or certificate is no longer usable,
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

## Exact manual execution request

Provide a physical-console operator and both admissible rows:

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
gh run download 29735606631 `
  --name pulsar-signed-msix-probe `
  --dir .\dist\windows-probe
Get-FileHash `
  .\dist\windows-probe\PulsarProbe-0.1.0.0-x64-signed.msix `
  -Algorithm SHA256
```

The printed digest must equal
`869f5d5613419be30c64097b8d9cf9ac62bc2277db42464b6dbf7dc973c679f5`.
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
task. Resume only after both sealed bundles are attached and independently
inspected; a failed row must retain FAIL/BLOCKED plus its concrete next action.
