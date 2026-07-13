# TASK-260712-47uve0 — implementation R1 results

Date: 2026-07-13  
Role: developer  
Scope: reusable Windows protected credential repository, strict onboarding client, crash-safe recovery service, recovery export, and clipboard adapter.

## Behavior matrix

| Area | Implemented behavior | Evidence |
|---|---|---|
| Credential model | Versioned bundle with independent node and control capabilities, control-only recovery state, origin-bound credentials, limited/full control state, and redacted normal/debug formatting | `credential_model.go`; credential-model tests in `protected_repository_test.go` |
| Legacy compatibility | Existing `Credentials`, `LoadCredentials`, `Credentials.Save`, `/pair`, CLI, and Win32 startup callers keep a node-only view; re-pair preserves control | `config.go`, `pair.go`, `main.go`; compatibility tests |
| Protected storage | Long-lived and pending tokens are encoded in a strict envelope and protected by current-user DPAPI on Windows; non-Windows production returns unsupported-platform rather than using plaintext | repository and platform files; DPAPI/fault tests; Windows cross-compilation |
| HTTP boundary | Typed create, invite, join, context, recover, rotate, Telegram-link, and legacy pair calls with strict method/path/auth/status/schema/body limits, disabled redirects, no automatic mutation retry, and redacted public errors | `onboarding_http.go`; HTTP tests |
| Coordinator identity | UTS46/STD3 canonical origin, exact literal-loopback HTTP restriction, safe WS derivation, origin/capability binding, and exact legacy WS preservation after validation | `coordinator_origin.go`; canonical-origin and HTTP tests |
| Recovery | Durable pending-token send barrier, process-global and Windows file-lock serialization, monotonic `ever_sent`, no-secret restart probe, response table, exact promotion, node preservation, and crash convergence | `recovery_service.go`; recovery tests |
| One-time recovery material | Secret field is unexported and redacted, explicit reveal/export/copy only, no JSON/text marshal path, export contains exactly three fields, and backup acknowledgement is explicit | `recovery_export.go`; export/service tests |
| Clipboard | Real non-NULL HWND plus dispatcher, exclusion/history/cloud markers before publication, bounded TTL, lease/sequence/payload checks, bounded retry, and preservation of newer clipboard content | clipboard files; deterministic fake-clipboard tests |
| UI integration boundary | Typed onboarding controller hooks are exposed without redesigning or claiming the Windows UI | `onboarding_service.go` |

## Plaintext migration matrix

| Condition | Result |
|---|---|
| Valid legacy source, no protected destination | Strict bounded read, durable protected write, decrypt/read-back exact comparison, then exact legacy deletion |
| Equivalent protected destination and surviving legacy source | Protected state is validated first; legacy deletion is retried and restart converges |
| Conflicting protected destination | Fail closed; both source and destination are retained |
| Corrupt protected destination | Fail closed; destination is not overwritten and plaintext source is retained |
| Write, flush, close, move, reopen, read, decrypt, compare, or read-close failure | Migration reports failure and retains at least one readable copy; no plaintext fallback is installed |
| Re-pair after migration | Node state changes only; valid control capability remains intact |
| Successful migration | Node token, orbit, slot, and legacy WS URL bytes remain unchanged |

## DPAPI and durable-file fault matrix

| Invariant/fault | Verified response |
|---|---|
| DPAPI flags/scope | `CRYPTPROTECT_UI_FORBIDDEN`; no machine scope, entropy, or secret description |
| Partial native output on protect/unprotect failure | Output is zeroed where possible and `LocalFree` is called exactly once |
| Envelope corruption | Wrong magic/version/length/schema, duplicate/unknown/trailing JSON, truncation, oversize, and trailing bytes are rejected |
| Ciphertext oversize | Size is capped at 1 MiB before allocation |
| Short read/write | Complete loops are required; zero progress fails |
| Temp open/write/flush/close failure | No move occurs; acquired handles receive one close attempt and failed temp cleanup is attempted only after close |
| Durable move failure | Destination is not claimed successful; error text exposes only stable operation classes |
| Read-back/open/size/read/decrypt/compare failure | Protected write is not accepted; corrupt destination is retained for diagnosis |
| Read-handle close failure | Recovery send barrier remains closed |
| Stale temp cleanup | Only task-owned random temp names are eligible; unrelated files are preserved |
| Verified recovery transition | Send callback becomes reachable only after durable `ever_sent=true` write and exact read-back |

## Recovery response matrix

| Operation/result | State transition |
|---|---|
| Consume 200 | Promote the exact pending control/context into the active bundle, preserve node bytes, verify active write, then delete pending |
| Consume 400/403/429/5xx/network/cancel/decode ambiguity | Retain pending state |
| Restart probe 200 | Promote and delete after verified active write |
| Restart probe 403 `insufficient_capability` | Promote limited control context and retain known protected metadata |
| Restart probe 401 | Retain the same tuple/token and require the user secret for retry |
| Restart probe 429/5xx/network | Retain pending state and report retry metadata without secret-bearing errors |
| Probe 401 then recovery 403 | Retain pending; destructive abandon requires explicit warned confirmation |
| Active token already equals pending token after a crash | Verify identity and delete the exact pending record without generating or sending again |
| Cancellation before send gate | May remove only the exact unsent candidate |
| Cancellation after send gate | Cannot erase or replace the sent candidate |

## Clipboard and one-time-material matrix

| Scenario | Result |
|---|---|
| Copy without real owner HWND/dispatcher | Rejected before secret exposure |
| Marker registration/publication failure | Rejected before publishing recovery text |
| Successful copy | Publishes Unicode text plus monitor/history/cloud exclusion formats and records an in-memory lease only |
| Display or copy | Does not mark recovery material backed up |
| Export | Writes only the explicit user destination and exactly `actor_id`, `recovery_id`, `recovery_secret`; partial failure is not reported as success |
| TTL/explicit clear with unchanged clipboard | Sequence and exact payload are checked and the clipboard is emptied in one open critical section |
| External or newer copy | Preserved; stale timers and old leases cannot clear it |
| Transient Win32 contention/sentinel ambiguity | Bounded retry; no unbounded timer or secret-bearing closure metadata |
| Post-exposure close ambiguity | A clearable lease remains available rather than silently losing cleanup ownership |

## Verification commands

All commands below passed after the last code edit.

| Command | Result |
|---|---|
| `go test . -run 'Test(Credential|Legacy|Repair|CrossOrigin|Recovery|Durable|Protected|DPAPI|Stale|Canonical|Coordinator|Human|Onboarding|OneTime)' -count=20` | PASS |
| `go test -count=1 ./...` | PASS, full uncached module suite |
| `go test -race -count=1 ./...` | PASS, full module race suite |
| `go vet ./...` | PASS |
| `go build ./...` | PASS |
| `go mod verify` | PASS |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet .` | PASS |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/pulsar-win-amd64.exe .` | PASS |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/pulsar-win-amd64.test.exe .` | PASS |
| `GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go vet .` | PASS |
| `GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/pulsar-win-arm64.exe .` | PASS |
| `GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/pulsar-win-arm64.test.exe .` | PASS |
| `gofmt` over the explicit task file list | PASS; no unrelated formatting retained |
| `git diff --check` | PASS |
| Scoped production canary/log/plaintext/deep-link scan | PASS |
| `task-board validate` from the project root | PASS: board valid, no issues |

Host tests use injected protectors, transports, file operations, clocks, schedulers, and clipboard fakes. They do not access a developer credential store, clipboard, or real network.

## Changed/new file SHA-256 inventory

| SHA-256 | File |
|---|---|
| `64448e05231609e9a96a7aa125d5db3fd508cab19ead1abaa04c556364c7f7ba` | `LOGBOOK.md` |
| `6abb846231e0ed695d9abfab66036556028a74add76820699f48f77f12fda6b3` | `pulsar-win/config.go` |
| `2638a3fa40f8bfec6acb9b3496a498c998e5e50c9a2e4f9bde11538452520afb` | `pulsar-win/config_test.go` |
| `548ce15595637dbc8e0cfa9b44d1420892e6ce7614bf3d2c934d11b0311e58b0` | `pulsar-win/go.mod` |
| `c62fdd1718d05c49f92e82028786b6fadc206899990af32fa253ee56b2813096` | `pulsar-win/go.sum` |
| `552bb536ff97ba4d49572456b8c351533714e6d13c4786e097739be35579186a` | `pulsar-win/main.go` |
| `9726210e0013aad0dd671dddaf3bd29617a5287bd086a3dfb102ae8616b4ca15` | `pulsar-win/pair.go` |
| `949ff89df386518e0111ad783a2a35976abf55ec6742b2455ba82638e00ca914` | `pulsar-win/pair_test.go` |
| `3db4828b903602ebf3cc14d0ea8f50c84eb1da5d3a413cbb400a2542af6626eb` | `pulsar-win/coordinator_origin.go` |
| `cf1fc740e4744e750a211f224019ce0aa906f6151b4a78ea406df2c5d8a90564` | `pulsar-win/coordinator_origin_test.go` |
| `9f74ff456001ab38b0a41a0dc0f7f6c3bc6e6576b154b32feeb5395c50aee138` | `pulsar-win/credential_model.go` |
| `aed6e75cb66dc462169daa0b1a9029766aba2ddc66b02578d9dc16e303057ce2` | `pulsar-win/data_protector.go` |
| `466d30c852419b89c1b8fdfd00d014aa25c92b54bcdfdd10e9ba26711f70f03a` | `pulsar-win/onboarding_http.go` |
| `9c889ed26304ffed690eb8dbec15dedefe54d6043721d0ada5060473b1073853` | `pulsar-win/onboarding_http_test.go` |
| `861cbe741e1a94298b420d5efc253f5fbdd099a97602a5aa8563e21f1ec42389` | `pulsar-win/onboarding_service.go` |
| `e14be38b351523558234478e8deccfda0ae5da5aa7d8138eacf099010ce75e54` | `pulsar-win/onboarding_service_test.go` |
| `c1867aba4178e6b213a158b152716ceae9f929867eb8aae2d2ca7eeedccda2be` | `pulsar-win/protected_repository.go` |
| `18b554407f99dfcaef1a38ba4bcdee106cf810f753c5090174b4ae8f3618c8a4` | `pulsar-win/protected_repository_test.go` |
| `4edce57785c066e0a7148e1d45e4cd53a6491c37ea6a6447e3cd63c259a8dbb3` | `pulsar-win/protected_platform_nonwindows.go` |
| `6c02770ffac933a081fcbdb886ea2fb59f5d7033cf806887db001678a9b5ecb5` | `pulsar-win/protected_platform_nonwindows_test.go` |
| `77fd1d18cafd17251d3ce5773e11a3aee1ac2b679c82b290286acedd170cfdb1` | `pulsar-win/protected_platform_windows.go` |
| `307cc379d4b84448ca62cac1298cd3b712e87d2c206071de8d58483a2128b185` | `pulsar-win/recovery_service.go` |
| `cca5bbfa1f1944d12e23c6190746a385aeb27d1e057f037fdba04fc29d4752e9` | `pulsar-win/recovery_service_test.go` |
| `becfea801554b79dbea62c345ddbd59c4fdf268a1660ffd306998f99423aa90d` | `pulsar-win/recovery_export.go` |
| `86fdc33beaf39060b4231d3ea7263efa8c3c86d1b6101e846f16859ecef7bdeb` | `pulsar-win/recovery_export_clipboard_test.go` |
| `023438ba97d3654d3aa2f757fcf464e32d3dff0a24c12cc0259af03240afbd1d` | `pulsar-win/recovery_clipboard.go` |
| `213ba4feb8f4e9878028a6217a3589520407f4e6bec09642e38f521b0d7de9e1` | `pulsar-win/recovery_clipboard_nonwindows.go` |
| `49280e935af743fa76ed43ba43a249e17813605f9d8dcced456a25f2bb43d1f7` | `pulsar-win/recovery_clipboard_windows.go` |
| `b837efcfb25b9d1240b009e6a854d1a2b362cb16f73c6ad24970b9e9911cf590` | `pulsar-win/strict_json.go` |

The outcome file digest is recorded by the board resource attachment; embedding its own digest would be self-referential.

## Dirty-tree accounting

- The repository contained broad concurrent sibling work before this task.
- `pulsar-win/.gitignore` and untracked `pulsar-win/cmd/`, `internal/`, `native/`, and `probe-msix/` belong to the concurrent lifecycle/probe producer and were not edited here.
- `LOGBOOK.md` was already shared/dirty; this task appended only its named task entry.
- `pulsar-win/ui_windows.go` remains byte-identical to the frozen baseline (`6ec4135c9b3ff771a7ee15bfaa7950bd3800ed25aff3ea34fe4485aa25c41a61`), and no diff remains in it or `player.go`.
- No commit, push, reset, checkout, clean, or unrelated rewrite was performed.

## Windows runtime-gap matrix

| Gate | This macOS run | Required downstream evidence |
|---|---|---|
| Native current-user DPAPI encrypt/decrypt and failure cleanup | Not runtime-executed; Windows code cross-vetted, cross-built, and test-compiled for amd64/arm64 | Run fault/runtime suite on supported Windows hardware under a real user profile |
| Native Win32 durable file operations and cross-process lock | Not runtime-executed | Exercise crash/fault matrix on Windows/NTFS and inspect handle/file behavior |
| Native clipboard with a real owner HWND/UI dispatcher | Not runtime-executed | Run copy/clear/contention/history/cloud-marker validation through the owning Windows UI |
| Installed MSIX migration | Not executed | Validate upgrade from an existing pair-only installed package and verify plaintext removal after protected read-back |
| Windows hardware end-to-end create/join/recover/Telegram link | Not executed | Execute UI-owner integration and coordinator E2E after the UI/data task consumes these hooks |

## Review gates

Before acceptance, perform root line review, independent security/migration review, changed-file hash comparison, and root reruns of the verification commands. Native Windows DPAPI/clipboard, installed-MSIX migration, and Windows-hardware evidence remain explicit downstream gates rather than host-test claims.
