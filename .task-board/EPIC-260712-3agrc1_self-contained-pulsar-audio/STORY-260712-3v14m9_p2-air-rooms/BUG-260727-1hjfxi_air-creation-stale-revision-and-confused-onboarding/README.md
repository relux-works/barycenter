# BUG-260727-1hjfxi: air-creation-stale-revision-and-confused-onboarding

## Description
Creating an Air from the macOS app shows a spinner and then a persistent changed-elsewhere error. The preceding identity bootstrap is also labeled Create an air even though it creates a Barycenter/Orbit, so users cannot form a correct mental model of the workflow.

## Scope
Diagnose the create-Air stale-revision failure end to end and define a user-friendly product architecture for Barycenter identity, saved Air membership, active playback, invite and recovery flows. Preserve the existing security model while removing duplicate terminology and unnecessary user-visible ceremony.

## Acceptance Criteria
Root cause is supported by client/server/log evidence. The proposed information architecture gives every user-visible object one name and one responsibility. First-device setup, Air creation, cross-device join, recovery and active-playback flows are specified with error/retry semantics. Implementation work is decomposable into verifiable follow-ups and no source change begins before approval.
