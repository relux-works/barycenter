# TASK-260712-2i0w6x independent exact-SHA review — VERDICT: ACCEPTED

Reviewer: independent security/privacy/persistence/correctness review (Fable, max effort).
Reviewed commit (verified in isolated detached worktree, clean before and after): `66a34edcbdf8c60fe5827041f0809930c46cfc69`.
No mutable working-tree production files were reviewed; no code was modified.

## Automated evidence re-run (all green, at the exact SHA)

- Focused: `go test ./internal/store ./internal/moderation -run 'TestE2EEReport|TestE2EEModerationDecision|TestServiceE2EE' -count=1` — ok.
- Focused race: same with `-race` — ok.
- Full coordinator: `go test ./...` — 22 packages ok, zero failures; `go vet ./...` clean.
- Packet unittest: `python3 -m unittest scripts.acceptance.test_e2ee_report_moderation_export` — 5/5 OK; validator prints `PASS (production disabled)`.
- Clean full harness: `python3 scripts/acceptance/run_automated.py --suite coordinator --require-clean` — status **pass**, 7/7 steps (contract tests: 227 OK, container probe + race, vet, coordinator tests, moderation contract, previous-head rollback), manifest git head `66a34ed…`, startDirty=false, endDirty=false.
- All 9 SHA-256 pins in `acceptance/phase3/e2ee-report-evidence-moderation-export-v1.json` recomputed and verified. Cascading re-pins in the 6 touched prior packets (schema-foundation, routing-rotation, opaque-router, macOS/Windows key-state, privacy/store pre-review) match the new on-disk hashes and their validators pass inside the harness — no prior-packet regression.

## Acceptance-criteria verification

- **No server-side decrypted evidence without recipient action + consent receipt** — confirmed at the store boundary. Schema admits no BLOB/plaintext/key columns in report tables (validator enforces); the only evidence entry is `AttachE2EEReportEvidence`, which requires an `explicit_report_evidence_export` consent row written atomically with metadata+state. The legacy unbound `CreateE2EEReportEvidenceMetadata` bypass was closed (now delegates to the strict transition; old permissive test updated to expect rejection). No crypto, no logging in the new store file; no `cmd/` runtime wiring, route, storage adapter, or advertised capability (grep + fail-closed validator).
- **Scoped moderation access, audited, expiring** — List/Evidence/Decide capabilities enforced per method; operator tokens resolve by hash and fail closed when revoked (tested). Authorize returns only the opaque `evidence/v1/<at-rest-digest>` reference and binding metadata; `evidence.read` is audited; expiry is enforced both on read and by the sweep; terminal (deleted/expired) references can never be re-authorized. Audit is append-only (UPDATE/DELETE triggers abort; tamper-tested) and content-free (ids/event types only; no statement or evidence data).
- **Metadata-only reporting functional** — creates the report row plus audit only; test asserts zero rows in all three evidence tables pre-consent and reporting still works on a revoked object.
- **Revoked access blocks new exports** — attach re-authorizes via `authorizeE2EEProtectedRecipientTx`: verified unrevoked device, unrevoked actor, clean fork, exact current snapshot, exact membership lineage, plus exact report/object/device/manifest/epoch/generation/revision binding and consent-before-create ordering. Revoked-object export denial is tested.
- **No plaintext before consent** — storage side proven by test (zero evidence rows, plaintext-pattern scan of report rows). Traffic capture, real devices, signed packages, provider deletion, live mailbox, real-crypto interop are honestly declared not-run and deferred to `EPIC-260714-th54l3` / open gates; the packet validator fails closed if any manual claim is invented or production flags flip.

## Architecture-fit checks

- Moderation `delete_media` reuses the canonical opaque purge: `deleteE2EEProtectedObjectTx` (chunk DELETE + terminal status + audit) extracted and shared by both the author delete path and `DeleteE2EEProtectedObjectForModeration`; behavior of the existing path is preserved (full suite + opaque-router packet green). Chunk removal after moderation delete is asserted in tests.
- Actor/orbit actions reuse canonical `DisableActorForModeration`/`DisableOrbitForModeration`, transmission cancellation, and disconnect paths via the dormant `ApplyE2EEDecision` seam.
- Schema is additive (`CREATE TABLE IF NOT EXISTS`), FK-checked at init, restart-safe (reopen + decision replay tested); checkpoint-injection tests prove report and evidence creation roll back atomically; attach/delete/decision replays are idempotent; revision-guarded updates handle concurrency (race suite green).
- ADR and packet are accurate and appropriately modest: production-dark, no runtime claims, exact runtime obligations for a future wiring task spelled out.

## Non-blocking findings (Low/Info; no action required for acceptance)

1. **Low** — `CreateE2EEModerationReport` reuse ignores a differing reason/statement on a repeat report (silently returns the first report). Matches legacy non-E2EE reporting semantics; future runtime UX should surface this.
2. **Low** — a pending `delete_media` decision can only be physically applied by the operator who requested it (`requested_by_operator_id` gate). If that operator is revoked mid-flow the purge has no recovery path short of a new seam. Deliberate least-privilege; note for the runtime task.
3. **Low** — `ListE2EEReportAuditEvents` caps at 500 with no pagination; every read appends an event, so a long-lived report's audit could exceed one page.
4. **Info** — the expiry sweep predicate (`m.retention_expires_at`) is not covered by the `e2ee_report_evidence_expiry` index (which is on state status/updated_at); negligible at current scale.
5. **Info** — legacy `e2ee_report_evidence_metadata` has no immutability trigger and its `report_id` has no FK to the new reports table; integrity is enforced transactionally by the single strict writer and the UNIQUE/FK bindings on consents/state. Hardening opportunity only.
6. **Info** — `AuthorizeE2EEReportEvidence` reports deleted evidence as `ErrModerationEvidenceExpired` (terminal states conflated in one error); pinned by tests, cosmetic.

## Verdict

**ACCEPTED — no Critical/High/Medium findings; nothing blocking.** Implementation matches the acceptance criteria at the declared production-dark scope, fits the established E2EE/moderation architecture, reuses canonical delete/disable paths, and all automated evidence re-ran green at the exact SHA. Manual/production evidence remains correctly unclaimed in `EPIC-260714-th54l3` and open production gates.
