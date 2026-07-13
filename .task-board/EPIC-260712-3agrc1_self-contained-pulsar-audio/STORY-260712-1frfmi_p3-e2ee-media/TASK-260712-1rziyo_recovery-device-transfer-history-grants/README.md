# Implement recovery device-transfer and history-grant control flows

## Description
Coordinate explicit encrypted recovery packages without inventing server-readable escrow or promising impossible key restoration.

## Scope
Support current access bootstrap through an authorized surviving device, and optionally through a separately threat-modeled user-held recovery capability whose server copy is encrypted client-side. Issue one-time time-bound transfer packages, rotate away lost or cloned devices and require an authorized explicit history grant for named media or ranges. Define approval quorum, expiry, revoke, audit and device-verification UX data. If no valid recovery path exists, surface irreversible protected-history loss and preserve current unencrypted account recovery separately.

## Acceptance Criteria
A new or recovered device gets no historical protected key by default. Coordinator alone cannot recover or unwrap media; replayed, expired, revoked, foreign or cloned transfer packages fail. Lost-device rotation excludes the old device, successful grants reveal only approved scope and unrecoverable history is stated honestly rather than bypassed.
