# Independent exact-SHA review verdict: TASK-260712-1rziyo

**Verdict: ACCEPTED**

Reviewed commit `94e506629c46473bc890575539750b1a993bbc50` (full diff from parent `85673ef`) in a detached worktree (`git worktree add --detach`, no production files modified). Review date: 2026-07-20.

## Independently re-run evidence (all in the detached worktree at the exact SHA)

| Evidence | Result |
|---|---|
| `go test ./internal/store -run 'Test(E2EERecovery|E2EEHistoryGrant|E2EESameUser|E2EEOneTime|LegacyUnboundE2EE)' -count=1` | ok (1.3s) |
| same, with `-race` | ok (21.0s) |
| `pulsar-win: go test -race ./... -run TestWindowsE2EE -count=1` | ok, all packages |
| `node-app: swift test --filter MacE2EEKeyStateTests` | 11/11 tests passed |
| `python3 -m unittest scripts.acceptance.test_e2ee_recovery_device_transfer` | 5/5 OK |
| Full `scripts/acceptance/run_automated.py` | 16/16, exit 0 |
| Acceptance packet artifact SHA-256 pins vs worktree files | all 11 match |
| Producer receipt `.temp/acceptance/task-260712-1rziyo-local/manifest.json` | present, 16 logs, inspected |

## Answers to the six required review questions

1. **Current-epoch bootstrap / transfer packages** — YES, fail-closed. Packages are bound at create AND consume to the exact clean group revision, current epoch, target snapshot digest, issuer+recipient actor/Orbit, member revisions, device revisions, and verification digests (`e2ee_recovery.go` create :112, consume :206). `device_transfer`/`recovery` require two distinct verified devices in the same Orbit (:152-155); TTL ≤ 15 min; one-time consume via `status='pending' AND revision=?` CAS; replay/foreign/expired/revoked/stale/clone all verified failing in `TestE2EERecoveryTransferIsBoundCurrentAndOneTime` and `TestE2EESameUserDeviceTransferBootstrapsCurrentEpochWithoutHistory` (which also asserts zero implicit history grants for the new device).
2. **History grants** — YES. Scoped to one named `ready` protected object with an epoch interval containing the object's epoch; issuer must be object author device or current Air owner (:464-466); `one_time` (max_reads=1) or `time_bound` (≤32 reads, ≤30 days); read budget is atomic (`UPDATE ... WHERE read_count < max_reads`, one winner proven under `-race` in `TestE2EEOneTimeHistoryGrantConcurrentReadHasOneWinner`); expiring, revocable by issuer or recipient; audit rows are content-free (operation/outcome/actor/device/epoch only, verified against `e2ee_audit_events` schema).
3. **Lost-device revocation** — YES, atomic. `RevokeE2EEPublicDevice` now revokes every pending transfer package and active history grant touching the device and runs `reconcileE2EERotationTx` per affected group in the same transaction (`e2ee_routing.go` diff), with a crash-checkpoint before commit. Test asserts revoked artifacts + `required`/`device_revoke` rotation. No credential resurrection: rotation reuses existing requirement machinery; old device stays `revoked_at != 0` and fails `authorizedE2EERecoveryMemberTx`.
4. **Coordinator secrecy** — NO new seam. Only opaque `encrypted_package`/`encrypted_grant` blobs + digests are stored; binding tables carry lineage integers/digests only; validator enforces no plaintext/media_key/recovery_secret columns, no log statements in the three repositories, and no runtime wiring of consume/authorize into any entrypoint. Checkpoint-injected rollback test shows no partial artifacts survive a crash before commit.
5. **Platform reset & cleanup** — YES, fail-closed on both platforms. Reset deletes only the three fixed identity slots (record before witness), validates `expectedDeviceID` when slots are fully present, and does not relabel old installation-bound state — group/grant/cache records stay bound to the old random installation ID and are unopenable by the replacement identity (`requireInstallation` + per-record installationID checks). Expired-grant cleanup is caller-enumerated, deduplicated, capped at 100, retains active grants, and stops on corrupt records. Partial-install → rollbackOrClone → reset → re-enroll is tested on both platforms.
6. **Consistency** — YES. Immutability triggers freeze all binding columns except `consumed_at` and read counters/timestamps; expiry sweep bounded to 1000/tx; shared policy vectors (`protocol/e2ee-recovery-v1-vectors.json`, production-disabled) are asserted by coordinator, macOS, and Windows tests; legacy unbound `CreateE2EEHistoryGrant`/`CreateE2EETransferPackage` entrypoints now delegate to the authorized paths and fail closed (tested); acceptance pins match; manual evidence honestly `not-run` and deferred to EPIC-260714-th54l3; production gates EPC-001/002/004/005 + TASK-260712-1ulshp remain open.

## Findings

- **Low (non-blocking, code quality)**: In `RevokeAuthorizedE2EETransferPackage` (`coordinator/internal/store/e2ee_recovery.go:322`) and `RevokeAuthorizedE2EEHistoryGrant` (`:618`), the status/ownership check runs before the `err != nil` check on the scanned row, so a genuine non-ErrNoRows DB error is misreported as `ErrE2EE*Unavailable` instead of surfacing the internal error. Behavior stays fail-closed; only error classification is off. Suggest reordering in a later cleanup pass.
- **Info**: A fully-present but corrupt/clone-flagged identity cannot be reset via `resetDeviceIdentityForReenrollment` (validation throws before deletion). Deliberate stop-the-line semantics consistent with the ADR; real-device recovery UX is deferred to the manual epic.
- **Info**: `e2ee_history_grants` has no expiry index (packages got `e2ee_transfer_packages_expiry`); the bounded sweep scans by status+expires_at. Negligible at expected volumes.

No Critical/High/Medium findings. Real hardware, native Keychain/DPAPI interop, signed packages, production crypto, and real-app UX are correctly treated as deferred to EPIC-260714-th54l3, not as satisfied evidence.
