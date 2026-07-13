# TASK-260712-38qsku — identity rollout and rollback runbook

Date: 2026-07-14 (Asia/Tbilisi)
Current coordinator: task bytes based on `c4951968ee5e5dc40a985bac3e8684befd019343`
Pinned rollback target: `e8bd240664a40b9cc78b974f3c34ad30712e2aa5`

## Safety contract

The schema migration is additive, but rollback is not an unconditional binary
downgrade. Before the previous coordinator starts, the current binary must
project security-relevant additive state into the legacy tables. The supported
rollback order is therefore:

`stop ingress and the coordinator -> back up -> project -> verify -> start the pinned predecessor`.

Keep `coordinator.yml` predecessor-neutral while that revision remains a
rollback target. Enable the current feature only with
`DUET_SELF_SERVICE_ONBOARDING=1`. The old binary uses strict YAML field
decoding and rejects `self_service_onboarding:` in YAML, although it safely
ignores the environment variable.

Never paste a node token, control token, invite/link code, recovery secret,
Telegram token, database copy, or request body into logs, tickets, or shell
arguments. Store backups with mode `0600` and the same access boundary as the
live database.

## Preflight and hold conditions

1. Freeze identity/onboarding writes and record the exact current and rollback
   binary hashes. The rollback binary must correspond to the pinned revision
   above; a merely similar version is not accepted evidence.
2. Confirm the deployed YAML contains no current-only fields. The one-shot
   projection command performs a strict pinned-predecessor decode before it
   opens or mutates the database and aborts on an incompatible source.
3. Take and verify a SQLite/Litestream backup. For the native deployment, after
   stopping the coordinator:

   ```bash
   sudo systemctl stop duet-coordinator
   sudo -u duet sqlite3 /var/lib/duet/duet.db \
     ".backup '/var/lib/duet/backup/pre-identity-change.db'"
   sudo -u duet sqlite3 /var/lib/duet/backup/pre-identity-change.db \
     'PRAGMA integrity_check; PRAGMA foreign_key_check;'
   ```

4. Hold rollout or rollback if `integrity_check` is not `ok`,
   `foreign_key_check` returns a row, the expected backup cannot be restored in
   a scratch location, or the current full/previous-head compatibility gates
   are not green on the exact release bytes.

## Staged rollout

1. Deploy the current coordinator with
   `DUET_SELF_SERVICE_ONBOARDING` empty or non-`1`. Do not put the key in YAML.
   Additive tables migrate while the new resolver and routes remain off.
2. Verify `/healthz`, normal legacy node reconnection, existing pair tokens,
   Telegram commands, and the startup field `self_service_onboarding=false`.
3. Run the database health and baseline queries below. Save counts, not row
   contents containing identity metadata.
4. Enable `DUET_SELF_SERVICE_ONBOARDING=1` for the internal/canary cohort and
   restart one single-writer coordinator instance.
5. Verify startup reports `self_service_onboarding=true`; existing nodes still
   reconnect; primary/companion/satellite roles and slots are unchanged; a
   control-only recovery reissue does not masquerade as a paired node; and new
   create, invite, recovery, and Telegram-link flows return `Cache-Control:
   no-store` without secrets in URLs or logs.
6. Expand only while the observability and manual gates below stay clean.

Container deployments receive the flag from `docker-compose.yml` and
`deploy/coolify.env.example`. Native systemd deployments receive it from
`/etc/duet/coordinator.env`, installed from
`deploy/coordinator.env.example`; `coordinator.yml` remains unchanged.

## Observability and manual gates

Use a read-only SQLite session. These queries must not print credential hashes
or recovery identifiers.

```sql
PRAGMA integrity_check;
PRAGMA foreign_key_check;

SELECT status, COUNT(*) FROM orbits GROUP BY status ORDER BY status;

SELECT limiter_class, COUNT(*)
FROM rate_limit_audit_events
WHERE created_at >= (unixepoch('now', '-15 minutes') * 1000)
GROUP BY limiter_class ORDER BY limiter_class;

SELECT type, COUNT(*)
FROM audit_events
WHERE created_at >= (unixepoch('now', '-15 minutes') * 1000)
GROUP BY type ORDER BY type;

SELECT COUNT(*) AS quarantined_last_15m
FROM audit_events
WHERE type = 'identity.alignment_quarantined'
  AND created_at >= (unixepoch('now', '-15 minutes') * 1000);
```

Hold expansion on any alignment quarantine, migration/reconciliation failure,
unexpected 5xx, unexplained 401/403 increase, sustained 429 increase outside a
known test, role/slot drift, lost legacy token, client protected-store error,
or recovery generation conflict. A 429 is valid only when its durable
`security.rate_limited` audit row was committed.

Manual compatibility checks, using normal clients so secrets never enter shell
history:

- an already paired macOS and Windows Pulsar reconnects before and after the
  flag change with the same node identity;
- primary and companion control actions succeed; satellite and every node
  token remain forbidden from administration;
- recovery rotates once, displays once, and the reissued control credential is
  stored as control-only until an independently valid node credential exists;
- replaying an invite, link code, or mismatched recovery tuple yields the same
  generic credential failure and creates no extra actor, slot, or audit success;
- client diagnostics, clipboard/pasteboard status, protected-store errors, HTTP
  URLs, proxy logs, and coordinator logs contain no plaintext credential.

## Planned rollback to the pinned predecessor

1. Announce a write freeze. Remove external ingress and stop the current
   coordinator. Merely setting the feature flag to off is not the rollback
   procedure.
2. Take a new verified backup as described in preflight.
3. Keep the current binary available and run its one-shot projection while it
   is the only writer:

   ```bash
   sudo -u duet /usr/local/bin/duet-coordinator \
     --config /etc/duet/coordinator.yml \
     --project-identity-rollback
   ```

   For Compose:

   ```bash
   docker compose stop coordinator
   docker compose run --rm --no-deps coordinator -project-identity-rollback
   ```

   Success prints only `identity rollback projection complete`. The operation
   is atomic and idempotent. It opens the store with the feature disabled, so a
   retry cannot restore its own pending projection.
4. Before starting the old binary, every query below must return `0`; then run
   both PRAGMAs again:

   ```sql
   SELECT COUNT(*)
   FROM orbits o
   LEFT JOIN rollback_projections rp ON rp.orbit_id = o.id
   WHERE o.status = 'disabled'
     AND (o.max_pulsars <> 0 OR o.max_members <> 0
          OR rp.orbit_id IS NULL OR rp.restored_at IS NOT NULL);

   SELECT COUNT(*)
   FROM slots s JOIN orbits o ON o.id = s.orbit_id
   WHERE o.status = 'disabled' AND s.revoked_at IS NULL;

   SELECT COUNT(*)
   FROM installation_credentials ic
   JOIN actors a ON a.id = ic.actor_id
   JOIN slots s ON s.orbit_id = ic.slot_orbit_id AND s.slot = ic.slot_name
   WHERE a.revoked_at IS NOT NULL AND s.revoked_at IS NULL;

   SELECT COUNT(*)
   FROM invites i JOIN orbits o ON o.id = i.orbit_id
   WHERE o.status = 'disabled' AND i.used_at IS NULL;

   SELECT COUNT(*)
   FROM device_invites i JOIN orbits o ON o.id = i.orbit_id
   WHERE o.status = 'disabled' AND i.consumed_at IS NULL;

   SELECT COUNT(*)
   FROM telegram_link_codes l JOIN orbits o ON o.id = l.orbit_id
   WHERE o.status = 'disabled'
     AND l.consumed_at IS NULL AND l.invalidated_at IS NULL;

   PRAGMA integrity_check;
   PRAGMA foreign_key_check;
   ```

5. Unset `DUET_SELF_SERVICE_ONBOARDING`, deploy the exact pinned predecessor,
   and keep ingress closed while verifying disabled-orbit tokens fail and its
   real `PairSlot`/`AddMember` surfaces are blocked by the projected legacy
   quotas. The tagged integration gate exercises those exact predecessor APIs;
   do not substitute hand-written SQL emulation as release evidence.
6. Reopen ingress only after the old binary passes health, legacy node, role,
   invite, and disabled-orbit checks.

## Return to the current coordinator

Stop ingress and the predecessor, back up its final database, deploy the current
binary, and enable `DUET_SELF_SERVICE_ONBOARDING=1`. Reconciliation imports the
old interval and restores `max_pulsars`/`max_members` from each pending journal
row. Verify:

```sql
SELECT COUNT(*) FROM rollback_projections WHERE restored_at IS NULL;
PRAGMA integrity_check;
PRAGMA foreign_key_check;
```

The first query must be `0`. Projection slot revocations are deliberately
one-way: a projected slot remains revoked after re-enable and must be explicitly
re-paired/re-provisioned. Do not restore a slot token or synthesize credential
material from a backup.

## Emergency rollback

If projection cannot be completed, keep ingress closed and affected tenants
offline. Starting the predecessor without projection is not safe: its legacy
`PairSlot` path can mint authority that does not understand current disabled
state. Restore the verified pre-change backup to a scratch environment for
diagnosis, or repair the current projection path; never count an unprojected
old-binary boot as a successful rollback.

## Automated release gates

From repository root, the accepted release bytes must pass:

```bash
cd coordinator
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
go test -tags previoushead -count=1 ./internal/store \
  -run '^TestR8ExactPreviousHEAD(AuthorityRoundTrip|TwoGenerationProjectionComposition|ConfigBootstrapContract)$'

cd ../node-app
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test

cd ../pulsar-win
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
```

The tagged coordinator test requires full Git history because it extracts and
runs the exact pinned predecessor source.
