# TASK-260712-47uve0 — cold security/migration review R3

Date: 2026-07-14 (Asia/Tbilisi)
Reviewed boundary: the 29 frozen Windows files in
`TASK-260712-47uve0-independent-review-guard-r3.md`
Baseline: `976cef3add17e59714bb43f5a0e3a5cf6fe06cfa`

## Review identity and boundary

All 29/29 frozen file hashes matched the R3 guard before substantive review.
The earlier R2 run was only a shared-`LOGBOOK.md` boundary abort and made no
code judgment.

This was a cold second pass by the same strict-sequential executor. The user
explicitly required inline execution outside the task-board spawn workflow, so
independence is not claimed. This report records the verdict on the original
frozen R1 bytes; fixes and final acceptance are evaluated separately in the R4
root-audit artifact.

## Verdict

`BACK TO DEVELOPMENT`

## Severity-ranked findings on the frozen R1 bytes

| Severity | Frozen location | Concrete failure schedule | Missing regression / disposition |
|---|---|---|---|
| High | `pulsar-win/onboarding_service.go:57`; `protected_repository.go:152` | Rotation was neither actor-scope locked nor bound to the exact stored control-token generation. A competing credential update between the remote rotation and local metadata write could let a stale caller overwrite recovery metadata for a newer control generation. | Reproduced by `TestOnboardingRotateIsScopeLockedAndControlGenerationBound`; fixed with recovery-scope serialization, preflight identity checks, and exact capability-bound metadata update. |
| High | `pulsar-win/recovery_clipboard.go:134` | Every failed automatic clear scheduled another timer forever. Persistent clipboard contention retained a secret-bearing lease indefinitely without a terminal state or bounded retry budget. | Reproduced by `TestRecoveryClipboardAutomaticClearRetriesAreBounded`; fixed with three retries (1s/5s/30s), terminal generic status, and explicit manual retry. |
| High | `pulsar-win/recovery_clipboard.go:81,134` | The state mutex was held while waiting for a synchronous UI dispatcher. A dispatcher that observed cleanup state re-entered the mutex and deadlocked copy/cleanup, extending exposure indefinitely. | Reproduced by `TestRecoveryClipboardDispatcherCanObserveStatusWithoutLockInversion`; fixed by separating operation serialization from short state locking. |
| High | `pulsar-win/onboarding_http.go:419`; sensitive service container structs | Owned request objects retained the bearer after transport completion, including the final response request. Default `%+v`/`%#v` formatting of repository/client/service/exporter containers exposed origins, paths, or nested sensitive state. | Reproduced by `TestOnboardingHTTPClearsBearerFromOwnedRequestAfterTransport` and `TestSensitiveServiceContainerFormattingIsRedacted`; fixed with case-insensitive header scrubbing and explicit redacted `String`/`GoString` methods. |
| High | `pulsar-win/protected_repository.go:388` | After control promotion, a crash before pending deletion followed by recovery rotation left the stale pending record permanently conflicting with the newer recovery generation. Resume could never converge or delete the exact stale record. | Reproduced by `TestRecoveryDeleteCrashPreservesLaterRotationAndAcknowledgement`; fixed by recognizing only the exact already-promoted control identity and preserving the newer unconsumed generation byte-for-byte. |
| Medium | `pulsar-win/recovery_clipboard_windows.go:208,245,253` | `GlobalUnlock` treated a zero return using potentially stale thread-local last-error state, while `GlobalFree` failures were ignored. Marker/text cleanup also depended on a hard-coded handle index. Native failure paths could be misclassified or leak task-owned allocations. | Fixed with OS-thread-pinned last-error clearing, documented zero/`NO_ERROR` handling, checked all `GlobalFree` results, and derived text index. Cross-compiled only; native runtime remains a downstream gate. |
| Medium | `pulsar-win/protected_repository.go:686` | Protected bundle decoding accepted explicit `null` for optional objects/scalars because `encoding/json` normalized them to nil/zero values before model validation. Hostile protected input was not byte-schema strict. | Reproduced by `TestCredentialBundleRejectsExplicitNullOptionalFields`; fixed with an exact pre-unmarshal object/nested-scalar schema. |
| Medium | `pulsar-win/onboarding_http.go:470,517` | Duplicate or parameter-confused `Content-Type` values and noncanonical/duplicate `Retry-After` representations were accepted. An empty retry header on a non-429 response was also invisible through `Header.Get`. | Reproduced by the malformed-media and retry-header/body regression tables; fixed with case-insensitive raw header enumeration, strict media parsing, and byte-canonical decimal matching. |
| Medium | `pulsar-win/strict_json.go:15` | Invalid raw UTF-8 and unpaired UTF-16 surrogate escapes were accepted by `encoding/json` as replacement characters. Distinct wire bytes could collapse to the same decoded value. | Reproduced by `TestOnboardingHTTPRejectsInvalidJSONUnicode`; fixed with raw UTF-8 and surrogate-pair validation before decoding. |
| Low | `pulsar-win/strict_json.go:125`; `onboarding_http.go:419` | Cancellation while reading or closing a response body was flattened into `invalid_response`, unlike cancellation during `Do`. | Reproduced in `TestOnboardingHTTPCancellationIsStable`; fixed while preserving only the safe cancellation identity. |

Every finding was first represented by a failing regression against the frozen
behavior, then fixed on the task branch. No unrelated product area was edited.

## Native evidence boundary

This macOS review did not run current-user DPAPI, Win32 durable file/lock APIs,
or a real HWND clipboard path, and it did not perform an installed-MSIX upgrade.
Windows amd64/arm64 compile evidence cannot replace those runtime claims.

The native API corrections were checked against primary documentation:

- [GlobalUnlock](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-globalunlock)
- [GlobalFree](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-globalfree)
- [SetClipboardData](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setclipboarddata)
- [IsClipboardFormatAvailable](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-isclipboardformatavailable)
- [GetClipboardData](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getclipboarddata)
- [Go `runtime.LockOSThread`](https://pkg.go.dev/runtime#LockOSThread)
