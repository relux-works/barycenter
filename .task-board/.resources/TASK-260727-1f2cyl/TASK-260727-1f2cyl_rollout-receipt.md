# Air-authority controlled-rollout receipt — TASK-260727-1f2cyl

Date: 2026-07-27
Role: developer (code + tests)
Branch: main

## Scope & honesty boundary

This task is written against "the deployed coordinator / production". Real
production is a **remote Coolify deployment** (see `docker-compose.yml`,
`deploy/duet-coordinator.service`) whose SQLite DB lives in the `duet-data`
volume at `/var/lib/duet`. That host is **not reachable from this environment**,
and flipping real production authority is an outward-facing, hard-to-reverse
action that requires an explicit human trigger. So this receipt does **not**
mutate real production.

Instead it delivers and **proves end-to-end** the reversible rollout mechanism
against a production-like store copy (two orbits joined by an active legacy
link, advanced to `airs_shadow` exactly as production reaches it), and documents
the exact human runbook + rollback trigger to run against real production.

## The gap this task closes

Air authority is a DB state machine in the `air_authority` singleton:
`links_authoritative → airs_shadow → airs_authoritative` (+ `rollback_hold`),
driven by `store.CutoverLinksToAirs` / `store.RollbackAirsToLinks` (generation
CAS, WAL checkpoint before every authority flip, divergence + rollback-unsafe
gates). Production is currently `airs_shadow` (health `air_rooms_enabled=false`),
per BUG-260727-1hjfxi.

Before this task **no operator surface wired those store functions to the
shipped binary** — there was no way to actually enable Air authority on a
deployed coordinator. This task adds two one-shot operator commands, mirroring
the existing `--project-identity-rollback` single-writer pattern:

- `duet-coordinator --config <cfg> --air-cutover`  → `airs_shadow → airs_authoritative`
- `duet-coordinator --config <cfg> --air-rollback` → `airs_authoritative → links_authoritative` (clean only)

Each opens the store with self-service serving disabled (so the normal
coordinator must be stopped and the command is the only writer), prints a
redacted parseable before/after receipt (mode, generation, divergence,
`air_rooms_enabled`), and exits. A rollback that cannot complete cleanly (Airs
already diverged from legacy links) leaves the store in `rollback_hold` and
exits non-zero, instructing the operator to restore from backup.

Code: `coordinator/cmd/duet-coordinator/main.go`
Tests: `coordinator/cmd/duet-coordinator/air_authority_command_test.go`

## Rehearsal evidence (production-like store copy)

Full log: `.temp/TASK-260727-1f2cyl/rehearsal.log`. All commands ran as
standalone processes with real exit codes.

| Step | Command | Result |
| --- | --- | --- |
| Identity / version | `--version` | `0.1.0-dev` |
| PRE-state | `authority duet.db` | `airs_shadow` gen=1 divergence=0 **air_rooms_enabled=false** orbits_readable=2 |
| Backup proof | `cp duet.db duet.db.backup` + sha256 | both `ec39b75b73a3fd15f4297c996b3c6a01251c7f5b9f1cf7cd6ee45bd8714a11a6` (identical) |
| **Cutover** | `--air-cutover` | before=`airs_shadow` gen1 → after=`airs_authoritative` gen2, `result=ok`, exit=0 |
| Health (`/healthz`, live server) | `curl /healthz` | `"air_rooms_enabled":true,"air_authority_state":"airs_authoritative"`, `"status":"ok"`, `"orbits":2` |
| POST-state | `authority duet.db` | `airs_authoritative` gen=2 divergence=0 **air_rooms_enabled=true** orbits_readable=2 |
| Create+invite probe | create Air + single-use invite | `probe_air_status=parked`, `probe_invite_status=open` — no revision-conflict, no partial commit |
| Clean rollback (fresh cutover copy) | `--air-cutover` then `--air-rollback` | `airs_authoritative` → `links_authoritative` gen3, `result=ok`, exit=0, air_rooms_enabled=false |
| **Safety** — rollback on diverged store | `--air-rollback` | `result=rollback_hold`, **exit=1**, "restore from backup to revert" |
| Reversibility | `cp duet.db.backup duet.db` | back to `airs_shadow` gen1, orbits_readable=2 |

`orbits_readable=2` at every step → existing Barycenter state stays readable
through cutover, rollback, hold and restore. No migration/integrity errors in
the server logs; `/healthz` returned `status:ok`.

Deployed `/healthz` phase2 after cutover (no secrets present in this payload):

```json
"phase2":{"ready":true,"streamed_tracks_enabled":false,"air_rooms_enabled":true,"air_authority_state":"airs_authoritative"}
```

## Production runbook (human trigger — NOT executed here)

Single-writer: the coordinator is one instance (SQLite `MaxOpenConns(1)`), so it
must be stopped for the command to be the only writer.

Enable (cutover):
```
# 0. Litestream sidecar already backs up continuously; confirm a fresh replica.
systemctl stop duet-coordinator                     # or: stop the Coolify service
cp /var/lib/duet/duet.db /var/lib/duet/duet.db.pre-air-cutover   # explicit point-in-time backup
duet-coordinator --config /etc/duet/coordinator.yml --air-cutover
#   expect: before_mode=airs_shadow ... after_mode=airs_authoritative ... result=ok
systemctl start duet-coordinator
curl -s http://127.0.0.1:8080/healthz | jq .phase2
#   expect: "air_rooms_enabled":true,"air_authority_state":"airs_authoritative"
```

Rollback trigger — any FAILED gate causes immediate rollback:
- cutover exits non-zero, or
- `/healthz` does not report `air_rooms_enabled:true` + `status:ok`, or
- a targeted create+invite probe returns `revision_conflict` / partial commit, or
- logs show migration/integrity errors.

Rollback:
```
systemctl stop duet-coordinator
duet-coordinator --config /etc/duet/coordinator.yml --air-rollback
#   clean:  after_mode=links_authoritative result=ok  (exit 0)
#   HELD:   result=rollback_hold (exit 1) -> Airs already diverged; do NOT force.
#           Restore the point-in-time backup instead:
#           cp /var/lib/duet/duet.db.pre-air-cutover /var/lib/duet/duet.db
systemctl start duet-coordinator
curl -s http://127.0.0.1:8080/healthz | jq .phase2   # expect air_rooms_enabled:false
```

Note: once a real user create/mutates an Air after cutover, divergence>0 and
`--air-rollback` intentionally HOLDS. That is the safe behavior — reverting then
is a restore-from-backup decision, which is why the pre-cutover copy above is
mandatory before enabling.

## Verdict

**KEEP-READY / GO, pending human execution against real production.**

The reversible enable path and its rollback are built, tested (4 new tests green)
and rehearsed end-to-end with an explicit, proven backup and two rollback paths
(clean command + backup restore). This task does not itself flip real production
(no reachable host + human-authorization boundary); an operator runs the runbook
above. If any listed gate fails during that execution, roll back immediately per
the trigger list.

## Gates

- `gofmt -l` (main.go, test): clean (exit 0)
- `go vet ./cmd/duet-coordinator/`: exit 0
- `go build ./...`: exit 0
- `go test ./cmd/duet-coordinator/ -run 'TestAirAuthority|TestRunAirAuthorityCommand'`: exit 0 (4/4 PASS)
- `go test ./internal/store/`: exit 0
- Pre-existing, NOT caused by this change: `TestModerationHTTPAuthPrivacyEvidenceAndDecision` and
  `TestModerationHTTPRevokedAndLeastPrivilegeOperatorsFailClosed` fail identically with this change
  reverted (verified via `git stash` of main.go + test file). Matches the BUG-260727-1hjfxi reviewer note.
