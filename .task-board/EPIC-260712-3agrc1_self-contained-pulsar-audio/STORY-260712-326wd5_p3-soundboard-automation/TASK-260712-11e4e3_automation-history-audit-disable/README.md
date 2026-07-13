# Extend history audit and emergency automation controls

## Description
Expose actor-scoped attribution and fast safe control actions without forking earlier history or moderation services.

## Scope
Extend canonical history with manual cue, schedule and API trigger source, actor or principal fingerprint, schedule and execution IDs, cue version, target summary, scheduled, accepted and terminal times, exact skipped, denied, rate-limited and cancelled reasons. Add authorized pending cancel, schedule disable, principal revoke and orbit emergency-disable commands with immutable audit. Never return bearer secrets or allow history reads to infer foreign cues, schedules, targets or actors.

## Acceptance Criteria
Every trigger attempt is attributable and its one canonical execution state reconciles with transmissions and receipts. Authorized emergency actions take effect within the contract bound and are audited; foreign scope probes are indistinguishable from missing resources. History never exposes token material, audio content or private filenames and uses the shared delete and moderation state.
