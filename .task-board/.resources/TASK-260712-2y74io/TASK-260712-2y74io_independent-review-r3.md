# TASK-260712-2y74io — independent R3 review

Date: 2026-07-13
Role: reviewer
Verdict: BACK TO DEVELOPMENT

I reviewed the complete task card and guards, producer R3 outcome, every producer-listed file in full, the lifecycle diagram, docs/spec-self-contained-audio.md sections 3.13, 18, and 19 P1.0/P1.7, the accepted Rev16 bridge contract and root acceptance under TASK-260712-dib11l, and the root-review amendments. Because pulsar-win is untracked, I inspected current files directly and recomputed all ten producer SHA-256 values; every hash matches the R3 outcome. No product file was modified.

## Blocking findings

### F1 — HIGH — confirmed WM_ENDSESSION executes ordinary release paths before the abrupt waiter exit

Locations:
- pulsar-win/cmd/pulsar-win-probe/main_windows.go:388-403
- release calls reached by those drains at main_windows.go:619, 698, 746, 771, 805, 1015, 1044, 1062, and 1110
- missing production assertion in lifecycle_source_test.go:47-117; current shutdown tests exercise only lifecycleTracker state

Failure schedule:
1. The hidden window handles confirmed WM_ENDSESSION, requests the nonblocking stop, signals shutdownEvent, and returns.
2. The waiter observes shutdownEvent and sets confirmedShutdown at line 388.
3. Before checking that flag at line 401, it executes drainTerminalIntent, drainCommands, all permission/discovery/default/capture/picker drains, artifact retry, UI transitions, and evidence failure handling.
4. Any terminal operation reached by those drains invokes its ordinary Release export. An already-finalized capture takes CaptureRelease at line 771; a newly terminal capture reaches line 1015. Permission, enumeration, default-device, and picker operations have equivalent release calls.

This violates the accepted Rev16 WM_ENDSESSION contract: the waiter may best-effort drain buffered capture data but must not call CaptureRelease or CapDestroy; it must exit immediately and leave process resources to Windows. Rev16 also requires a production test proving zero CaptureRelease and zero CapDestroy on this path. The current source-presence and tracker tests do not exercise waiter dispatch or helper release counts.

Required correction:
- branch on confirmed shutdown before the ordinary drain sequence;
- use a shutdown-specific best-effort drain that cannot call any Release export, cannot post CLEANUP_READY, and exits the waiter;
- add a deterministic production-seam test with active capture plus pending permission/picker/discovery operations and coalesced signals, asserting stop precedes shutdown wake, buffered drain is bounded, every release count is zero, CapDestroy/CLEANUP_READY are absent, and the waiter exits.

Violated invariant/AC: accepted Rev16 imminent-process-termination ownership, R10 production-seam evidence, and the task requirement for an honest bounded shutdown path without contradictory cleanup behavior.

### F2 — HIGH — sticky evidence failure does not stop already-queued passing rows from being written

Locations:
- pulsar-win/cmd/pulsar-win-probe/coordinators.go:167-180 and 211-240
- lifecycle_r3_test.go:521-593 and 595-638

Failure schedule:
1. Required row A is accepted while healthy and blocks in logFn.
2. Before A returns its injected error, later passing cleanup row B is accepted into operations because failed is still false.
3. The worker receives A, sets failed=true, then unconditionally invokes logFn for B on its next loop iteration.
4. B can be durably written even though its required predecessor is missing. A synchronous caller eventually returns false because failed is sticky, but that does not remove the already-written passing row.

This contradicts the R2-F5 correction contract that no passing cleanup claim survive missing required evidence and undermines the task AC requiring an ordered evidence path. Existing tests call a later clean claim only after the first failure is already visible, or test saturation order; they do not queue a passing lifecycle row behind an in-flight failing prerequisite.

Required correction:
- once the worker observes sticky failure, do not invoke logFn or syncFn for subsequent queued operations; return a sticky failure to synchronous waiters and discard asynchronous follow-ons;
- add barrier tests for blocking and nonblocking row B queued before row A reports failure, using real lifecycle pass actions and asserting the underlying writer never sees B.

## Architecture and scope

The supplied PlantUML flow remains a valid high-level one-purpose lifecycle sequence. F1 is an implementation divergence from its Windows OS handoff branch and the more precise Rev16 ownership contract. AppContainer boundaries are preserved: manifest tests confirm no runFullTrust or broad filesystem capability and the reviewed capability set remains exact. Privacy and temporary-artifact checks pass. Signed Windows 10/11 AppContainer delivery, hardware capture, WTS/power events, WACK, MSVC helper execution, and actual WM_ENDSESSION timing remain honest downstream gates.

## Independent verification

From pulsar-win:

- test -z "$(gofmt -l ...ten reviewed Go files...)" — PASS, no output.
- go test -count=50 ./cmd/pulsar-win-probe -run 'TestR3F|TestR3WindowsWiring' — PASS, 1.465s.
- go test -count=50 ./internal/winprobe -run 'TestSanitizeLogEvent' — PASS, 0.338s.
- go test -count=1 ./... — PASS: root 3.114s, probe 0.790s, winprobe 2.749s, wire 1.090s.
- go test -race -count=1 ./... — PASS: root 4.565s, probe 1.462s, winprobe 4.054s, wire 1.777s.
- go vet ./... — PASS, no output.
- GOOS=windows GOARCH=amd64 go build -o /tmp/TASK-260712-2y74io-review-probe.exe ./cmd/pulsar-win-probe — PASS.
- GOOS=windows GOARCH=amd64 go vet ./... — PASS.
- Windows amd64 test compilation for probe and winprobe — PASS.
- xmllint --noout probe-msix/AppxManifest.xml.in — PASS.
- focused artifact/manifest/sandbox/package/privacy suite — PASS, 0.356s.
- Rev16 windows-consistency-check.sh — PASS, 0 anti-patterns.
- PlantUML start/end delimiter checks — PASS.
- git diff --check — PASS.
- task-board validate — PASS.

Unavailable on this Darwin host: pwsh, cmake, cl, msbuild, MakeAppx, appcert, plantuml, native Windows execution, signed MSIX/AppContainer, and Windows 10/11 hardware.

Green verification establishes host correctness, race cleanliness for portable code, cross-buildability, and sandbox preservation. It does not cover either production failure schedule above.