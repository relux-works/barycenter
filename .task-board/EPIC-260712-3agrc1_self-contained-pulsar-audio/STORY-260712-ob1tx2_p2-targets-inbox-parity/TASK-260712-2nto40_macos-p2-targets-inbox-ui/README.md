# Implement macOS Phase 2 targets and inbox UI

## Description
Render the shared N-target, consent and inbox model in the macOS main window and menu bar with accessible, race-safe actions.

## Scope
Add active-Air and explicit permitted target selection, include-origin and track capability policy, versioned file-rights consent, paginated history and inbox, TTL and exact receipts, manual local or targeted replay, dismiss, delete, report and mute. Use opaque selectors and shared commands, preserve local drafts on network failure, show unsupported Phase 1 nodes before send, never autoplay an inbox item, and meet keyboard and VoiceOver requirements.

## Acceptance Criteria
macOS completes B5-B7 parity without displaying or constructing foreign targets. Consent, queue or replace, replay and moderation actions produce canonical outcomes; pagination and reconnect do not duplicate commands; every inbox playback requires an explicit current action; RU and EN keyboard and VoiceOver checks pass.
