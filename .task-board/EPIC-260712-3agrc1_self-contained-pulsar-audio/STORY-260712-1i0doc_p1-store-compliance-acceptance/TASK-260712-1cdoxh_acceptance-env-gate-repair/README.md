# Repair and freeze the Phase 1 acceptance environment

## Description
Create one reproducible, provenance-tracked harness for A1-A8, realtime gates, migrations and Store evidence.

## Scope
Pin and document the exact Go, Xcode and Swift toolchains, including repair of the current node-app swift test failure caused by the unavailable Testing module. Provide clean Windows 10 and 11 install images or machines, two-Pulsar topology, optional internal Spotify fixtures, deterministic coordinator data, Store-package identity, WACK execution, migration fixtures from the shipped schema and a previous-version rollback rehearsal. Define sanitized artifact directories, build hashes, clock-sync and p95 sampling methodology, memory measurement for the maximum clip, screenshot capture and pass or fail templates. Separate unavailable real hardware or Partner Center access from repository setup debt.

## Acceptance Criteria
A fresh operator can run coordinator and Windows tests, Windows cross-build, Swift tests on the pinned toolchain, all golden and migration suites, WACK and every automated acceptance fixture using documented commands. The Swift baseline is green rather than waived. Live prerequisites, sample counts, measurement methods, build provenance and sanitized artifact locations are explicit, and prior-version rollback can be rehearsed without destructive production data.
