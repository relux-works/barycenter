# Phase 1 shared delivery presentation model

- Date: 2026-07-14
- Task: `TASK-260712-1gx6mh`
- Contract: `p1-history-presence-telegram-v1`
- Implementation: `coordinator/internal/presentation`

The coordinator now owns one transport-neutral label model for HTTP read
models, Windows/macOS consumers and Telegram. A serialized label is always:

```json
{"key":"delivery.overlay","en":"Overlay","ru":"Поверх эфира"}
```

Clients select `en` or `ru`; they do not translate wire enums independently.
`key` is stable semantic identity. The English/Russian strings are golden copy
and may change only with an intentional golden digest update and review.

## Covered semantics

- sender, member and origin Barycenter names;
- direct target names, including multi-Pulsar Barycenters;
- This Pulsar, My Barycenter, Current Air, named pairwise Air and selected
  recipient audiences;
- include/exclude-origin copy;
- requested and effective overlay, interrupt and after-current delivery;
- automatic downgrade and sender-confirmed fallback notices;
- interrupt confirmation, fallback choices, expiry and too-late results;
- every accepted aggregate/media/target state and every frozen transmission
  reason code.

`PresentDelivery` returns requested and effective labels separately plus an
optional downgrade notice. A surface must never render a downgraded
after-current delivery as a successful overlay.

## Privacy-safe naming

Presentation inputs contain human metadata, not database or transport IDs.
Whitespace is normalized and visible values are bounded to 120 Unicode scalar
values. Exact numeric Telegram/database IDs, raw one-letter slots, `orbit:42`,
`42:a`, `a@42`, typed internal IDs and public object IDs are rejected as names
and replaced by stable copy:

| Missing/unsafe metadata | English | Russian |
| --- | --- | --- |
| sender | Unknown sender | Неизвестный отправитель |
| member | Unknown member | Неизвестный участник |
| origin | Unknown Barycenter | Неизвестный Барицентр |
| target | Unknown recipient | Неизвестный получатель |
| approach peer | Current Air | Текущий эфир |

A structured slot may be rendered only as part of a human target label such
as `«Home», Pulsar A`; the raw `a` or composite `42:a` is never output. The
legacy bot `/home`, `/status`, queue, voice target and provider-error helpers
now call this model and HTML-escape the resulting plain text at the transport
boundary.

## Receipt behavior

`ReceiptLabel(status, reason)` prefers an exact reason when one is authorized
for the viewer and otherwise renders the exact status. Unknown values return
`Status unavailable` / `Статус недоступен` and are never echoed. The caller
must omit a blocked reason before presentation when the viewer may not know
whether an actor or orbit block applied.

The catalog covers all constants in `internal/store/transmission.go`, including
offline timing, local/orbit DND, actor/orbit block, readiness, media integrity,
clock/device/audio graph, capability, cancellation, approach, moderation,
restart and expiry outcomes.

## Executable evidence

`coordinator/internal/presentation/presentation_test.go` provides:

- a SHA-256 golden over the sorted RU/EN static catalog and representative
  dynamic sender/origin/target/approach labels;
- an exhaustive inventory assertion for all transmission reason constants;
- direct and linked target/audience fixtures;
- requested/effective downgrade and interrupt-confirmation fixtures;
- raw database, Telegram, node and composite identifier leak canaries; and
- a transport-wording/duplicate-key guard.

The next history, presence and Telegram tasks consume this package rather than
adding local dictionaries or exposing raw identifiers.
