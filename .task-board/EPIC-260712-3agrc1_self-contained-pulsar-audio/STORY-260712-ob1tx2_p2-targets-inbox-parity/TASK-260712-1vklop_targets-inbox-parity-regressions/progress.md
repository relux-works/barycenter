## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:12:52Z

## Last Update
2026-07-16T03:59:50Z

## Blocked By
- TASK-260712-2bk0vy
- TASK-260712-1c34fe
- TASK-260712-2j5fkr
- TASK-260712-2zoy4u
- TASK-260712-2vipy3
- TASK-260712-2ctf3x
- TASK-260712-cuplon
- TASK-260712-2nto40

## Blocks
- TASK-260712-20cuna
- TASK-260712-21kz3b
- TASK-260712-3u5cdn
- TASK-260712-3qybi2
- TASK-260712-1fpb9q
- TASK-260712-qi81vf
- TASK-260712-n11rg6

## Checklist
- [x] Cover non target API and media invisibility
- [x] Cover N recipient personal delivery and no broadcast fallback
- [x] Cover inbox replay delete TTL and no late autoplay rules
- [x] Cover mixed version unsupported target visibility and rights revocation
- [x] Map automated and manual evidence to B5 through B7
- [x] Test targeted tracks, opaque target forgery, cursor isolation and new-member old-item denial
- [x] Test consent version, canonical moderation revocation and all platform or Telegram parity

## Notes
2026-07-16 strict-sequence start after TASK-260712-cuplon code PR #136 merge 15f675e and tracking PR #137 merge 1d49243; hosted runs 29468731725 and 29469062833 accepted. Implementing inline outside task-board spawn workflow. Scope is adversarial automated parity fixtures and an explicit B5-B7 evidence map; all real-app, physical-hardware, Narrator/VoiceOver, audible and mixed-fleet hands-on proof remains in EPIC-260714-th54l3 and will be named, not claimed.
2026-07-16 accepted on exact engineering head 1b15cafbabd7543e5a7ee4d96af977d4abb1b994 through PR #138, merge 029346cef66e1db66238cee290086aca1fab97ce, after hosted run 29470131117 passed coordinator, node-core, pulsar-win and signed packaged-probe. The local all-suite acceptance manifest local-task-260712-1vklop-final passed all 12 commands, including previous-head rollback, Windows race/cross-build and 232 Swift tests. Nineteen repository invariants cover B5-B7 ACL, immutable N-recipient snapshots, TTL/manual replay/no-autoplay, cursor isolation, consent, canonical moderation revocation, mixed-version fail-closed behavior and one Windows/macOS/Telegram fixture. Real packaged UI, physical hardware, accessibility readers, audible playback, real-network denial and mixed-fleet proof remain manual-required in TASK-260712-3u5cdn under EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
- [p2-targets-inbox-parity-sequence.puml](file://TASK-260712-1vklop/p2-targets-inbox-parity-sequence.puml) — Regression coverage reference for explicit target miss and replay flow
- [P2 targets/inbox parity regression evidence](../../../../docs/analysis/p2-targets-inbox-parity-regression-evidence.md) — Executable B5-B7 evidence map and explicit manual boundary
- [PR #138](https://github.com/relux-works/barycenter/pull/138) — Accepted engineering change and hosted CI provenance
