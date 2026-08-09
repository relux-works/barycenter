# TASK-260727-1f2cyl: enable-air-authority-controlled-rollout

## Description
Inspect the deployed coordinator configuration and production health, prove a reversible rollback path, then enable authoritative Air rooms for controlled acceptance without exposing secrets or altering unrelated Barycenter state.

## Scope
Coordinator deployment and configuration only: record the exact pre-state, locate the authoritative Air feature flags and migration gates, validate backup and rollback mechanics, deploy the minimal configuration or code already accepted by BUG-260727-1hjfxi, verify health and targeted create-plus-invite behavior, and capture redacted evidence. Desktop UX and the cross-device journey are outside this task.

## Acceptance Criteria
A redacted receipt records exact before and after coordinator identity, version and Air capability state. The deployed health endpoint reports Air rooms enabled with authoritative ownership rather than shadow mode. A safe targeted probe can create an Air and initial single-use invite without revision-conflict or partial commit. Existing Barycenter state remains readable, logs show no migration or integrity errors, and an explicit rollback command and trigger are documented; any failed gate causes immediate rollback.
