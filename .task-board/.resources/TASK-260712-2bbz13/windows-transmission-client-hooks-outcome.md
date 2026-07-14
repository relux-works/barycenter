# Windows transmission client hooks — outcome

Accepted engineering head: 219306ceda548b64a6bb72e279c9ac9da4e65313
PR: https://github.com/relux-works/barycenter/pull/25
Final exact-code hosted run: https://github.com/relux-works/barycenter/actions/runs/29326895259

## Delivered

- Added a generation-safe media_clip_v1 lifecycle for prepare, authenticated same-origin download, exact size and SHA-256 verification, bounded WAV decode, coordinator-clock scheduling, play, cancel and exact-once terminal receipts.
- Kept network I/O, decode, persistence and scheduling away from the WebSocket read path and render callback boundary.
- Added durable local DND revision/state and a privacy-bounded presence projection with atomic Windows replacement.
- Preserved legacy play_voice and solo_voice behavior and sanitized externally visible media failures.
- Added mixer-owned overlay/interrupt routing seams. Production advertisement of overlay_mix_v1 and interrupt_resume_v1 remains intentionally withheld until their dedicated implementation tasks.
- Added deterministic unit, integration, race, ordering, cancellation, deadline, redaction, persistence and fetch-policy coverage.

## Verification

Local gates passed:

- cd pulsar-win && go vet ./...
- cd pulsar-win && go test ./...
- cd pulsar-win && go test -race ./...
- repeated lifecycle/race stress suites
- Windows amd64 and arm64 cross-builds
- Windows test-binary compile
- coordinator vet, full test, race and pinned rollback compatibility
- node-app Swift build
- git diff --check

Hosted exact-code run 29326895259 passed all four jobs: pulsar-win, pulsar-win-packaged-probe, coordinator and node-core. The first hosted attempt exposed a test-only POSIX permission assertion on Windows; commit 219306c made the assertion platform-correct without weakening runtime persistence behavior.

## Deliberate boundary

This outcome proves compilation, deterministic behavior and signed-package CI construction only. It does not claim installation, audible playback, timing quality, Windows 10/11 compatibility or any other real-app/physical-hardware result. Those checks remain routed to manual epic EPIC-260714-th54l3. Overlay/duck/limiter and interrupt/resume delivery capabilities remain gated for their later strict-sequence tasks.