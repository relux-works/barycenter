# Phase 1 acceptance environment

This is the reproducible engineering gate for Phase 1. It generates repository
and CI evidence; it does **not** pass real-app, audible, physical-hardware,
Windows 10/11, WACK, screenshot, or Partner Center checks. Those observations
belong to `EPIC-260714-th54l3` and remain `manual-required` until a human records
them. The evidence boundary is shown in
[`p1-acceptance-environment.puml`](../diagrams/p1-acceptance-environment.puml).

## Frozen toolchains

The machine-readable authority is [`acceptance/toolchains.json`](../../acceptance/toolchains.json):

- Go 1.25.12 (`GOTOOLCHAIN=go1.25.12`) for both Go modules;
- Xcode 26.2 build 17C52 with Swift 6.2.3; `xcrun swift`, never the standalone
  Command Line Tools Swift, runs the package tests;
- GitHub hosted runner images `ubuntu-24.04`, `macos-15`, and `windows-2025`.

Runner labels are explicit to avoid silent `-latest` image migrations; see the
[GitHub-hosted runners reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners).

WACK is different from the language toolchains: the operator must use the
current Store-supported Windows SDK and record the installed WACK file version.
The expected executable is `C:\Program Files (x86)\Windows Kits\10\App
Certification Kit\appcert.exe`; see Microsoft's
[Windows App Certification Kit instructions](https://learn.microsoft.com/en-us/windows/uwp/debug-test-perf/windows-app-certification-kit).

## One-command automated gate

Prerequisites are Git history (the pinned predecessor commit must be present),
Python 3.9+, Go with toolchain download enabled, ffmpeg/ffprobe, and the pinned
full Xcode when running Swift.

```sh
python3 scripts/acceptance/run_automated.py --suite all --require-clean
```

The available suites are `coordinator`, `windows`, `swift`, and `all`. The gate
runs vet, full unit/golden/migration suites, the moderation contract check, the
exact previous-coordinator rollback suite, the Windows race detector, and
Windows amd64/arm64 cross-builds. A missing or mismatched pinned toolchain is a
failure; it is never waived. The full Xcode lookup checks an explicit
`DEVELOPER_DIR`, then `Xcode_26.2.app`, then `Xcode.app`, but accepts only the
exact pinned version and build.

Each invocation creates a new mode-0700 directory beneath
`.temp/acceptance/<UTC-run-id>/`. It contains one sanitized log per command and
`manifest.json` with commit, start/end dirty-state and sanitized dirty paths,
host, exact toolchains, command exit
codes, relative artifact names, sizes, and SHA-256 hashes. Repository/home paths
and common token/credential forms are redacted before logs are persisted. Raw
subprocess output is never written to disk. Use `--require-clean` for evidence
that can be promoted; it rejects both a dirty start and repository drift caused
by the suite itself.

The P1 engineering/manual boundary is frozen separately in
[`phase1-engineering-readiness.json`](../../acceptance/phase1-engineering-readiness.json).
Its fail-closed validator is part of the acceptance contract tests and can be
run directly:

```sh
python3 scripts/acceptance/validate_phase1_readiness.py
```

It verifies exact Git/tree and evidence hashes, distinguishes the GitHub
Actions API head from the checked-out PR merge-ref, requires all four hosted
artifacts, maps every A1-A8 scenario to the ordered manual P1 program and keeps
release, Store submission and Partner Center mutation false.

## Nonproduction migration and rollback rehearsal

The automated gate creates fresh temporary databases and executes the current
schema, exact immutable predecessor source, previous-authority mutations, and
current reconciliation. It covers identity, uploads, processing, lifecycle,
integrated media, transmissions, and moderation. It never opens a production
database.

```sh
python3 scripts/acceptance/run_automated.py \
  --suite coordinator --require-clean --run-id phase1-rollback-rehearsal
```

The run is acceptable only when the `previous-head-rollback` command is present
with exit code zero in the manifest and the manifest itself belongs to the
intended commit. Preserve the complete run directory; do not copy database
fixtures or unsanitized data into evidence.

## Real-app topology and deterministic start

Use only a new nonproduction coordinator database. The logical seed contract is
[`phase1-topology.json`](../../acceptance/fixtures/phase1-topology.json): one
acceptance orbit, two members, and two separately onboarded Pulsars. Generate
installation/recovery secrets at runtime and never put them in the fixture,
screenshots, command history, or artifacts. Optional Spotify playback uses an
operator-owned account and is not a prerequisite for generic-media A1-A8.

Prepare the current schema and exactly that logical topology with:

```sh
cd coordinator
GOTOOLCHAIN=go1.25.12 go run ./cmd/phase1-acceptance-fixture \
  -db ../.temp/acceptance/phase1-live/private/coordinator.db \
  -credentials ../.temp/acceptance/phase1-live/private/credentials.json \
  -confirm-nonproduction
```

Both paths must be new and remain below the repository acceptance directory.
The credential file is mode 0600 and is deliberately excluded from promotable
evidence. The command emits only IDs; it never prints secrets. Move each
installation's material directly into the appropriate OS credential store,
then securely remove the credential file when onboarding is complete.

Before sampling:

1. Build and sign one commit; record commit, package SHA-256, package identity,
   signer subject, and version in both machine result files.
2. Start the clean temporary coordinator, onboard Pulsar A and B independently,
   and verify their distinct installation IDs without recording credentials.
3. Synchronize coordinator and both machines to named UTC time sources. Record
   source, offset, uncertainty, and measurement timestamp; reject a run if any
   absolute offset or uncertainty exceeds 25 ms.
4. Wait for both clients' ready barrier. Do not substitute process-up or socket
   connected for application ready.

Clean Windows 10 and Windows 11 install/image preparation, OS build numbers,
driver/device inventory, package install, real audio, and screenshots are manual
evidence in `EPIC-260714-th54l3`.

## WACK

Run from an elevated terminal in the active interactive user session (not
Session 0):

```powershell
./scripts/acceptance/run_wack.ps1 `
  -PackagePath ./dist/Pulsar.msix `
  -RunId 20260715-win11-wack
```

The script resets WACK, runs the package test, and writes the XML plus a manifest
containing package/report hashes, commit, WACK version, and timestamps. It fails
if elevation, interactivity, the executable, package, or report is missing. Exit
zero means the tool completed; the operator must still review the XML and set
the manual task result. Hosted CI deliberately does not claim a WACK pass.

## Sampling and artifact contract

Use three warm-ups, then at least 30 successful measured samples for each p95.
Failures are recorded and fail the run; they are never dropped from the result.
The calculator uses nearest-rank p95 (`ceil(0.95 * n)`) and enforces:

- 15-second clip stop-record to audible p95 at most 4,000 ms;
- ready-barrier synchronized start skew p95 at most 100 ms;
- peak application memory at most 250 MiB while processing/playing the maximum
  Phase 1 clip (180 seconds, 50 MiB input limit).

Copy [`phase1-metrics.csv`](../../acceptance/templates/phase1-metrics.csv), add
rows, and evaluate it without changing the original capture:

```sh
python3 scripts/acceptance/evaluate_metrics.py metrics.csv --output metrics-result.json
```

For timing, capture monotonic timestamps at the recorder stop, both clients'
ready barrier, scheduled start, and first audible-buffer callback. For memory,
sample the Pulsar process working set at least once per second from before
intake through terminal playback; retain the maximum, tool name, tool version,
and raw sanitized CSV. Screenshots must show only the relevant result and OS/app
version, with identities, paths, tokens, notifications, and unrelated windows
redacted before copying to evidence.

Start each manual machine result from
[`manual-result.json`](../../acceptance/templates/manual-result.json). Store only
sanitized relative paths below `.temp/acceptance/<run-id>/`, add SHA-256 for every
file to the result, and preserve originals outside Git. A missing sample,
provenance field, hash, screenshot, WACK report, or manual review is `fail` or
`manual-required`, never `pass`.
