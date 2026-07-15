# Phase 1 macOS UGC controls

## Surface inventory

The macOS Phase 1 app has one authenticated remote-media surface: the History
list in the main window. The status menu navigates and controls recording but
does not render media. Try Locally contains device-local self-test files, and
outgoing draft deletion is not an action on accessible remote UGC.

Each History row projects the coordinator's canonical `actions` list. It does
not infer authorization from direction, sender display name or an opaque ID:

| Coordinator action | macOS control | Result shown |
| --- | --- | --- |
| `report` | reason picker, optional details and Submit report | received or already received |
| `block_actor` | Block sender with confirmation | blocked or already blocked |
| `delete` | Delete permanently with confirmation | deleted and no longer replayable |
| `replay` | Replay | accepted or already accepted |

Unsupported actions are absent from the row. The composition checks the current
row's allowed actions again before calling NodeCore, so a stale or fabricated UI
request cannot turn a hidden control into backend access. Active, expired and
otherwise unavailable media follow the coordinator policy rather than a second
macOS policy implementation.

## Report and result contract

The frozen reasons are `spam`, `harassment`, `illegal`, `sexual_content`,
`violence` and `other`. Optional details are trimmed, single-line and bounded to
2,000 UTF-8 bytes before the authenticated history action is sent. NodeCore
validates delete, report and block responses and projects only stable outcome
codes; report and block reuse flags remain visible as “already” outcomes.
Opaque history, media, report and block IDs never enter user-facing result or
error copy.

## Accessibility and verification boundary

The SwiftUI row uses native buttons, a labelled Picker and a labelled TextField.
Delete and block use visible confirmation dialogs with destructive semantics for
delete. Result and retry banners combine icon and localized text for VoiceOver,
and action availability does not depend on color.

Automated tests cover the six reasons, EN/RU copy, own/foreign action projection,
stale-action denial, optional-detail bounds, repeated backend actions, offline
messages, confirmation/accessibility source seams and the full Xcode build/test
path. Physical keyboard and VoiceOver observation in a packaged app remains
manual evidence in `TASK-260712-e5mfqj` under `EPIC-260714-th54l3`.
