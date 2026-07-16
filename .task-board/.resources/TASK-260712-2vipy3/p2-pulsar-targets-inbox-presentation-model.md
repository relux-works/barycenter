# Phase 2 Pulsar targets, inbox and history presentation model

- Date: 2026-07-16
- Task: `TASK-260712-2vipy3`
- Contract: `pulsar.targets-inbox-presentation.v1`
- Server implementation: `coordinator/internal/presentation`
- macOS model: `node-app/Sources/NodeAppUI/PulsarTargetsInboxModel.swift`
- Windows model: `pulsar-win/targets_inbox_model.go`

This task supplies the transport-neutral model and command boundary consumed by
the later native macOS and Windows views. It does not claim that either final
view is implemented or tested on real hardware.

## One model, server-owned authority

The coordinator adds the same `presentation` object to explicit target,
inbox, history and paginated receipt projections. Every label has stable
semantic identity plus exact English and Russian copy:

```json
{"key":"inbox.availability.available","en":"Available","ru":"Доступно"}
```

Clients choose `en` or `ru`; they do not translate wire enums. Dynamic sender,
source and target labels still pass through the existing bounded privacy-safe
presentation helpers. Internal report state remains redacted from sender
receipts.

The action array remains the server-returned capability list. The localized
label is presentation only. Swift, Go and the coordinator all refuse to build
a mutation unless the action is present in the latest `ready` model. The
coordinator still reauthorizes the actor, current binding, content policy,
media and action on every request, so presentation hints never become an
authorization source.

## Explicit targets and capabilities

`GET /v1/transmission-targets` remains additive and returns only opaque `trf_`
references. Each option now also contains:

- expiration time;
- `known`, `mixed` or `unknown` capability state;
- the sorted capability intersection of currently connected exact targets;
- a privacy-safe localized target label.

Offline capability state is deliberately `unknown`, not optimistic. A
Barycenter with only some online Pulsars is `mixed`. These values are advisory:
send-time mixed-version validation remains authoritative and can still return
the existing whole-request `422 unsupported_targets` response.

The platform models drop expired and duplicate references, remove selections
missing from a replacement model and cap explicit selection at 64. The same
fail-closed command seam permits only a current server-derived `this_pulsar`,
`own_barycenter`, `current_air` or `explicit` audience; explicit routing also
requires a current non-empty opaque selection. A caller cannot construct a raw
actor, orbit, slot or binding target.

## Inbox, history and receipts

The shared model represents:

- inbox availability and TTL separately from the missed receipt;
- requested and effective delivery separately;
- paginated inbox, history and per-target receipt chains;
- partial delivery counts and safe target labels;
- replay, dismiss, delete, report and `block_actor` (shown as “Mute sender”)
  only when the server returned the capability; inbox report and mute resolve
  only through that row's server-returned `history_item_id` capability;
- current, required or stale content-policy state; and
- the current Air title and targeted-track policy without accepting Air or
  identity IDs from a view.

Opaque cursors are current page-chain capabilities. Clients neither parse nor
log them. A `410 cursor_expired` result discards the chain and refreshes from
the first page.

## Honest freshness and no late autoplay

The five surface states are `loading`, `ready`, `stale`, `offline` and
`coordinator_error`. Retained rows may remain visible while stale or offline,
but every mutation and pagination command is disabled until a fresh `ready`
replacement arrives; only refresh remains available.

There is intentionally no autoplay command. Reading or paginating an inbox
cannot start playback, and every missed item requires an explicit replay
command after current policy and action revalidation.

## Executable evidence

- `scripts/validate_pulsar_targets_inbox_presentation.py` freezes the command,
  authority, localization and no-autoplay boundary.
- coordinator tests cover the EN/RU golden catalog, fail-closed unknown enums,
  privacy-safe HTTP projections and advisory capability states.
- `PulsarTargetsInboxModelTests` and `targets_inbox_model_test.go` read the same
  protocol contract, prune expired authority, reject manufactured commands,
  redact opaque descriptions and exercise every command seam.

The native view/layout work remains strictly in `TASK-260712-2nto40` and
`TASK-260712-cuplon`.
