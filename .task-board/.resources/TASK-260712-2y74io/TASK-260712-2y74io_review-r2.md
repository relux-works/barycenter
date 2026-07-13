# Independent review outcome — TASK-260712-2y74io — round 2

## Verdict

**BACK TO DEVELOPMENT.** The R1–R10 rework materially improves generation binding, stale-continuation suppression, shutdown gating, non-droppable quit intent, exit arbitration, waiter-only permission-query ownership, repeated-signal evidence, and tray ownership. Host/race/cross-build validation is green. Approval is still blocked by two production lifecycle failure paths; the green tests do not execute either schedule.

## Scope reviewed

Read in full:

- Task card, lifecycle PlantUML, implementation guard, independent review guard, R1 rework guard, producer R2 outcome, prior independent review, and root round-1 review.
- `docs/spec-self-contained-audio.md` §3.13, §18, and §19 P1.0/P1.7.
- Accepted bridge Rev16, accepted root review, and P1 root amendments.
- Every producer-changed production/test/documentation file in full:
  - `pulsar-win/cmd/pulsar-win-probe/lifecycle.go`
  - `pulsar-win/cmd/pulsar-win-probe/main_windows.go`
  - `pulsar-win/cmd/pulsar-win-probe/window_windows.go`
  - `pulsar-win/cmd/pulsar-win-probe/lifecycle_test.go`
  - `pulsar-win/cmd/pulsar-win-probe/lifecycle_rework_test.go`
  - `pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go`
  - `pulsar-win/probe-msix/README.md`
  - `LOGBOOK.md`
- Relevant helper, artifact, logging, query-failure, startup-cleanup, Win32 layout, manifest, and package-build seams.
- Because the probe tree remains untracked, the current files were inspected directly; git diff was not treated as implementation evidence.

Producer R2 SHA-256 inventory was independently recomputed and matches the outcome:
- lifecycle.go: `fe948dd433258e603159052ad5a785425844afe68e3a4a493e3f2937d224a326`
- main_windows.go: `a19ed77402dc1291442ab5d7932851520ee22a6c0eb73be5c0201c76af0b6bb8`
- window_windows.go: `91c6145f0ab114e7ed38f6ff5188019ec57abc1c8951c89dc32eca27aeb2cde3`
- lifecycle_test.go: `d6df215f601d7b5af2f91986f8e27b468c1a8d588e2cb8ab8216a85b6943ac05`
- lifecycle_rework_test.go: `b48b1bccb5a042d0693b1952c0a95380704d99ce65b55c3673fd207580accb88`
- lifecycle_source_test.go: `1a8903a5e9a614a14491c428d03107bd6e7b2cbcf1b2a14c72e5260ae41b1b7a`
- README.md: `186b8c04b82e749dac9c7b816a909614cb60f1aefa26f010f843be75b89e83a2`
- LOGBOOK.md: `22c1cda7a78cb576fed831f2ab6ac02c5741ee9f2821fb9ad941b01133045380`

## Blocking findings

### F1 — HIGH — a failed idle-cleanup PostMessage permanently strands lifecycle cleanup

**Locations:** `main_windows.go:1422-1436`, `main_windows.go:1463-1475`, `main_windows.go:1614-1620`; shutdown-cancel caller at `window_windows.go:395-402`.

**Failure schedule:** suspend, session lock, or permission revoke is observed with no native capture (or capture reaches release). The tracker advances the run to `capture_stop_requested` or `capture_released`. `postIdleLifecycleCleanup` makes one `PostMessageW(wmAppLifecycleIdleCleanup)` attempt and ignores the false return. If the critical UI post fails because of an invalid/tearing-down HWND, message-queue exhaustion, or another Win32 post failure, no pending-cleanup bit/event is retained and neither the bounded waiter loop nor a timer retries the transition. Permission polling no longer re-enters the path after `changed` becomes false. A later resume/unlock calls `beginRearm`, but it is rejected because the active lifecycle run has not reached `idle`. The hotkey therefore remains registered across the lifecycle edge and the run/start gate is stranded indefinitely. The same one-shot loss exists after shutdown cancellation and after a released capture.

**Violated invariant/AC:** every P1.0 edge must either complete ordered capture/artifact/hotkey cleanup or record a concrete platform limitation and next action; repeated cycles must not leave a leaked hotkey or stuck lifecycle gate. Logging that a critical message failed is not cleanup or a recoverable terminal disposition.

**Required correction:** make idle-cleanup intent non-droppable and independently observable, with a UI-thread retry/fallback analogous to terminal intent. A failed post must retain pending ownership and be retried from a bounded production driver; timer/post failure must escalate to a defined fail-closed disposition. Add deterministic production-seam tests for failed first post then successful retry, persistent post failure, no-capture edge, post-release edge, shutdown cancellation, and resume/unlock racing the pending post. Prove hotkey ownership and lifecycle gates reach a terminal state exactly once.

### F2 — HIGH — AccessChanged plus failed CheckAccess fails open and can continue capture

**Location:** `main_windows.go:469-474`.

**Failure schedule:** the permission-change event is the observed revoke signal, so `accessChangedSignal=true`. The waiter calls `CapPermissionCheck`; a failed HRESULT logs `permission_status_query` and returns before it snapshots the active capture or calls `requestLifecycleStop`. If subsequent checks keep failing and the signed-hardware WASAPI fallback does not produce a deterministic terminal error, capture silently continues after an observed permission change. The event-specific failure record does not identify a lifecycle cleanup disposition or include a concrete next action. This is precisely the hardware uncertainty that the accepted bridge says must not be treated as acceptable continuation.

**Violated invariant/AC:** permission revoke must drive clean stop/discard/hotkey cleanup wherever observable, or produce a concrete blocked platform limitation and next action. Accepted Rev16 §P1 feasibility states that silent continued capture after permission revocation is unacceptable.

**Required correction:** treat a signaled AccessChanged event with indeterminate status as fail-closed for active capture—stop through the production lifecycle seam, prevent promotion, and log the ambiguity/next hardware action—or implement another equally safe bounded proof that permission remains allowed. Add a deterministic production-seam regression with `accessChangedSignal=true`, failed `PermissionCheck`, active capture, and no native WASAPI error; assert exactly one stop, non-promotable cleanup, hotkey cleanup, and no continued/new capture. Also cover transient query recovery without reopening before state is proven.

## Coverage assessment

`lifecycle_rework_test.go` exercises the portable tracker/coordinator abstractions, and `lifecycle_source_test.go` checks only `CapPermissionCheck` call-site ownership. A search of all probe tests found no PostMessage/postTransition failure schedule and no AccessChanged/CheckAccess-failure schedule. Thus the R2 suite remains green while F1 and F2 are present. These missing regressions are part of the required correction, not signed-hardware substitutes.

## Independent verification

Run from `pulsar-win` unless noted:

1. `go test -count=1 ./cmd/pulsar-win-probe`
   - PASS: `ok relux.works/duet/pulsar-win/cmd/pulsar-win-probe 0.365s`
2. `go test -race -count=1 ./cmd/pulsar-win-probe`
   - PASS: `ok relux.works/duet/pulsar-win/cmd/pulsar-win-probe 1.392s`
3. `go test -count=1 ./...`
   - PASS: all four packages; durations 2.861s, 0.739s, 2.354s, 1.447s.
4. `go test -race -count=1 ./...`
   - PASS: all four packages; durations 4.532s, 1.469s, 4.043s, 2.199s.
5. `go vet ./...`
   - PASS, no diagnostics.
6. `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...`
   - PASS, no diagnostics.
7. `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags '-s -w -buildid= -H windowsgui' -o /tmp/TASK-260712-2y74io-review-probe.exe ./cmd/pulsar-win-probe`
   - PASS; Windows GUI executable produced (2.9 MiB).
8. Per-package Windows test compilation:
   `for pkg in $(go list ./...); do GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c ... "$pkg"; done`
   - PASS; four Windows test executables produced for root, probe, winprobe, and wire packages.
9. `gofmt -d` over all six changed Go source/test files
   - PASS, empty diff.
10. `xmllint --noout probe-msix/AppxManifest.xml.in`
    - PASS.
11. Forbidden-capability search for `runFullTrust|broadFileSystemAccess|documentsLibrary|musicLibrary|picturesLibrary|videosLibrary|removableStorage`
    - PASS, no matches.
12. Required manifest marker search
    - PASS: `appContainer`, `packagedClassicApp`, and only the required `microphone` device capability are present.
13. `go test -count=10 -run 'TestR(1|2|3|4|5|6|7|8|9|10)' ./cmd/pulsar-win-probe`
    - PASS: `ok ... 0.614s`.
14. `staticcheck`, `pwsh`, and `cmake`
    - NOT RUN: none is installed on this macOS host.

No source, test, documentation, manifest, or board file was directly edited by this reviewer. Only board notes/status/resource mutations are made.

## Honest platform boundary

No signed MSIX, native Windows helper runtime, WACK, Windows race execution, real microphone, WTS lock delivery, power delivery, privacy revoke, or Windows 10/11 lifecycle run is available on this host. Those remain mandatory downstream hardware gates and cannot cure F1/F2, which are visible in current production control flow.

## Required handoff

Return to review only after F1/F2 are corrected as production ownership/state-machine behavior, deterministic regressions are added, all host/race/vet/Windows cross checks are fresh, and the signed-Windows gates remain explicitly unclaimed.