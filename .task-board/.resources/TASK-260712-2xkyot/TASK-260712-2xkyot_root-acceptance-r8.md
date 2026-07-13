# TASK-260712-2xkyot root acceptance R8

Date: 2026-07-13  
Reviewer: root orchestrator  
Verdict: **ACCEPTED**

This acceptance supersedes producer/reviewer status changes as the root decision. I reviewed the R6 implementation and adversarial tests directly, required the independent R7 security/compatibility pass, verified the frozen boundary again after that review, and reran the final checks below. No release blocker remains in the Telegram migration/link scope.

## Accepted boundary

- `coordinator/internal/bot/bot.go` — `96d295381a10197506eee4bf0d99adb7f0a9ecbf04bc3abb596e929f33fa5b04`
- `coordinator/internal/bot/bot_test.go` — `96638935ed384bd6ff99a776bcd6b505eb39a96b0aafaeabb3625355411db04b`
- `coordinator/cmd/duet-coordinator/telegram_identity_test.go` — `175c65f22c92649d27140964f911b1b7deb9621a2e1361301b66ccba8481b1ac`
- `coordinator/internal/store/identity_telegram.go` — `1d99a568881d5bc22b53166a9d76cd04d6bae10ef59a53c27d39d5b1dab72451`
- `coordinator/internal/store/identity.go` — `840eea9ca9222e2077b363599b173ea2f6060e752fcfaa8a0f4361536fd38134`
- `coordinator/internal/store/identity_schema.go` — `6c28dd5fbcfea56357584a4c033ed9f13c8ab1875b50f69c70c072c17937308f`
- `coordinator/internal/store/onboarding.go` — `77f30536883a5798274e0b001bde7299f37bea5f6e64b0d402f1362cc1bba0f9`
- `coordinator/internal/store/security_audit.go` — `194c04fc7861b9521b98d42d7e84e1a517807445e7974b5bcc073a498f821faa`
- `coordinator/cmd/duet-coordinator/loop.go` — `90940d1252d9a44b6174bb7482b8a71aed522c450022321c02003e3c3f6137c1`
- R6 producer outcome — `9a04b44784201d11ad688ae624f3202343946d16ec37746bf59a0c2205c5cd16`
- R7 independent review — `db4d2d4fc522e4f35c1cc47a3f12faacffd24869f6c49ebf92a1f5b33983dc34`

## Root review conclusions

- Telegram consume remains a trusted in-process Bot boundary; no unauthenticated HTTP consume route exists.
- The rolling limiter reserves before database work, rejected attempts advance the window, and every emitted rate-limit response requires a durable typed security audit. Audit failure becomes a generic structural failure without refunding the attempt.
- The fixed-shape credential gate, dummy digest, constant-time comparison, `BEGIN IMMEDIATE` writer serialization, single-use code update, actor/membership/legacy writes, and audit commit preserve one-winner semantics and rollback compatibility.
- Linking creates Telegram actor/membership state without transferring app-installation credential or slot ownership. Revoked, left, disabled, migrated, and mixed legacy/self-service role paths remain explicitly covered.
- Outbox overflow, send/delete failures, redirects, Telegram response descriptions, token-bearing URLs, form bodies, file identifiers, paths, joined/wrapped errors, chat IDs, message IDs, user IDs, display names, and link codes cross only constant/sanitized logging boundaries.
- The real `Bot.Run -> loop -> Store` tests prove durable-audit success/failure, committed consume despite asynchronous delete/send failure, and exact outbox saturation without identity rollback or canary leakage.

## Final root verification

All commands exited zero:

- focused bot privacy suite, `-count=20`;
- real-bot identity/privacy suite under `-race`, `-count=10`;
- pinned previous-head Store compatibility;
- full `go test -race -count=1 ./...`;
- `go vet ./...` and previous-head vet;
- `go build ./...`;
- NUL-safe full coordinator `gofmt -l` scan (no output);
- `git diff --check`;
- `task-board validate`.

Live Telegram, external CI, and a production database were not exercised. Those are deployment gates, not defects in this accepted implementation boundary. The combined dirty worktree was preserved; no commit or push was made.
