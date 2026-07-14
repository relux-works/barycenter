# macOS transmission client hooks outcome

## Result

Implemented the frozen macOS client-side clip lifecycle on exact code head `9622e00914195a5a17e4420cc1de5d8ce7a16921` in PR #24. This is automated engineering evidence only. It makes no claim about audible output, a packaged app on real hardware, or physical speaker timing.

## Capability boundary

The build advertises `media_clip_v1` plus the existing `seamless_adoption_v1`. It deliberately does not advertise `overlay_mix_v1` or `interrupt_resume_v1`. The client owns authenticated prepare, generation state, coordinator-clock timing, receipts and cleanup; exact overlay ducking and interrupt resume remain disabled until their dedicated mixer tasks supply those capabilities. Rogue or lost delivery capability fails with the frozen typed code and never falls back through legacy voice.

## Implemented

- Same-origin bearer download with URL-credential rejection, redirect refusal, absolute 34 MiB streamed cutoff, exact response/file size and SHA-256 verification, bounded timeouts and owned-temp cleanup.
- AVAudioFile open before ready, canonical ceil-duration equality, media expiry and exact prepare-deadline handling.
- Serial generation-safe preparing, ready, armed, playing, cancelling and terminal transitions; lower generations, duplicates, reordered play and cancel tombstones cannot start stale work.
- Existing ClockSync conversion, exact 100 ms late window, first-sample deadline disarm, and exactly-once started, ended, failed and cancelled receipts.
- Media work stays on async/control paths. CoordinatorClient send access is serialized and render callbacks receive no network, file or lifecycle work.
- Crash-safe node-local DND revisions and typed privacy-bounded presence persistence; reconnect replays the same idempotent DND revision.
- Existing `play_voice` and `solo_voice` protocol and PlayerCore routes remain separate and unchanged.

## Automated evidence

- Hosted CI run `29324579129` passed all four jobs on exact head: node-core Swift suite with 145 tests, coordinator vet/tests plus pinned previous-head rollback, Windows portable vet/tests/cross-build, and signed MSIX package-contract checks.
- Earlier implementation run `29324261074` also passed all four jobs before the final download-bound and test-lock hardening delta.
- Local gates: Swift build and test parser; coordinator vet, full test and race; pinned rollback; Windows vet, full test, race and amd64 cross-build; task-board validation. Local Swift tests cannot start under this host CommandLineTools because its existing `Testing` module is absent, so hosted macOS CI is authoritative.

## Downstream handoff

`TASK-260712-2bbz13` can mirror the lifecycle contract on Windows. `TASK-260712-31vvjt` consumes receipts and generation outcomes. Exact mixer implementation remains in `TASK-260712-1hqiek`, `TASK-260712-2zbmq4` and `TASK-260712-8mwyiv`. The separate manual epic `EPIC-260714-th54l3` retains all real-app and physical-hardware verification.