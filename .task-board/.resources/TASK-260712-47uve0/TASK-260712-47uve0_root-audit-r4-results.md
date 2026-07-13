# TASK-260712-47uve0 — root audit R4 results

Date: 2026-07-14 (Asia/Tbilisi)
Task: Windows DPAPI onboarding credential client
Base: `976cef3add17e59714bb43f5a0e3a5cf6fe06cfa`

## Disposition

Accepted as the Windows onboarding **code checkpoint**. The original task AC
is implemented, the R3 findings are closed on the final bytes, and host plus
Windows cross-build gates are green. This does not claim independent review or
native Windows runtime evidence.

The next strict-sequence tasks own the remaining native proof:

- `TASK-260712-38qsku` — installed migration and rollback verification;
- `TASK-260712-13rbnw` — signed MSIX probe packaging;
- `TASK-260712-1vtwkl` — real Windows 10/11 evidence matrix.

## Closed R3 finding matrix

| Area | Final behavior | Regression evidence |
|---|---|---|
| Rotation linearization | Full control credential is validated; the actor/origin recovery scope is held across preflight, remote rotation, local update, and release; local metadata update matches the exact control token generation. Remote one-time material is still returned alongside a local persistence error. | `TestOnboardingRotateIsScopeLockedAndControlGenerationBound`; existing actor-mismatch and one-time-material tests |
| Crash convergence | An exact promoted pending token can be retired after a later rotation only when actor/origin match and the newer recovery generation is unconsumed; its acknowledgement and ciphertext bytes are preserved. Other identity changes fail closed. | `TestRecoveryDeleteCrashPreservesLaterRotationAndAcknowledgement`; `TestRecoveryExactPromotionIdentityFailsClosed` |
| Clipboard lifecycle | Copy/clear operations serialize without holding state across the UI dispatcher. Automatic cleanup has a TTL plus exactly three retries (1s/5s/30s), then exposes only `automatic_cleanup_failed`; the exact lease remains available for explicit retry. | `TestRecoveryClipboardAutomaticClearRetriesAreBounded`; `TestRecoveryClipboardDispatcherCanObserveStatusWithoutLockInversion`; clipboard race suite |
| Win32 clipboard ownership | `GlobalUnlock` clears/reads last-error on one pinned OS thread; all task-owned `GlobalFree` results are checked; ownership transfer and remaining-handle cleanup are indexed from the registered marker count. | Windows amd64/arm64 vet/build/test compilation; host fake-backend fault matrix; primary API documentation review |
| Credential wire schema | Bundle top-level and nested objects reject unknown/missing/wrong-scalar/explicit-null shapes before struct unmarshal. | `TestCredentialBundleRejectsExplicitNullOptionalFields`; protected-envelope strictness suite |
| HTTP response contract | Exactly one JSON media type is required, with no parameters except optional UTF-8 charset. `Retry-After` is one byte-canonical positive decimal value exactly matching the integer body, and is forbidden on non-429 responses. | malformed media, retry header, and retry body tables |
| JSON Unicode | Raw JSON must be valid UTF-8 and every escaped surrogate must be a well-formed pair before decoding. | `TestOnboardingHTTPRejectsInvalidJSONUnicode` including a valid astral pair |
| Cancellation | Safe `context.Canceled`/`DeadlineExceeded` identity survives body read/close failures; all other reader details remain redacted. | `TestOnboardingHTTPCancellationIsStable` |
| Request and formatting privacy | Mutable request bodies are zeroed immediately after transport return; all case variants of Authorization are deleted from owned original/final requests; sensitive service containers have redacted normal/debug formatting. | `TestOnboardingHTTPClearsBearerFromOwnedRequestAfterTransport`; `TestSensitiveServiceContainerFormattingIsRedacted`; clipboard formatting canary |

## Final verification matrix

All commands below passed after the last production/test edit.

| Command | Result |
|---|---|
| `go test . -run 'Test(Credential|Legacy|Repair|CrossOrigin|Recovery|Durable|Protected|DPAPI|Stale|Canonical|Coordinator|Human|Onboarding|OneTime|Sensitive)' -count=50` | PASS (`relux.works/duet/pulsar-win`, 50 focused repetitions) |
| `go test -count=1 ./...` | PASS: root, probe, winprobe, and wire packages |
| `go test -race -count=1 ./...` | PASS: full module race suite |
| `go vet ./...` | PASS |
| `go build ./...` | PASS |
| `go mod verify` | PASS (`all modules verified`) |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet .` | PASS |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ...` | PASS, SHA-256 `12830a7751af5c63a4e30ca0fca479a97614200a095e76d480709d4fd93fd693` |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c ...` | PASS, SHA-256 `68264d1002c7bd35f6ec9c84a9f6151ddb61783155883993291038a263333499` |
| `GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go vet .` | PASS |
| `GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build ...` | PASS, SHA-256 `e61c9f46bb2f9309dfb6e5ee58c794fc8435f45e28a7aa1e3253edc1ada55747` |
| `GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go test -c ...` | PASS, SHA-256 `a3a928a272589c9cab9d8e81f23ef913724ecc650c6340a263c67f45765fe72e` |
| Explicit `gofmt`; `git diff --check` | PASS |
| Scoped production canary/log/plaintext/deep-link scan | PASS; no task canary in production, sensitive sinks manually classified |
| `task-board validate` | PASS after R3/R4 resource attachment and tracker update |

Host tests use injected protectors, transports, file operations, schedulers,
and clipboard backends. They did not touch a developer credential store,
clipboard, or real network.

## Final changed-file SHA-256 inventory

| SHA-256 | File |
|---|---|
| `030e2a2a8ed426d23b065c32747af085ff58f2f211a626b967f252533688bd66` | `pulsar-win/onboarding_http.go` |
| `c05c610b7faba84d519a98d0dff23ec8618c73e3d9b6feaf598d55f12b053101` | `pulsar-win/onboarding_http_test.go` |
| `e6aed8a4917a93a17ca2aacaf79e81b924a3531164deeba3ff83e35d77bfdde1` | `pulsar-win/onboarding_service.go` |
| `d707e136b67f8d3ed71a3fa3e58143e762416debb289cfdd17689bd8e7f5d5aa` | `pulsar-win/onboarding_service_test.go` |
| `5021868e5f32b37f962c633d0a250a611eabe7a9cb06c6eaf1e610db273318c9` | `pulsar-win/protected_repository.go` |
| `8f049c07f99dbb50d70d02b02afba3b93d9c56598cfd1faacee53d46c55a0953` | `pulsar-win/protected_repository_test.go` |
| `1b3c12901d20c0f02c40c553c4f689fc973141f0d347f2983679f4bd20d49dab` | `pulsar-win/recovery_clipboard.go` |
| `bfe1d6da9acdbfbb55acc52531bc4b2319138ce456b4b0bc891a50c16f797e72` | `pulsar-win/recovery_clipboard_windows.go` |
| `52b1fec25d888b01c269c34e6a04f2227416b0152f065f39208e0fcf6bf671d8` | `pulsar-win/recovery_export.go` |
| `f55c6a12a6b59989edeca82ff193e522dae0f09308e5183d41e400316bde2ac2` | `pulsar-win/recovery_export_clipboard_test.go` |
| `b146a4f0447694d185e5d87eaa1ebad599cd84df9ab2e9870868e040ca76b93e` | `pulsar-win/recovery_service.go` |
| `ef085fdae50905acef29db0a421f5dca57fcd86ad637922bccb746d2e1037ab9` | `pulsar-win/recovery_service_test.go` |
| `74af6aa1bb94b35002918c49d654c1021a39b8d0d8da54544d4a23483e1907d0` | `pulsar-win/strict_json.go` |

## Native Windows evidence gap

| Gate | Current evidence | Required downstream proof |
|---|---|---|
| Current-user DPAPI and native allocation cleanup | Source/fault audit plus amd64/arm64 compile | Execute encrypt/decrypt/corruption/failure cases under a real supported Windows user profile |
| Durable NTFS write/read-back and cross-process lock | Injectable fault/concurrency suite plus compile | Run crash/fault schedules on Windows/NTFS and inspect handle ownership |
| Real HWND clipboard | Deterministic fake lifecycle tests plus Win32 source/API audit | Exercise owner-window copy, contention, sequence changes, history/cloud exclusion formats, and terminal/manual cleanup on Windows |
| Installed upgrade/rollback | Not executed in this task | Upgrade a real pair-only installation, verify protected read-back and plaintext removal, then run rollback matrix in `TASK-260712-38qsku` |
| Signed package and Windows 10/11 | Not available here | Close `TASK-260712-13rbnw` and `TASK-260712-1vtwkl` with real signed hardware evidence |

## Rollback

Revert the single landed task commit if a downstream native gate finds a
blocking defect. Preserve both the versioned protected destination and any
surviving exact legacy source for diagnosis; never restore or synthesize a
plaintext credential fallback. A rollback that changes credential layout must
be exercised by `TASK-260712-38qsku` before release.

## Review limitation

Implementation, cold review, remediation, and root audit were performed by the
same executor to honor the user's strict inline/no-spawn instruction. The R3
historical rejection and R4 acceptance are separated, but they are not an
independent-party security attestation.
