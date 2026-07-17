# Approve Phase 3 independent automation safety review

## Description
Obtain implementation-independent automation safety approval for the exact Phase 3 root-reviewed candidate after the manual C7 real-app matrix is complete.

## Scope
Select a technically qualified reviewer who implemented none of the reviewed cue, schedule, principal, runtime, Telegram or client administration paths. The reviewer names the exact root-reviewed commit, reruns representative adversarial checks, consumes signed C7 real-app artifacts, records findings and retests, and signs or rejects the candidate. Any affected code, schedule contract, fixture or runtime-config delta reopens root and domain review.

## Acceptance Criteria
An implementation-independent reviewer records identity, independence, exact commit and artifact hashes; C7 physical evidence covers Windows, macOS and Telegram surfaces; every Critical and High finding is fixed and independently re-reviewed; no authorization or target-policy bypass, secret exposure, duplicate fire, restart or clock catch-up, unbounded queue, revoke or emergency-stop escape, DND violation or microphone activation remains. Otherwise automation activation and Phase 3 promotion remain blocked.
