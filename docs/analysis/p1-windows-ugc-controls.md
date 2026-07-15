# Phase 1 Windows UGC controls

## Surface inventory

The Phase 1 Windows shell has one authenticated remote-media surface:
`History`. Its selected history item is the only place where a user can inspect
or act on received or sent coordinator media. The tray contains navigation and
recording controls but does not render media. `Try locally` contains only local
self-test files, and the outgoing draft controls operate on the current user's
unsubmitted draft rather than accessible UGC.

Consequently, `History` is the required Phase 1 report, sender-block and
owner-delete surface. It projects coordinator `actions` and never derives
authorization from direction, sender names or opaque identifiers:

| Coordinator action | Windows control | User-visible result |
| --- | --- | --- |
| `report` | reason selector, optional details and Submit report | received or already received |
| `block_actor` | Block sender | blocked or already blocked |
| `delete` | Delete permanently | deleted and no longer replayable |
| `replay` | Replay | accepted or already accepted |

Controls not present in the selected item's action list are hidden. Thus a
foreign item can expose report and block without owner delete, while owned media
can expose delete. Active, expired or otherwise unsupported media inherits the
coordinator's authoritative action policy instead of duplicating it in Windows.

## Frozen report contract

Windows sends the canonical history report action with one of `spam`,
`harassment`, `illegal`, `sexual_content`, `violence` or `other`, plus optional
trimmed details of at most 2,000 bytes. EN and RU labels are local presentation;
the wire values remain stable. Reports and sender blocks preserve backend reuse
outcomes, so a repeated action is not presented as a new moderation event.

Opaque history, report, block and media identifiers stay below the composition
boundary. The shell receives display fields, allowed-action booleans and stable
outcome codes only. Rejections and transport failures become localized,
retryable, privacy-safe messages.

## Accessibility and verification boundary

The native implementation uses standard tab-stop Win32 buttons and an edit
control with an adjacent visible label. Dialog keyboard navigation remains
enabled through `IsDialogMessageW`; the action name, selected reason and details
label provide the accessible names without exposing raw IDs. Automated tests
cover action projection, own/foreign and active-media authorization, all frozen
reasons, EN/RU outcome copy, repeated actions, denial, offline retry messaging,
and the Windows cross-build.

Physical keyboard-only and screen-reader observations in a packaged Windows app
remain manual evidence under `EPIC-260714-th54l3`; automated coverage does not
claim those observations.
