# Threat-model and freeze soundboard and automation semantics

## Description
Resolve the optional automation surface and every safety boundary before persistence or API work.

## Scope
Choose the first supported entry point from loopback local API, authenticated network API or webhook and document bind address, TLS or origin, CSRF where relevant and threat model. Freeze cue-class media only, manual soundboard versus scheduled or API feature flags, actor and least-privilege token scopes, one-time secret display, target selectors, DND and block precedence, IANA timezone and DST behavior, missed or duplicate fire semantics, crash and clock-jump handling, rate and concurrency caps, quick disable, audit vocabulary and mixed-version downgrade. Automation can never start a microphone, invent a target, bypass policy or silently play a missed event later.

## Acceptance Criteria
A reviewed contract chooses an exact initial surface and feature flags and closes network exposure, authentication, scheduling and media-scope ambiguity. It defines at-most-once execution identity, skipped and repeated DST behavior, no-catch-up defaults, revoke race semantics, deny reasons and no-microphone invariants. Downstream work has no product or security choice left open.
