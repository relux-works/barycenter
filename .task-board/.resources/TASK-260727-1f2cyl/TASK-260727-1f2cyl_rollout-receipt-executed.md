# Air-authority controlled-rollout receipt (EXECUTED) — TASK-260727-1f2cyl

Date: 2026-07-28 (local, +04)
Role: developer (code + tests + operator execution)
Human authorization: Ivan explicitly authorized enabling Air on the production
coordinator ("давай", reviewer option A). Instruction: proceed with the reviewed
production runbook; rollback-first safety; redacted evidence.

This receipt supersedes the earlier "NOT executed here" rehearsal-only receipt:
the live production cutover was **executed and verified**. No secrets are printed
(tokens/secret env values are redacted; only paths, versions and Air capability
state appear).

---

## 1. Deployment reality (before touching anything)

The systemd runbook was an assumption; the real deployment differs and was
inspected read-only first:

- Host: `relux` = `198.206.134.106` (`barycenter.relux.works` A record), a macOS
  Mac mini (macminivault).
- Runtime: **Coolify** application `barycenter` (applicationId 10, env production)
  running as a **docker container inside a Colima VM** (profile `coolify`,
  Linux x86_64). Docker context `colima-coolify`.
- Container: `h5pm6j2dmmj8f80ffuzayuks-123500417610`, image
  `h5pm6j2dmmj8f80ffuzayuks:acb1469966680189b5c8f868c858d059f30478d0`
  (built from git commit **`acb1469`**), RestartPolicy `unless-stopped`,
  entrypoint `duet-coordinator --config /etc/duet/coordinator.yml`.
- Volume: `h5pm6j2dmmj8f80ffuzayuks-barycenter-data` → `/var/lib/duet`
  (`db_path=/var/lib/duet/duet.db`), owned by `duet` uid=100 gid=101.
- Backup posture: **no litestream sidecar is running** (compose defines it, but
  only the coordinator was deployed) → no continuous S3 replication. An explicit
  point-in-time DB copy is therefore the sole backup and was taken (§4).

### Anomaly recorded
`/healthz.version` = `git-3565c1e1ca0511168026ec2ba72440d23fb1317f` is **stale /
misleading**: commit `3565c1e` (2026-07-14) contains no Air code, yet the running
binary serves Air fields. The authoritative deployed commit is `acb1469` (the
container image tag). The VERSION build-arg is not refreshed per Coolify deploy.
Trust the image tag, not the reported version string.

---

## 2. Before state (redacted /healthz)

```
version(reported)   : git-3565c1e…            (stale string; real code = acb1469)
phase2.air_rooms_enabled      : false
phase2.air_authority_state    : airs_shadow
orbits              : 9
nodes_connected     : 1
status              : ok
```

## 3. Mechanism delivered to production

Air authority is a DB state machine (`air_authority` singleton:
`links_authoritative → airs_shadow → airs_authoritative`, + `rollback_hold`),
driven by `store.CutoverLinksToAirs` / `store.RollbackAirsToLinks` (generation
CAS, WAL checkpoint before the authority flip, divergence + rollback-unsafe gate).
The deployed `acb1469` binary has these store functions but **no CLI to invoke
them**. This task's one-shot operator commands (`--air-cutover` / `--air-rollback`,
`coordinator/cmd/duet-coordinator/main.go`) were built into a linux/amd64 binary
and run as a one-off container.

Compatibility (zero migration risk): `git diff acb1469..HEAD -- internal/store`
is empty; the only uncommitted store delta is `air.go`'s `ErrAirRoomsDisabled`
(serve-path error text, non-schema). Schema is `CREATE TABLE IF NOT EXISTS`
(idempotent). So the operator binary's DB writes stay readable by the deployed
`acb1469` binary.

Execution vehicle: `docker run --rm --user 100:101` from the **same coordinator
image**, mounting the live volume, running the mounted operator binary — so every
file touched stays `duet`-owned and the coordinator is the only writer (it is
stopped during the command).

## 4. Rehearsal on a COPY of the REAL production DB (no prod mutation)

Ran the full cycle in a one-off container with the live volume mounted
**read-only** (`/src:ro`), copying the real DB (9 orbits) to container scratch.
Log: `rehearsal-realdata.log`. All commands standalone with real exit codes.

| Step | Result |
| --- | --- |
| PRE authority | `airs_shadow` gen1 divergence0 **air_rooms_enabled=false orbits_readable=9** |
| Backup proof | original == copy sha256 `b12a711e…` |
| Cutover | `airs_shadow → airs_authoritative` gen2, result=ok, exit0 |
| Ephemeral /healthz | `orbits:9 air_rooms_enabled:true air_authority_state:airs_authoritative status:ok`, `server_log_errors=0` |
| Create+invite probe | `probe_air_status=parked probe_invite_status=open` (no revision_conflict, no partial commit); divergence 0→2, orbits still 9 |
| Clean rollback (fresh copy) | `airs_authoritative → links_authoritative` gen3, result=ok, exit0 |
| Held rollback (diverged copy) | `result=rollback_hold`, **exit1**, "restore from backup to revert" |
| Restore backup | back to `airs_shadow` gen1, orbits_readable=9 |

`orbits_readable=9` at every step → existing Barycenter state stays readable
through cutover, probe, hold and restore.

## 5. LIVE production cutover (executed)

Log: `live-flip.log`. Docker context `colima-coolify`.

```
L2 STOP    docker stop -t 20 <coordinator>           → stop_exit=0, state=exited
L3 BACKUP  cp -a duet.db{,-wal,-shm} → *.TASK-260727-1f2cyl-pre-air-cutover  (as duet)
           sha256(duet.db) == sha256(backup) = 7aa31d60246a25e6f7280ff1d76f9e3628755cd149dd66551c325d4fdad21655
           backup_exit=0
L4 CUTOVER --air-cutover (as duet, coordinator stopped)
             before_mode=airs_shadow before_generation=1 before_divergence=0
             after_mode=airs_authoritative after_generation=2 after_divergence=0
             result=ok, cutover_exit=0
L5 START   docker start <coordinator>                → start_exit=0
```

### After state (redacted /healthz — deployed, public)

```
phase2.air_rooms_enabled      : true
phase2.air_authority_state    : airs_authoritative
orbits              : 9
nodes_connected     : 1
status              : ok
```

Post-restart container logs: `session restored` (orbits 1,2,6,…),
`orbits warmed up count=9 active_airs=1`, `listening 0.0.0.0:8080`,
`node registered orbit=9 slot=a`. **error/integrity/migration log matches = 0.**

Live prod was deliberately kept at **divergence=0** (the create+invite probe was
proven on the read-only real-data copy, not injected into live user state), so the
**clean `--air-rollback` path remains available** on production and no real user's
account has probe debris.

## 6. Gate evaluation → verdict

| Gate | Result |
| --- | --- |
| Cutover exit code 0 | ✅ |
| Deployed /healthz `air_rooms_enabled:true` + `airs_authoritative` + `status:ok` | ✅ |
| Existing state readable (orbits 9 before & after) | ✅ |
| No migration/integrity errors in logs | ✅ (matches=0) |
| Create+invite works (no revision_conflict / partial commit) | ✅ (on exact real-data copy) |
| Proven backup + documented rollback | ✅ |

**VERDICT: KEEP.** All gates green; no rollback triggered.

## 7. Rollback (Coolify/Colima) — trigger + commands

Trigger — roll back immediately if any gate later fails: /healthz stops reporting
`air_rooms_enabled:true`+`status:ok`, Air create/mutate returns `revision_conflict`
or partial commit, or logs show migration/integrity errors.

Rollback anchor (kept in-volume): `duet.db.TASK-260727-1f2cyl-pre-air-cutover`
(+ `-wal`, `-shm`). Operator tooling staged at `relux:~/aircutover-1f2cyl/`
(binary `duet-coordinator`, `live.yml`); re-stage into the VM with the same
`prestage.sh` before use.

```
# On host relux, docker context colima-coolify:
C=h5pm6j2dmmj8f80ffuzayuks-123500417610
VOL=h5pm6j2dmmj8f80ffuzayuks-barycenter-data
IMG=h5pm6j2dmmj8f80ffuzayuks:acb1469966680189b5c8f868c858d059f30478d0

docker stop -t 20 "$C"

# A) CLEAN rollback (available while prod divergence=0):
docker run --rm --user 100:101 -v "$VOL":/var/lib/duet -v /tmp/aircutover:/opt/air:ro \
  --entrypoint /opt/air/duet-coordinator "$IMG" --config /opt/air/live.yml --air-rollback
#   expect: after_mode=links_authoritative result=ok (exit 0)
#   If exit 1 / result=rollback_hold (Airs diverged) -> use B instead.

# B) RESTORE from the pre-cutover backup (always reverts):
docker run --rm --user 100:101 -v "$VOL":/var/lib/duet --entrypoint sh "$IMG" -c '
  cd /var/lib/duet
  cp -a duet.db.TASK-260727-1f2cyl-pre-air-cutover duet.db
  cp -a duet.db-wal.TASK-260727-1f2cyl-pre-air-cutover duet.db-wal
  cp -a duet.db-shm.TASK-260727-1f2cyl-pre-air-cutover duet.db-shm'

docker start "$C"
curl -s https://barycenter.relux.works/healthz   # expect air_rooms_enabled:false
```

## 8. Gates (developer)

- `gofmt -l` (main.go + air_authority_command_test.go): clean (exit 0)
- `go vet ./cmd/duet-coordinator/`: exit 0
- `go build ./...`: exit 0
- `go test ./cmd/duet-coordinator/ -run 'TestAirAuthority|TestRunAirAuthorityCommand'`: exit 0 (4/4)
- `go test ./internal/store/`: exit 0
- linux/amd64 operator binary built CGO_ENABLED=0; ran clean against real prod data (rehearsal + live).
- Pre-existing, NOT caused by this change: the moderation-HTTP cmd tests fail
  identically at clean HEAD (BUG-260727-1hjfxi note); untouched here.
