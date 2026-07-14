# Execute final C7 soundboard and automation acceptance

## Description
Run C7 on the root-reviewed and independently automation-reviewed build.

## Scope
Test manual saved cues separately from automation; IANA timezone and DST skip or repeat, forward and backward clock jumps, duplicate API and scheduler events, restart claim points, quiet hours, DND, block, Air leave, target ACL, media disable, local recipient ceiling, secret revoke, stale Telegram callbacks, rate and concurrency caps, queue bounds, emergency disable and no microphone. Cover Windows, macOS and Telegram and all enabled entry points.

## Acceptance Criteria
C7 passes with at-most-once and no-catch-up evidence, immediate revoke and kill-switch bounds, attributable outcomes and no policy or capture bypass. Manual soundboard behavior remains correct when automation is disabled; every failed environment or unshipped entry point is named rather than assumed.
