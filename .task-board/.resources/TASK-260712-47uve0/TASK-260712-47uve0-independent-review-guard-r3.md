# TASK-260712-47uve0 — independent Windows security/migration review R3

Date: 2026-07-13 (Asia/Tbilisi)

This is a fresh review-only run over the unchanged R1 implementation. R2 made
no substantive code judgment because its guard incorrectly froze the shared
concurrently-owned `LOGBOOK.md`; read its procedural report (SHA-256
`e56e5adf5400c28fc344bb98bb411a9e2a4cc275c86cc3d257d8cc662205818e`)
but restart the security/migration review from the beginning. `LOGBOOK.md` is
deliberately outside this R3 boundary and must neither be edited nor used as an
abort condition.

The R1 producer handoff remains unaccepted. Independently falsify production
and evidence. Do not edit production/existing tests, commit, push, reset,
checkout, clean, use real network, or touch a developer credential store or
clipboard. Review-only adversarial tests may be created only in a fresh `/tmp`
copy. The sole workspace output is the required R3 report resource.

Read completely:

- `TASK-260712-47uve0_implementation-r1-results.md` (SHA-256
  `18f356dc7ed8b58d0182f254895478438034c5c0243bbe7a3e52c01f48da9995`);
- `TASK-260712-47uve0-implementation-guard-r1.md` (SHA-256
  `3800f471301c4263452b3194f25bbd72574adb92e0062f3c000f5feccf2dc0a2`);
- the complete R2 procedural report above;
- every frozen production/test file below plus relevant callers and coordinator
  protocol implementations. Do not trust producer claims or passing tests.

## Frozen R1 code boundary

Abort and report a boundary violation if any hash below differs before or after
review. Shared board files, `LOGBOOK.md`, lifecycle/probe subtrees, and unrelated
dirty files are intentionally not frozen and must simply be preserved.

```text
6abb846231e0ed695d9abfab66036556028a74add76820699f48f77f12fda6b3  pulsar-win/config.go
2638a3fa40f8bfec6acb9b3496a498c998e5e50c9a2e4f9bde11538452520afb  pulsar-win/config_test.go
548ce15595637dbc8e0cfa9b44d1420892e6ce7614bf3d2c934d11b0311e58b0  pulsar-win/go.mod
c62fdd1718d05c49f92e82028786b6fadc206899990af32fa253ee56b2813096  pulsar-win/go.sum
552bb536ff97ba4d49572456b8c351533714e6d13c4786e097739be35579186a  pulsar-win/main.go
9726210e0013aad0dd671dddaf3bd29617a5287bd086a3dfb102ae8616b4ca15  pulsar-win/pair.go
949ff89df386518e0111ad783a2a35976abf55ec6742b2455ba82638e00ca914  pulsar-win/pair_test.go
3db4828b903602ebf3cc14d0ea8f50c84eb1da5d3a413cbb400a2542af6626eb  pulsar-win/coordinator_origin.go
cf1fc740e4744e750a211f224019ce0aa906f6151b4a78ea406df2c5d8a90564  pulsar-win/coordinator_origin_test.go
9f74ff456001ab38b0a41a0dc0f7f6c3bc6e6576b154b32feeb5395c50aee138  pulsar-win/credential_model.go
aed6e75cb66dc462169daa0b1a9029766aba2ddc66b02578d9dc16e303057ce2  pulsar-win/data_protector.go
466d30c852419b89c1b8fdfd00d014aa25c92b54bcdfdd10e9ba26711f70f03a  pulsar-win/onboarding_http.go
9c889ed26304ffed690eb8dbec15dedefe54d6043721d0ada5060473b1073853  pulsar-win/onboarding_http_test.go
861cbe741e1a94298b420d5efc253f5fbdd099a97602a5aa8563e21f1ec42389  pulsar-win/onboarding_service.go
e14be38b351523558234478e8deccfda0ae5da5aa7d8138eacf099010ce75e54  pulsar-win/onboarding_service_test.go
c1867aba4178e6b213a158b152716ceae9f929867eb8aae2d2ca7eeedccda2be  pulsar-win/protected_repository.go
18b554407f99dfcaef1a38ba4bcdee106cf810f753c5090174b4ae8f3618c8a4  pulsar-win/protected_repository_test.go
4edce57785c066e0a7148e1d45e4cd53a6491c37ea6a6447e3cd63c259a8dbb3  pulsar-win/protected_platform_nonwindows.go
6c02770ffac933a081fcbdb886ea2fb59f5d7033cf806887db001678a9b5ecb5  pulsar-win/protected_platform_nonwindows_test.go
77fd1d18cafd17251d3ce5773e11a3aee1ac2b679c82b290286acedd170cfdb1  pulsar-win/protected_platform_windows.go
307cc379d4b84448ca62cac1298cd3b712e87d2c206071de8d58483a2128b185  pulsar-win/recovery_service.go
cca5bbfa1f1944d12e23c6190746a385aeb27d1e057f037fdba04fc29d4752e9  pulsar-win/recovery_service_test.go
becfea801554b79dbea62c345ddbd59c4fdf268a1660ffd306998f99423aa90d  pulsar-win/recovery_export.go
86fdc33beaf39060b4231d3ea7263efa8c3c86d1b6101e846f16859ecef7bdeb  pulsar-win/recovery_export_clipboard_test.go
023438ba97d3654d3aa2f757fcf464e32d3dff0a24c12cc0259af03240afbd1d  pulsar-win/recovery_clipboard.go
213ba4feb8f4e9878028a6217a3589520407f4e6bec09642e38f521b0d7de9e1  pulsar-win/recovery_clipboard_nonwindows.go
49280e935af743fa76ed43ba43a249e17813605f9d8dcced456a25f2bb43d1f7  pulsar-win/recovery_clipboard_windows.go
b837efcfb25b9d1240b009e6a854d1a2b362cb16f73c6ad24970b9e9911cf590  pulsar-win/strict_json.go
6ec4135c9b3ff771a7ee15bfaa7950bd3800ed25aff3ea34fe4485aa25c41a61  pulsar-win/ui_windows.go
```

## Mandatory falsification areas

1. Treat credential/pending schemas as hostile protected input: missing,
   duplicate, unknown/trailing fields; explicit `null`; wrong scalar/nested
   types; noncanonical numbers; corrupt envelopes; partial unprotect output;
   exact zero/free/close ownership.
2. Exercise every durable-file/migration boundary: absent/equivalent/
   control-only/conflicting/corrupt destination; temp create, short/zero write,
   flush, close, move, reopen, size/read/decrypt/decode/read-close failures;
   stale-temp ownership; repository/process concurrency. Prove the intended
   branch was reached rather than accepting a test that failed earlier.
3. Audit UTS46/STD3/Bidi/Joiner origin vectors, mapped root dots, ambiguous
   IPv4, IPv6 zones, encoded authorities, literal-loopback HTTP, exact WS path,
   final method/origin/path, 307/308 behavior, and immutable typed bearer origin.
4. Audit endpoint-specific request/success/error semantics against coordinator:
   exact method/path/body/auth/status/media/cache headers, title echo,
   roles/tokens/codes/bot name, cancellation during `Do` and body read, close
   failures, bounded streaming, and owned mutable-secret zeroing.
5. Audit recovery crash/concurrency schedules: exact scope lock, send barrier,
   cancellation on both sides, all probe/consume responses, repeated limited
   promotion, active promotion/delete/rotation/ack races, rotation capability
   generation binding, node-byte preservation, and exact pending deletion.
6. Audit direct export and actual Win32 clipboard code: checked write/sync/close;
   marker allocation/transfer/free; sequence/API sentinels; documented
   `GlobalLock`/`GlobalUnlock` last-error semantics; exposure ambiguity; lease
   replacement; bounded retry; atomic compare-and-clear; newer-content safety.
7. Audit formatting/reflection/errors/request retention/filenames/logs with
   public canaries. Separate intended request/encrypted persistence/explicit
   reveal/export/copy from disclosure.
8. Independently run focused repetitions, full/race tests, vet/build/module
   verification, Windows amd64/arm64 vet/build/test compilation, formatting,
   diff check, privacy scans, and board validation. State native-Windows gaps.

Severity-rank every finding with file/line, concrete schedule, and missing or
false-positive test. Write exactly
`TASK-260712-47uve0_independent-review-r3.md`, attach it as an outcome resource,
and give exactly one verdict: `ACCEPT FOR ROOT AUDIT` or `BACK TO DEVELOPMENT`.
Do not edit the task implementation or mark it done; root retains acceptance.
