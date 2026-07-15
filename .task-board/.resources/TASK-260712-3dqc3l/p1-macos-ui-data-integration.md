# P1 macOS routing, presence, history and failure integration

- Task: `TASK-260712-3dqc3l`
- Scope: best-effort macOS 14 code and deterministic automated verification
- Manual app/hardware evidence: `EPIC-260714-th54l3`

## Accepted integration boundary

The macOS shell now has three deliberately separate compositions:

| Composition | Authority | Explicit exclusion |
|---|---|---|
| `MacIdentityAppComposition` | self-service Create/Join, protected credential persistence, explicit one-time recovery export | media, playback and self-test |
| `MacCaptureAppComposition` | microphone/file intake, cues, disposable self-test and durable finalized local drafts | coordinator and HTTP work |
| `MacPhaseOneAppComposition` | authenticated upload, transmission, presence, history, receipts and allowed history actions | microphone ownership and self-test handles |

The split keeps local self-test usable accountlessly and makes the network
boundary auditable. `MacCaptureAppComposition` emits only a finalized
`user_recording` handle to the Phase 1 composition. The outbox rejects
`self_test` handles even if a caller tries to attach one directly.

## Identity and transport authority

Create and Join no longer use a bot link as their implementation. They call
the accepted self-service identity API through `OnboardingService`. Join
activates only after the returned bundle is stored in protected credential
storage. Create additionally retains its one-time recovery material in memory
and does not activate the new credentials until the user explicitly saves the
recovery payload and the protected recovery metadata records that
acknowledgement.

`PhaseOneAppClient` is constructed only from a canonical secure
`CredentialBundle` with an active orbit-bound control capability. All app API
requests use that origin and bearer; redirects, credential query parameters,
non-canonical identifiers, oversized responses and unexpected response media
types fail closed. The client binds the frozen endpoints:

- resumable `POST/PUT /v1/media/uploads`;
- resolved `POST /v1/transmissions`;
- `GET /v1/presence`;
- paginated `GET /v1/history`;
- allowed delete, replay and block-actor history actions; and
- explicit media deletion for an uploaded-but-unsent outbox item.

The shell presents only canonical human targets: This Pulsar, My Barycenter
and Current air. Opaque media, transmission, history and actor references are
kept inside typed actions and are never rendered as labels.

## Durable draft and retry contract

`PhaseOneDraftOutbox` persists a versioned owner-only JSON operation record
next to the capture store. A record freezes the selected route, requested
delivery and two deterministic actor-scoped idempotency keys before the first
network call.

1. A finalized user recording remains in `durable_unsent` across restart.
2. Upload failure leaves the WAV and operation retryable with the same upload
   key.
3. A completed upload response is persisted with `media_id` before local
   cleanup.
4. Only after that confirmation may the WAV be removed.
5. Transmission failure retains the server media reference and original
   transmission key; retry does not upload again and cannot change route or
   delivery underneath the idempotency key.
6. Explicit delete removes a local-only draft directly. For an uploaded draft
   it first deletes server media; a repeated `media_not_found` after a committed
   delete is treated as idempotent completion.

Requested and effective delivery remain distinct in both the outbox card and
history. A capability downgrade, protocol error or coordinator outage is
rendered as degraded/retryable state; no request is labelled sent merely
because it was attempted.

## Automated evidence

The focused Swift suites cover:

- exact authenticated upload/transmission/presence/history methods, headers
  and JSON values;
- redirect and missing-credential rejection;
- restart after confirmed upload plus failed transmission, with one upload and
  two calls using the same transmission key;
- frozen route/delivery on retry;
- explicit local deletion; and
- direct rejection of a disposable self-test handle by the network outbox.

The shell suite covers complete EN/RU copy, typed route/delivery labels,
outgoing state projection, identity recovery-required state and every stable
action seam. Release build compilation covers SwiftUI/AppKit/NodeCore wiring.

No live coordinator availability, real credential, microphone, audible
playback, packaged app, screenshot, accessibility traversal or hardware result
is claimed by this task. Those observations remain in the manual-test epic.

## Diagrams

- [macOS Phase 1 data components](../diagrams/p1-macos-ui-data-components.puml)
- [durable draft restart sequence](../diagrams/p1-macos-ui-data-restart-sequence.puml)
