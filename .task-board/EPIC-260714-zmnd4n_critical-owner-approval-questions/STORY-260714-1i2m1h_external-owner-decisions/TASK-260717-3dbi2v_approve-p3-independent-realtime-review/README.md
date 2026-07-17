# Approve Phase 3 independent realtime audio review

## Description
Obtain implementation-independent realtime audio approval for the exact Phase 3 root-reviewed candidate after the manual C1-C3 physical matrix is complete.

## Scope
Select a technically qualified reviewer who implemented none of the reviewed live PTT, capture, DSP, jitter, mixer or lifecycle paths. The reviewer names the exact root-reviewed commit, reruns representative repository checks, consumes signed C1-C3 real-app hardware artifacts, records findings and retests, and signs or rejects the candidate. Any affected code or fixture delta reopens root and domain review.

## Acceptance Criteria
An implementation-independent reviewer records identity, independence, exact commit and artifact hashes; C1-C3 physical evidence identifies Windows and macOS hardware and passes the frozen bounds; every Critical and High finding is fixed and independently re-reviewed; no stuck capture, lost release, old-generation capture, unbounded jitter or buffer, callback blocking or allocation, echo failure, limiter bypass or unsafe device-route lifecycle remains. Otherwise live_ptt and Phase 3 promotion remain blocked.
