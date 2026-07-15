# P2 versioned content-policy consent

Task: `TASK-260712-2ctf3x`
Contract: `p2-content-policy-consent.v1`

## Frozen authority

- The coordinator owns current version `1.0`, binary policy hash `a4d59ec7e9bfd8aeb2ec5d84356517580bde8df4540e6a2162f9206cd7ecd30e`, effective time, and server acceptance time.
- Localized source hashes are `a25d1b46b530fb64f18224618701f67ed80ace9ce5c1b1cfb1a7c3d70a1988ca` for EN and `4726df3e447674a5d6a87a34a5f05363c53604a80da3c1d6e69a3f9c05f41082` for RU. English controls conflicts.
- Terms and Content Guidelines are `https://barycenter.live/legal/terms` and `https://barycenter.live/legal/content-guidelines`.
- A durable grant records actor, orbit, version, hash, locale, transport, server acceptance time, revoke time and revision. It stores no content, filename or raw transport identifier.
- Terms acceptance is not a content-ownership claim. Every new app file upload separately requires `rights_acknowledged=true`; the server does not persist that reminder as proof of ownership or legal validity.

## Routes and enforcement

| Operation | Contract |
|---|---|
| Display | `GET /v1/content-policy?locale=en|ru` |
| Read current grant | `GET /v1/content-policy/acceptance` |
| Accept exact displayed version/hash | `PUT /v1/content-policy/acceptance` |
| Revoke | `DELETE /v1/content-policy/acceptance?locale=en|ru` |
| Missing, revoked or stale grant | `428 content_policy_acceptance_required` |

New user-media upload, transmission creation and manual replay fail closed on the current actor grant. Existing accepted media retains its prior retention and moderation lifecycle. An idempotent replay of an already accepted request returns its frozen result before evaluating a new grant, while a new replay action is reauthorized.

Telegram `audio` and `document` events are checked before canonical media acceptance or download. `/content_policy [ru|en]` displays the same version, hashes, copy and links; `/accept_content_policy [ru|en]` records a Telegram ActorContext grant. The frozen legacy Telegram voice path remains unchanged.

macOS and Windows clients expose display/current/accept/revoke operations, validate the exact response contract, never trust a client timestamp, and reject upload calls without an explicit per-upload rights acknowledgement. The macOS outgoing row has a separate rights toggle and presents current server policy when a fresh grant is required. Windows presents a native Terms-and-rights confirmation and accepts the server manifest only when the current grant is absent.

## Evidence

- Store tests cover RU/EN manifests, control and Telegram authorization, node denial, actor mismatch, revoke/reaccept revisions, stale hash, mutation rate limit, metadata minimization, upload/transmission gates and unchanged old media.
- HTTP tests cover display-before-accept flow, exact server time, current-grant read, revoke, `428`, and separate upload rights acknowledgement.
- Telegram tests prove audio/document never reaches ingest before the grant, legacy voice remains accepted, and revoke restores the pre-download gate.
- macOS and Windows client tests cover exact routes, hash propagation, grant lifecycle, URL validation, and local rejection of a missing per-upload acknowledgement.
- `scripts/validate_targets_inbox_contract.py` fail-closes the machine-readable policy invariants in `acceptance/targets-inbox-contract-v1.json`.
