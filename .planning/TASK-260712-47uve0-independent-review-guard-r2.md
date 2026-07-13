# TASK-260712-47uve0 — independent Windows security/migration review R2

Date: 2026-07-13 (Asia/Tbilisi)

This is a review-only run. The R1 producer handoff is not accepted. Independently
attempt to falsify the implementation and its evidence. Do not edit production
or existing test sources, commit, push, reset, clean, use real network, touch a
developer credential store/clipboard, or mark the task done. Review-only tests
may be created only in a fresh `/tmp` copy. The sole workspace output is the
required review report resource.

Read completely before judging:

- `TASK-260712-47uve0_implementation-r1-results.md` (SHA-256
  `18f356dc7ed8b58d0182f254895478438034c5c0243bbe7a3e52c01f48da9995`);
- `TASK-260712-47uve0-implementation-guard-r1.md` (SHA-256
  `3800f471301c4263452b3194f25bbd72574adb92e0062f3c000f5feccf2dc0a2`);
- every production and test file below, in full, plus relevant frozen callers
  and coordinator protocol implementations. Do not trust producer summaries or
  tests merely because they pass.

## Frozen R1 boundary

Abort and report a boundary violation if any hash differs before or after the
review:

```text
64448e05231609e9a96a7aa125d5db3fd508cab19ead1abaa04c556364c7f7ba  LOGBOOK.md
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

1. Audit credential/pending schemas as hostile protected input: missing,
   duplicate, unknown and trailing fields; explicit `null`; wrong scalar types;
   noncanonical numbers; nested-object variants; corrupt envelopes; partial
   unprotect output; exact zero/free/close ownership.
2. Exercise every durable-file and migration crash boundary: legacy plus absent,
   equivalent, control-only, conflicting or corrupt destination; temp create,
   short/zero write, flush, close, move, reopen, size/read/decrypt/decode/read-close
   failures; stale-temp ownership; process/repository concurrency. Distinguish
   the actual error reached so a test cannot pass for the wrong reason.
3. Audit origin and bearer binding: complete UTS46/STD3/Bidi/Joiner vectors,
   mapped trailing roots, ambiguous IPv4, IPv6 zones, encoded authorities,
   literal-loopback-only HTTP, exact WS `/ws`, final method/origin/path, real
   307/308 behavior, and typed node/control immutable identity.
4. Audit all endpoint-specific success and error semantics against the actual
   coordinator: exact method/path/body/auth/status/media/cache headers, create
   title echo, roles/slots/tokens/codes/bot username, strict errors, cancellation
   during Do and body read, body close failures, bounded streaming, and zeroing
   of owned mutable secret copies.
5. Audit recovery crash/concurrency schedules: exact-scope cross-process lock,
   unsent-to-sent barrier, cancellation on both sides of it, sent restart probe,
   200/401/403/429/5xx/network cases, limited-context metadata across repeated
   promotion, active promotion/delete/rotation/ack races, node byte preservation,
   and exact identity before pending deletion. No duplicate unauthorized send.
6. Audit direct export and the actual Win32 clipboard code, not just fakes:
   complete write/sync/checked-close behavior; marker allocation/transfer/free;
   sequence and API-failure sentinels; `GlobalLock`/`GlobalUnlock` documented
   last-error semantics; exposure ambiguity; lease replacement; transient clear
   retry; exact atomic compare-and-clear; no newer-content deletion.
7. Audit formatting, reflection, errors, request retention, filenames and logs
   with adversarial public canaries. Distinguish intended secret-bearing request,
   encrypted persistence, explicit display/export/copy from unintended copies.
8. Independently rerun focused repetitions, full/race tests, vet/build/module
   verification, Windows amd64/arm64 vet/build/test compilation, formatting,
   diff check, and board validation. Record unavailable native-Windows gates
   honestly; cross-compilation is not runtime proof.

Severity-rank each finding with exact file/line references, a concrete schedule,
and the missing/false-positive test. If no defect is found, state the adversarial
schedules actually attempted. Write exactly
`TASK-260712-47uve0_independent-review-r2.md`, attach it as an outcome resource,
and give exactly one verdict: `ACCEPT FOR ROOT AUDIT` or `BACK TO DEVELOPMENT`.
Do not mark the task done; root retains acceptance authority.
