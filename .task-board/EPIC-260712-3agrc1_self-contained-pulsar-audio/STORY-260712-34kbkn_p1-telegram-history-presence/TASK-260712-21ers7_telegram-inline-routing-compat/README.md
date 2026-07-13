# Implement Telegram routing actions without breaking default voice

## Description
Connect secure Telegram actions to common transmission semantics while preserving the zero-action legacy queue path and every race outcome.

## Scope
When a legacy voice becomes ready, immediately create its default after_current entry using the original trusted intake ordering and existing personal or broadcast policy, then display available audience and overlay, interrupt or after-current actions. An explicit callback before playback atomically cancels the pending default and creates the selected transmission at callback acceptance time; a race after start returns too_late without a duplicate. Non-voice clips follow the frozen explicit-action policy. Reuse common presentation and status models. Apply visible whole-transmission overlay downgrade, and require the second explicit confirmation before any interrupt fallback.

## Acceptance Criteria
Taking no action preserves current first-after-current FIFO and latency. Any accepted callback produces exactly one effective delivery with the same authorization, target snapshot, DND, block and receipt semantics as the app; retry or callback races cannot duplicate playback. Unsupported interrupt never silently falls back. Pairwise-approach and target labels remain human and no raw IDs leak.
