# Implement durable saved-cue media lifecycle

## Description
Make user and builtin cues durable without bypassing canonical ingest storage rights moderation quota or deletion.

## Scope
Represent a saved cue as an owner-scoped versioned reference to an eligible ready cue-class MediaItem or a hash-pinned builtin asset. Reuse signature probing, normalization, target ACL, content-policy consent and report or disable services. Define pin or reference-count retention distinct from ordinary seven-day clips, per-orbit cue count and byte quotas, dedupe, rename ordering and delete semantics. Deleting or disabling source media, actor or orbit must revoke future triggers and safely cancel eligible pending events; active playback follows the canonical transmission policy.

## Acceptance Criteria
Saved cues survive ordinary clip cleanup only through explicit accounted pinning, cannot outlive authorization or moderation disable and cannot reference foreign, corrupt, unready or oversized media. Quota and accounting reconcile after create, replace, delete, failure and crash. Builtin asset versions are hash-stable and no parallel upload, ACL or report path is introduced.
