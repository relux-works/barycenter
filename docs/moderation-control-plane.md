# Moderation control plane

`TASK-260712-2kec2s` adds a separate, least-privilege moderation credential
domain and reuses the existing identity, recipient-block, media-lifecycle,
transmission-scheduler, and WebSocket hub services for enforcement.

## Operator credentials

Operator tokens have the form `mod_<64 lowercase hex characters>`. They are
stored only as SHA-256 digests and cannot pass the app/node bearer parser.
App or node tokens cannot pass the operator bearer parser.

Stop other coordinator writers before running either one-shot command:

```sh
duet-coordinator \
  --config /etc/duet/coordinator.yml \
  --provision-moderation-operator "Ivan Oparin" \
  --moderation-operator-scopes list,evidence,decide

duet-coordinator \
  --config /etc/duet/coordinator.yml \
  --revoke-moderation-operator op_XXXXXXXXXXXXXXXXXXXXXXXXXX
```

The provision command prints the plaintext token once. Store it in the
operator's credential manager. Available independent scopes are `list`,
`evidence`, and `decide`; grant only those needed.

## User API

- `POST /v1/reports` with an app control token creates one idempotent report
  per reporter/media pair. The media must be foreign and must still be
  accessible through the reporter installation's immutable accepted
  transmission target.
- `GET /v1/reports/{report_id}` returns only `received` or `reviewed` status
  plus the reporter's own report/media identifiers and timestamps.
- `POST /v1/reports/{report_id}/block` with `{}` delegates to the canonical
  recipient actor-block service and disarms only that sender's accepted work
  for the reporting node.

Reasons are `spam`, `harassment`, `illegal`, `sexual_content`, `violence`, and
`other`. Free-form details are limited to 2,000 UTF-8 bytes. The persistent
per-actor limit is 10 new reports per rolling hour; idempotent replays do not
consume the limit.

## Operator API

- `GET /v1/moderation/reports?status=open&limit=50` requires `list`.
- `GET /v1/moderation/reports/{report_id}/evidence` requires `evidence`.
- `POST /v1/moderation/reports/{report_id}/decision` requires `decide` and a
  body containing one of `no_action`, `delete_media`, `disable_actor`, or
  `disable_orbit`.

Evidence responses are attachment downloads. Each authorization is appended
to the moderation audit before the descriptor is opened. Evidence access
expires 30 days after report creation. Canonical cleanup is held until that
deadline even if the media is deleted or expires; free-form report details are
scrubbed when the same retention window closes.

Decisions are persisted as `pending` before enforcement and become `applied`
after the canonical service completes. Repeating the same action resumes or
returns the existing decision; a different action conflicts. This provides
crash-safe retries without duplicating policy implementations:

- `delete_media` uses `media_lifecycle_v1`: not-started work is cancelled,
  active audio uses `fade_stop`, and interrupted main playback resumes once;
- `disable_actor` and `disable_orbit` revoke canonical credentials, deny
  future media fetches, cancel source and target scheduler work, and close live
  sockets;
- recipient block uses the shared `blocks` policy and scheduler.

## Audit and logging

Audit rows are append-only at the SQLite trigger boundary. They record report
creation/rate rejection, operator provisioning/revocation, evidence reads,
block actions, and requested/applied decisions. Audit rows contain no audio,
plaintext credentials, free-form report text, storage keys, or local paths.
Normal request logs likewise omit those values.

