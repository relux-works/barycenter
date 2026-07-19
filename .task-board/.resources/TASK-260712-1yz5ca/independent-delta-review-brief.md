# Independent delta review: TASK-260712-1yz5ca

Review exact producer commit `e4488ed2c0335e57910d704cf4bb4119593bbfdd` from `origin/feat/task-260712-1yz5ca` against its first parent. Do not implement fixes or modify repository code. You may update only task-board review state/resources as required by the reviewer role.

Return an explicit `APPROVE` or `REJECT`. Reject for any open Critical or High finding, dishonest evidence claim, plaintext/downgrade path, secret persistence/logging, authorization or lineage bypass, unbounded storage/egress/live state, unsafe migration/rollback, or a producer hash mismatch. Record Medium/Low/informational findings separately with exact file/line evidence and state whether they block this production-dark task.

Challenge these boundaries independently:

- Coordinator remains keyless and routes only encrypted manifests, opaque envelopes, ciphertext chunks, and distinct `BE` live frames. It must not select a production suite/container/library, decrypt payloads, persist live frame bytes, advertise `e2ee_media_v1`, or register production HTTP/WS routes.
- Protected objects bind exact actor/device/protocol/Air membership lineage, epoch, generation, target snapshot and manifest/ciphertext hashes. Check persisted fork, removed/rejoined/revoked/unsupported/non-target paths, author-only mutation, finalization completeness, immutable chunks, If-Range and aligned bounded ranges.
- Check upload and per-device egress bounds, tiny-range admission floor, failed-transaction quota rollback, staging limits, delete chunk removal and the explicit limitation that already copied client keys/ciphertext cannot be revoked by the coordinator.
- `BE` cannot downgrade to legacy plaintext `BP`. Check replay/gap/capture timing/rate/duration bounds; one active session; monotonic generation across restart; frame payload non-persistence; membership-change termination; slow/block/DND/policy/revoked/unsupported recipient isolation; monotonic idempotent receipts; bounded terminal pruning.
- Check the additive SQLite migration and exact-previous-head compatibility. Legacy media/transmission/inbox/history/cue tables must stay untouched by the new routers and feature-off behavior must remain intact.
- Confirm evidence is honest: runtime HTTP/WS wiring, selected cryptography, physical hardware, real app playback, external crypto implementation review, and production activation all remain explicitly unproved/open.

Recompute every SHA-256 pin in `acceptance/phase3/e2ee-opaque-media-router-v1.json`. Run at minimum:

1. `cd coordinator && go test ./...`
2. `cd coordinator && go vet ./...`
3. `cd coordinator && go test -race ./internal/store ./internal/e2eecontract -run 'TestE2EEOpaque|TestOpaqueLive' -count=1`
4. `cd coordinator && go test -race -timeout 15m ./internal/store ./internal/e2eecontract -count=1`
5. `python3 scripts/acceptance/validate_e2ee_opaque_media_router.py`
6. The exact acceptance-contract command in `scripts/acceptance/run_automated.py`; expected count is 212.

Producer evidence to verify, not trust: focused race store 10.285s / contract 1.388s; full race with `-timeout 15m` store 594.955s / contract 1.460s; acceptance 212/212. The earlier default-timeout full race was a non-accepted attempt: it timed out after 10m in unrelated `TestTransmissionSchedulerRechecksDNDWithoutSuppressingUserMessagesOnly`, with no race diagnostic.

If approved, create outcome resource `TASK-260712-1yz5ca_independent-delta-review-v1.md` containing exact SHA, reviewer model, verdict, findings by severity, reproduced commands/timings/counts, hash inventory result, migration/rollback result, keyless/production-dark determination, and all open EPC/manual/external gates. Check the task checklist and set the task to done only for this explicitly scoped dormant engineering result. If rejected, leave it routed for correction with the same evidence quality.
