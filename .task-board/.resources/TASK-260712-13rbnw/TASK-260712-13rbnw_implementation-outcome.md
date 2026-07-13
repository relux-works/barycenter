# TASK-260712-13rbnw — implementation outcome

Date: 2026-07-14  
Role: inline developer  
Handoff state: implementation complete; ready for frozen independent review and root audit. Real Windows 10/11 hardware evidence remains downstream.

## Outcome

The probe now has one reproducible signed-MSIX contract under the current Partner Center product identity:

- product `9P26FDCWV1GC`, package identity `ReluxWorksLLC.PulsarBarycenter`;
- Publisher `CN=60105954-A0D9-4E89-B32D-18AF2F423ABE`;
- package family `ReluxWorksLLC.PulsarBarycenter_q036g2bzd7ngc`;
- AUMID `ReluxWorksLLC.PulsarBarycenter_q036g2bzd7ngc!PulsarProbe`;
- x64 `packagedClassicApp` at `appContainer` trust;
- exactly the reviewed three network capabilities plus `microphone`, with no extension, `runFullTrust`, broad-filesystem, or library declaration.

`new-test-signing-certificate.ps1` creates a short-lived non-exportable CurrentUser Code Signing key whose Subject exactly equals the manifest Publisher. `build-probe.ps1` runs native and Go gates, packs with MakeAppx, signs with SignTool `/fd SHA256`, confirms the embedded signer, and emits a digest plus machine-readable contract. No PFX, password, private key, or certificate export is committed or uploaded.

`install-probe.ps1` reads and validates the manifest inside the untrusted archive before changing trust. The parser prohibits DTDs, external resolution, documents over 128 KiB, extra root/application declarations, target-family drift, capability drift, and identity drift. The opt-in local route trusts only the embedded self-signed Code Signing public certificate with the exact frozen Subject, revalidates Authenticode, detects package mutation, installs without replacing an existing production-family package, validates the installed manifest/PFN/architecture, optionally launches the exact AUMID, and writes a receipt without local absolute paths or signer data. Failure removes any task-installed package and newly added trust.

The Store route creates an explicitly unsigned Partner Center upload candidate with the same identity. It does not submit or mutate Partner Center; Store signing remains an authorized external action.

## Acceptance and checklist mapping

| Requirement | Exact implementation/evidence | Result |
| --- | --- | --- |
| Build and obtain a signed MSIX without guessing | `new-test-signing-certificate.ps1`, `build-probe.ps1`, CI artifact `pulsar-signed-msix-probe` | PASS |
| Current Partner Center identity, Publisher, family, x64 architecture | manifest, `package-contract.ps1`, Go and PowerShell negative regressions, build/install JSON | PASS |
| Preserve AppContainer and least-capability posture | exact declaration validator rejects extra root/application declarations and any capability delta | PASS |
| Local signing route | non-exportable test key, exact Subject, Code Signing EKU, SignTool SHA-256, no private export | PASS |
| Store signing route | documented unsigned candidate for product `9P26FDCWV1GC`; no submission claim | PASS, route only |
| Install and run without guessing | `install-probe.ps1 -TrustLocalTestSigner -Launch`; exact PFN/AUMID and bounded process observation | PASS for hosted install; launch command not executed in hosted CI |
| Logs and artifacts named | exact `%LOCALAPPDATA%\Packages\<PFN>\LocalState\PulsarProbe` layout and receipt fields | PASS |
| Real Win10/Win11 hardware proof | deliberately delegated to `TASK-260712-1vtwkl` | NOT CLAIMED |

All three board checklist items are satisfied by the files and evidence above.

## Verification on the final production bytes

Local host: macOS/Darwin. PowerShell, MakeAppx, SignTool, native MSVC execution, MSIX registration, WACK, and Windows hardware are unavailable locally.

```bash
cd pulsar-win
go vet ./...
go test ./...
```

PASS: root, probe command, `internal/winprobe`, and wire packages.

```bash
ruby -e 'require "yaml"; YAML.load_file(".github/workflows/ci.yml")'
git diff --check
```

PASS: workflow parses; diff has no whitespace errors.

GitHub Actions final run: `29292631211` (`https://github.com/relux-works/barycenter/actions/runs/29292631211`).

- coordinator: PASS;
- node-core: PASS;
- pulsar-win portable tests/resource generation/Windows cross-build: PASS;
- frozen package identity/declaration negative regressions: PASS;
- native CMake build and CTest: PASS, 1/1;
- Go vet/test and Windows GUI build inside package job: PASS;
- MakeAppx and SignTool SHA-256: PASS;
- trusted signed package registration and installed-contract receipt: PASS;
- signed artifact upload: PASS.

Downloaded final artifact inspection:

- signed MSIX SHA-256: `a0c3022b69c68f140969a7d7bef4cd0904f1b2872960e7a4511bea9462749be7`;
- manifest: current identity/Publisher, x64, `appContainer`, `packagedClassicApp`, exact four capabilities;
- package inventory: six PNG assets, probe EXE, capture DLL, manifest, block map, content types, code-integrity catalog, and package signature; no PFX or private-key file;
- install receipt: PFN/AUMID above, package digest matches build metadata, relative scenario/evidence paths, explicit hosted-install-only boundary.

## Exact source inventory and SHA-256

| File | SHA-256 |
| --- | --- |
| `.github/workflows/ci.yml` | `ae18fef8fcf8c3365df666cc3d166468e5343546aa33bee3bdfb21f029bdd5d8` |
| `pulsar-win/internal/winprobe/manifest.go` | `ca19cc43e90d0eaffa62f6e61250f6e18ac8aa59a65afb7da31a9ffe2dff4e8f` |
| `pulsar-win/internal/winprobe/manifest_test.go` | `f7b7a56823fd88c15951111fad7f149239eba27c72d33bcbd5ae4d25c2d0d672` |
| `pulsar-win/probe-msix/AppxManifest.xml.in` | `a59bbde45907273f181ca4ec7d68b102d5902b6ab51750a51cbaf48be61b98b9` |
| `pulsar-win/probe-msix/README.md` | `2b95802a902a2fd0292a55e791e1e4fcf149f4e9a2eb1df5695db009d2a62fa4` |
| `pulsar-win/probe-msix/build-probe.ps1` | `51216823b446189b3422031dbdebde91efc04213690792dcfebf141655708248` |
| `pulsar-win/probe-msix/install-probe.ps1` | `27b2d19ce5a8aebe28bed0d91ceb32785fe4cbf59825aa35f5f73545b690d6e6` |
| `pulsar-win/probe-msix/new-test-signing-certificate.ps1` | `de88cd1bc424c4d92234d71a296a851f6501d78ad94e67849de56025c988e362` |
| `pulsar-win/probe-msix/package-contract.ps1` | `30a0e8582b8f0b445766d15a634f70450bbc1ef6a6eb12da2d1146b6f80f5c61` |
| `pulsar-win/probe-msix/package-contract.Tests.ps1` | `02ee1aef9a3ca66f83e648e44a08a640ec1db18dde63be2c43d2953aff6c4c36` |

The hashes above were captured at implementation commit `f5b73f06a9e06f71c6193d982e6138e5bec68247`; the final audit must recalculate them before acceptance.

## Rollback and cleanup

On a test host, stop `pulsar-win-probe-amd64`, remove `ReluxWorksLLC.PulsarBarycenter` with `Remove-AppxPackage`, and remove the Trusted People entry only when `SignerTrustAdded` is true. The README contains exact commands. Source rollback is the ordinary revert of this task's commits; no schema, persistent server state, Partner Center state, or production deployment was changed.

## Residual gates

- The hosted runner proves signed package creation and registration, not GUI activation, microphone capture, lifecycle delivery, or real hardware.
- WACK and Partner Center certification were not run or claimed.
- `TASK-260712-1vtwkl` must install the signed package on real Windows 10 19041+ and Windows 11 machines and collect the full hardware/lifecycle matrix before the foundation section can close.
