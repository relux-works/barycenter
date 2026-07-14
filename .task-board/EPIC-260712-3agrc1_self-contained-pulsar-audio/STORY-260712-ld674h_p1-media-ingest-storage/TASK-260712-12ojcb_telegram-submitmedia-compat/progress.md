## Status
reviewing

## Assigned To
codex-inline

## Created
2026-07-12T15:28:12Z

## Last Update
2026-07-14T05:01:38Z

## Blocked By
- TASK-260712-2af2dp
- TASK-260712-2xkyot

## Blocks
- TASK-260712-3huupe
- TASK-260712-21ers7

## Checklist
- [x] Replace the bespoke Telegram voice pipeline with a SubmitMedia adapter
- [x] Preserve legacy FIFO ordering, target defaults and compatibility WAV playback
- [x] Add regression coverage for ordering and bot reply parity on the default voice path

## Notes
Strict sequential inline execution started 2026-07-14 from clean main merge 0f3148a379258b9af1934d3d6e582e7998c40f59 on branch task/task-260712-12ojcb-telegram-submitmedia-compat. Scope is the Telegram transport adapter over common SubmitMedia with default FIFO, target/reply and legacy WAV parity; inline actions/history/presence and manual app/hardware evidence remain out of scope.
Implementation review checkpoint 2026-07-14: Telegram acceptance atomically creates common source=telegram and legacy rows under one media ID; raw bytes enter the shared SubmitMedia instance; ready output maps back to FIFO legacy play_voice and authenticated /media WAV. Common failure codes are persisted, exact accepted/ready/failure replies and personal/broadcast behavior remain covered. Local go vet, full go test, full race, 20x focused stress, exact previous-head media-processing rollback and linux amd64 CGO-free build are green. No manual real-app or hardware evidence is claimed.
Code commit 908f89a2ebe96e5ac5e32c2979e743bb167a8b9e is locally accepted for hosted review. Production bot download is capped at 20 MiB plus one detection byte before SubmitMedia; retained failure bytes are private and their deletion state remains retryable until physical retention cleanup succeeds. Hosted CI and merge remain pending.

## Precondition Resources
(none)

## Outcome Resources
- [p1-media-ingest-sequence.puml](file://TASK-260712-12ojcb/p1-media-ingest-sequence.puml) — Telegram adapter path through common SubmitMedia service
