# Independent Phase 1 security and privacy review

## Description
Have a reviewer who implemented none of the reviewed tasks challenge the complete Phase 1 trust boundary before release.

## Scope
Review diffs, threat model and tests for bootstrap and capability auth, secret storage and redaction, invite and recovery replay, upload tokens and untrusted ffmpeg workers, target-snapshot ACL, direct-ID behavior, DND and block precedence, Telegram callbacks, history isolation, moderation operator auth and evidence access, rate limits, logs, screenshots and policy accuracy. Run or extend adversarial and fuzz tests without accepting implementation-agent self-report. Record severity, exploit path, evidence and owner for every finding.

## Acceptance Criteria
A reviewer independent of the implementation agents signs a source-linked report. All critical and high findings are fixed and re-reviewed; medium findings have explicit disposition. Negative tests cover every trust boundary, no secret/audio/tenant leak remains, and unresolved doubt blocks the Phase 1 root review.
