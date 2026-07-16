## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:12:52Z

## Last Update
2026-07-16T01:38:53Z

## Blocked By
- TASK-260712-2j5fkr
- TASK-260712-2zoy4u
- TASK-260712-1c34fe
- TASK-260712-2ctf3x
- TASK-260712-1gx6mh

## Blocks
- TASK-260712-1vklop
- TASK-260712-cuplon
- TASK-260712-2nto40
- TASK-260712-1q2kwa

## Checklist
- [x] Add current Air own and explicit target selection to routing surfaces
- [x] Show inbox and history with receipt pagination and partial delivery state
- [x] Add manual replay delete report and mute actions with policy gating
- [x] Keep non target media invisible and never late autoplay missed items
- [x] Expose only server-authorized opaque targets and action capabilities
- [x] Provide equivalent localized targeted-track, consent, inbox and receipt semantics

## Notes
2026-07-16 strict-sequence start after TASK-260712-2zoy4u acceptance and tracking merge 8a2defa. Implementing inline outside task-board spawn workflow. Auditing the shared macOS and Windows presentation/model seams against the frozen target, inbox, history, replay, consent and moderation contracts; no platform-specific final view or real-app/hardware claim belongs to this task.
2026-07-16 accepted after root inline review. The shared additive coordinator presentation projection and fail-closed Swift/Go models cover current Air, own and explicit routing, opaque target expiry/capabilities, inbox/history/receipt pagination, current-policy replay, dismiss/delete/report/mute actions, EN/RU semantics, stale/offline/error authority removal and an explicit no-late-autoplay boundary. Inbox moderation resolves only through the server-returned history_item_id. Branch was fetched and rebased onto origin/main; merge-base --is-ancestor origin/main HEAD passed before handoff. Code PR #132 head 5968046, hosted CI 29464453352 4/4 green, merged as 45f27ac. Local full coordinator only lacked libvorbis for two unchanged OGG fixture generators; hosted coordinator with ffmpeg passed them. Manual real-app/hardware evidence remains solely in EPIC-260714-th54l3 and is not claimed here.

## Precondition Resources
(none)

## Outcome Resources
- [p2-targets-inbox-parity-components.puml](file://TASK-260712-2vipy3/p2-targets-inbox-parity-components.puml) — Pulsar surface context for inbox history and explicit targets
- [p2-pulsar-targets-inbox-presentation-model.md](file://TASK-260712-2vipy3/p2-pulsar-targets-inbox-presentation-model.md) — Shared coordinator Windows and macOS presentation and command handoff
- [pulsar-targets-inbox-presentation-v1.json](file://TASK-260712-2vipy3/pulsar-targets-inbox-presentation-v1.json) — Frozen portable audience target inbox history and no-autoplay contract
- [acceptance-provenance.md](file://TASK-260712-2vipy3/acceptance-provenance.md) — Accepted code CI and merge provenance
