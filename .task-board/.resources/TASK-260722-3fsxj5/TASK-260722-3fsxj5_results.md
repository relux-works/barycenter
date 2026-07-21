# TASK-260722-3fsxj5 — live self-service onboarding rollout evidence

Date: 2026-07-22 (Asia/Tbilisi)
Role: developer / live rollout operator
Disposition: ready for independent review; this document contains no tokens, credentials, request bodies, or database contents.

## Result

The live Barycenter coordinator now runs the reviewed self-service onboarding release through its authoritative Coolify deployment path.

- Public endpoint: `https://barycenter.relux.works`
- Health: HTTP `200`, `status=ok`
- Reported version: `git-3565c1e1ca0511168026ec2ba72440d23fb1317f`
- Orbits: `3` before and `3` after
- Connected nodes: `0` before and `0` after
- Telegram: enabled before and after; final startup log records `telegram_enabled=true`
- Self-service flag: `DUET_SELF_SERVICE_ONBOARDING=1`, production/runtime-only
- Route probes after the final clean restart: Create `400`, device-invite `400`, consume `400`, unknown control path `404`
- Final database: integrity `ok`, no foreign-key rows, exactly the reviewed 21-table/schema-object set, and no durable probe identities/audits

No product source was changed by this operational task. The already-reviewed release source was built and tested at its exact commit; the repository change made here is the task logbook entry.

## Authoritative runtime and provenance

The Barycenter coordinator is **not** owned by `relux-remote-infra`. It is managed by Coolify on the live macOS host:

| Field | Exact value |
|---|---|
| Docker context | `colima-coolify` |
| Coolify application ID / UUID | `10` / `h5pm6j2dmmj8f80ffuzayuks` |
| Project / environment / service | `barycenter` / `production` / `barycenter` |
| Persistent volume | `h5pm6j2dmmj8f80ffuzayuks-barycenter-data` → `/var/lib/duet` |
| Repository | `relux-works/barycenter` |
| Durable source branch | `task/task-260712-38qsku-auth-migration-rollback` |
| Verified remote branch head | `3565c1e1ca0511168026ec2ba72440d23fb1317f` |
| Stored `git_commit_sha` | `3565c1e1ca0511168026ec2ba72440d23fb1317f` |
| Final container ID | `3c032b2923548537cf4715cd847074c2f4f2719b6bef9c627ec3a458b2446e6c` |
| Final image ref | `h5pm6j2dmmj8f80ffuzayuks:3565c1e1ca0511168026ec2ba72440d23fb1317f` |
| Final image ID/digest | `sha256:c6869bd0ef83ae05ff0232abaa2d716d772050441a91a5be0f0a6e0895c6e49d` |
| Final Compose config hash | `1d472d2ad693ff76ee142e2d6eee11e581292ea2c04092d574fd5a9a95edeade` |
| Final start time | `2026-07-21T22:08:38.981930805Z` |
| Binary SHA-256 | `d3484c6a5074c9d1c2ab4c51b220b7e0c4a47dff662085799d1228fa55ad5aae` |
| `/etc/duet/coordinator.yml` SHA-256 | `3f85e681daadaf98acb7a7ec074da2e2358d9adb7d40ef6b821fdf36a4b42cdf` |

The production `VERSION` variable is build-time-only and equals `git-3565c1e1ca0511168026ec2ba72440d23fb1317f`. The production onboarding flag is runtime-only and equals `1`. There is no preview onboarding flag.

## Pre-change state

| Field | Exact value |
|---|---|
| Container ID | `6421d9e796fd01961f4c495bba6479043e569d5360e8df4565e0e03298a45765` |
| Image ref | `h5pm6j2dmmj8f80ffuzayuks:431a04154f163c53f6b9cd06627fb20aa34fdee5` |
| Image ID | `sha256:854b0622b3be145cc4a8d4b530852711c7b8ad73c241e9f5ae653af69693c287` |
| Compose config hash | `cb70d36ebd7aafd8a7dc51e389bbd8e4a63f1d132b1cfb27011248093b5cc901` |
| Health | HTTP `200`; version `v0.3.0-beta.26`; orbits `3`; nodes `0` |
| Database | 12 legacy tables; 3 orbits, 3 members, 11 slots, 17 legacy invites |
| Flag | absent |
| Binary SHA-256 | `da4f80cadd5fdf31d1377d46b02e26e97faa3c893facf697f01c29935587821f` |
| Config SHA-256 | `44451491bb3279ecc2b283a5387dc1b1138dddbf9869b55f7f75478ad9674cac` |

The pre-change binary accepted only `-config` and `-version`; it had neither the onboarding routes nor `--project-identity-rollback`. A flag-only restart was therefore not a valid rollout.

The accepted candidate is commit `3565c1e1ca0511168026ec2ba72440d23fb1317f`. Relative to the coordinator bytes in the pre-change deployment, it contains the reviewed identity/onboarding implementation plus rollback reproducibility. The pinned predecessor is `e8bd240664a40b9cc78b974f3c34ad30712e2aa5`. This gate passed with exit `0`:

```bash
git diff --quiet e8bd240664a40b9cc78b974f3c34ad30712e2aa5 \
  431a04154f163c53f6b9cd06627fb20aa34fdee5 \
  -- coordinator deploy/coordinator.container.yml coordinator/Dockerfile
```

## Verified backups

Sensitive assets are intentionally **not attached to the board**. They are stored under:

`/Users/administrator/.local/state/barycenter-rollouts/TASK-260722-3fsxj5/prechange-20260721T212615Z`

The directory is mode `0700`; database/config/image files below are mode `0600`.

| Asset | Bytes | SHA-256 | Purpose |
|---|---:|---|---|
| `duet-prechange.db` | 159744 | `54db4c6c7be685e8df5385d803c09eba63a2ce910b6297ba1658fe27f062f66d` | Authoritative pre-change SQLite backup |
| `restore-rehearsal.db` | 159744 | `f2d2c460fc87fe876b2ffb888929687adf8b3ac5096ee9103ca9599895d21982` | Scratch `.restore` proof |
| `coolify-app-config.json` | 13350 | `a8d8d95a527b30cb513783b985b4ce419d78756432276de723f87ec3b368e780` | Pre-change Coolify config backup |
| `live-container-inspect.json` | 11960 | `58cf744511f91e9b5447e7f1dd121980a791c3c081e2d2207609e1a4afd4ba4d` | Pre-change full container inspect |
| `coolify-api-application.json` | 9427 | `3a66ccf0f6b4119fc389d84c4786b5738aa7162e992a0497e1e8dc3cfb7639ed` | Pre-change decrypted application export |
| `coolify-api-environment.json` | 5761 | `40a2096e814ee67a1ed92a848e4d5ac2b9e137b364582f4e3980e4feb46a7f77` | Pre-change decrypted env export |
| `pre-clean-restore-offline.db` | 1855488 | `a4c02b2d3d179b3831a7fd1d3fab0bdc7b4967e6c0b1a15ec2e852cb5ad7a33b` | Frozen backup immediately before clean restore |
| `post-clean-probes.db` | 241664 | `36d58e520cf7632f82a548a75e9df0a3f6788f12a764b6743fd50d6d0173d81e` | Final online backup after GET probes |
| `coolify-api-application-post-rollout.json` | 9507 | `fc1ba279d400fd24e7dc54e2823ef1c99b3194e8224c023490824002a7e5bc58` | Post-rollout app config |
| `coolify-api-environment-post-rollout.json` | 6366 | `e73867c0a5578a84495d73ad6f53a6a655f0cbb4be16f0b2f74ce85a54d51869` | Post-rollout env config |
| `live-container-inspect-post-clean.json` | 12053 | `7fb9f69a1c4f97804b0b4f00e63a270db06b3d3d4a0f3fbb08f2df241dda15c5` | Final full container inspect |
| `pinned-predecessor-e8bd240-image.tar` | 61655552 | `f85c29cf0124a24b6bafeed05d80b6d583b46dbacbc1869d463005cec18aa489` | Reload-tested predecessor OCI archive |

Backup gates:

- Live SQLite `.backup`: exit `0`.
- `PRAGMA integrity_check`: exit `0`, result `ok`.
- `PRAGMA foreign_key_check`: exit `0`, zero rows.
- Scratch `.restore`: exit `0`.
- Restored integrity and foreign-key gates: exit `0`.
- Config/inspect JSON parsing and exact-image/mount/no-prechange-flag gates: exit `0`.
- Final stopped-state comparison: exit `0`; every original column and row in all 12 legacy tables was identical to `duet-prechange.db`.
- Actual clean restore: restored active file SHA-256 equaled `54db4c…`, then the exact image migrated it to the reviewed schema.

The prior migrated database was also retained inside the volume as mode-0600 `duet.db.TASK-260722-3fsxj5-pre-clean-restore`; the off-volume `pre-clean-restore-offline.db` has the same logical state.

## Release validation

All release commands ran in a detached worktree at exact commit `3565c1e1ca0511168026ec2ba72440d23fb1317f`.

| Command / gate | Exit |
|---|---:|
| gofmt cleanliness | 0 |
| `go vet ./...` | 0 |
| `go test -count=1 ./...` | 0 |
| `go test -race -count=1 ./...` | 0 |
| `go build ./...` | 0 |
| `go test -tags previoushead -count=1 ./internal/store -run '^TestR8ExactPreviousHEAD(AuthorityRoundTrip\|TwoGenerationProjectionComposition\|ConfigBootstrapContract)$'` | 0 |
| Candidate Docker build with exact VERSION | 0 |
| Isolated restored-production migration/health/routes/counts/integrity | 0 |
| Six pre-rollback unsafe-state queries | 0; each result was `0` |

The exact candidate migration produced 3 active orbits, 14 actors, 14 memberships, 11 installation credentials, and zero device invites, Telegram link codes, or rollback projections while preserving the 3/3/11/17 legacy counts.

## Coolify deployment incident and recovery

Coolify accepts a stored `git_commit_sha`, but its installed ordinary deployment job performs `git ls-remote refs/heads/<git_branch>` and overwrites the queued commit. Pinning only `git_commit_sha` while leaving `git_branch=main` was therefore insufficient.

| Deployment UUID | Queue commit | Result |
|---|---|---|
| `eyd6dpo5znlmjqyo8d59fgx7` | `959fba59c562caa7a3dfd1176c1feda7f4207fe8` | Unintended current-main build; started 21:44:25Z |
| `lxk96g1er6u8329oegnxn5rb` | `3565c1e1ca0511168026ec2ba72440d23fb1317f` | Corrected exact deployment; exact container started 21:51:13Z |
| `n13wyplrevyhy96r22tcemcj` | `3565c1e1ca0511168026ec2ba72440d23fb1317f` | Explicit durability restart |
| `ovjq9etjsw76ropcpooncsek` | `3565c1e1ca0511168026ec2ba72440d23fb1317f` | Final restart after clean database restore |

The unintended image was not accepted as equivalent: an independent diff found 265 runtime-relevant files changed, and its binary/config hashes differed. It temporarily added 111 later schema tables. Recovery was:

1. Pin `git_branch` to the reviewed branch and verify its remote head is exact.
2. Deploy the exact candidate and verify it can read the temporarily advanced schema.
3. Stop the application to freeze writes.
4. Take and validate `pre-clean-restore-offline.db`.
5. Prove every pre-existing legacy row/original column still exactly equals the pre-change backup and all onboarding/audit mutation tables are empty.
6. Restore the verified pre-change database.
7. Restart only the exact reviewed image through Coolify.
8. Verify the final live schema exactly equals the 21-object preflight schema (`0|0` bidirectional `sqlite_master` difference), with no later tables left.

The erroneous image tag `959fba59…` was removed after recovery so it cannot appear as a rollback choice. It is reproducible from Git if forensic reconstruction is ever required.

## Final live gates

All commands below exited `0` unless explicitly noted otherwise.

| Gate | Observed result |
|---|---|
| Health | HTTP `200`; `status=ok`; exact version; `orbits=3`; `nodes_connected=0` |
| Runtime provenance | exact image ref, image ID, source commit, and flag in final container inspect |
| Startup | exact version, `telegram_enabled=true`, `self_service_onboarding=true`, 3 warmed orbits, 1 link, listening |
| Log scan | no error/warn migration, startup failure, panic, or fatal lines |
| Create GET `/v1/onboarding/orbits` | `400`, registered validation path |
| Device-invite GET `/v1/device-invites` | `400`, registered validation path |
| Consume GET `/v1/device-invites/consume` | `400`, registered validation path |
| Unknown GET `/v1/not-a-route-task-260722-3fsxj5` | `404`, negative control |
| Final DB integrity / FK | `ok` / zero rows |
| Schema equality | 21 live vs 21 candidate tables; zero extra/missing; zero `sqlite_master` differences |
| Legacy counts | orbits 3, active orbits 3, members 3, slots 11, invites 17 |
| Identity baseline | actors 14, memberships 14, installation credentials 11 |
| Mutation absence | device invites 0, link codes 0, audit events 0, recovery details 0, rate-limit audits 0, rollback projections 0 |
| Probe non-mutation | pre-probe and post-probe online backup hashes both `36d58e52…` |

No POST, PUT, PATCH, invite-consume, account-create, or credential mutation was used as a live route probe.

## Executable rollback posture

Pinned predecessor:

| Field | Value |
|---|---|
| Revision | `e8bd240664a40b9cc78b974f3c34ad30712e2aa5` |
| Preserved alias | `barycenter-rollback:TASK-260722-3fsxj5-e8bd240` |
| Coolify-compatible tag | `h5pm6j2dmmj8f80ffuzayuks:e8bd240664a40b9cc78b974f3c34ad30712e2aa5` |
| Image ID/digest | `sha256:4c23a2f199dea6e71a7b32f8162fc9fbb03b1355a0d4cca3abb32db957a02294` |
| Binary SHA-256 | `da4f80cadd5fdf31d1377d46b02e26e97faa3c893facf697f01c29935587821f` |
| Config SHA-256 | `44451491bb3279ecc2b283a5387dc1b1138dddbf9869b55f7f75478ad9674cac` |
| Version | `v0.3.0-beta.26` |

The rebuilt predecessor binary/config hashes are byte-for-byte identical to the pre-change container. The OCI archive was loaded with exit `0`. A fresh rehearsal on a copy of the final clean live database ran the projection, returned six zero safety results, passed integrity/FK, and booted this preserved predecessor healthy with 3 orbits.

Rollback is **not** merely switching the flag off. Use the following sequence with ingress unavailable while the application is stopped. Never place the API token or database content in logs or board evidence.

```bash
TASK_APP_UUID=h5pm6j2dmmj8f80ffuzayuks
TASK_VOLUME=h5pm6j2dmmj8f80ffuzayuks-barycenter-data
TASK_CURRENT_IMAGE=h5pm6j2dmmj8f80ffuzayuks:3565c1e1ca0511168026ec2ba72440d23fb1317f
TASK_PREDECESSOR_ALIAS=barycenter-rollback:TASK-260722-3fsxj5-e8bd240
TASK_PREDECESSOR_TAG=h5pm6j2dmmj8f80ffuzayuks:e8bd240664a40b9cc78b974f3c34ad30712e2aa5
TASK_BACKUP_DIR=/Users/administrator/.local/state/barycenter-rollouts/TASK-260722-3fsxj5/rollback-YYYYMMDDTHHMMSSZ
TASK_COOLIFY_TOKEN=$(<"$TASK_COOLIFY_TOKEN_FILE")

docker --context colima-coolify exec coolify curl \
  --silent --show-error --fail-with-body --request POST \
  --header "Authorization: Bearer $TASK_COOLIFY_TOKEN" \
  --header 'Accept: application/json' \
  "http://127.0.0.1:8080/api/v1/applications/$TASK_APP_UUID/stop"

for TASK_ATTEMPT in {1..30}; do
  test -z "$(docker --context colima-coolify ps --quiet \
    --filter label=coolify.applicationId=10)" && break
  sleep 1
done
test -z "$(docker --context colima-coolify ps --quiet \
  --filter label=coolify.applicationId=10)"

mkdir -p "$TASK_BACKUP_DIR"
chmod 700 "$TASK_BACKUP_DIR"
docker --context colima-coolify run --rm \
  -v "$TASK_VOLUME:/data" -v "$TASK_BACKUP_DIR:/backup" alpine:3.22 \
  sh -eu -c 'apk add --no-cache sqlite >/dev/null && sqlite3 /data/duet.db ".backup /backup/pre-rollback.db"'
chmod 600 "$TASK_BACKUP_DIR/pre-rollback.db"
sqlite3 "$TASK_BACKUP_DIR/pre-rollback.db" \
  'PRAGMA integrity_check; PRAGMA foreign_key_check;'

docker --context colima-coolify run --rm --network none \
  -v "$TASK_VOLUME:/var/lib/duet" "$TASK_CURRENT_IMAGE" \
  --project-identity-rollback
```

The projection must print only `identity rollback projection complete`. Run the six queries from `TASK-260712-38qsku_rollout-rollback-runbook.md`; every result must be `0`, then re-run both PRAGMAs. If any result is non-zero, keep the application stopped and do not start the predecessor.

Remove the production flag and restore the pre-change durable source/version configuration before queuing the old image:

```bash
set -o pipefail
TASK_FLAG_UUID=$(docker --context colima-coolify exec coolify curl \
  --silent --show-error --fail-with-body \
  --header "Authorization: Bearer $TASK_COOLIFY_TOKEN" \
  --header 'Accept: application/json' \
  "http://127.0.0.1:8080/api/v1/applications/$TASK_APP_UUID/envs" | \
  jq -r '.[] | select(.key=="DUET_SELF_SERVICE_ONBOARDING" and .is_preview==false) | .uuid')
test -n "$TASK_FLAG_UUID"

docker --context colima-coolify exec coolify curl \
  --silent --show-error --fail-with-body --request DELETE \
  --header "Authorization: Bearer $TASK_COOLIFY_TOKEN" \
  --header 'Accept: application/json' \
  "http://127.0.0.1:8080/api/v1/applications/$TASK_APP_UUID/envs/$TASK_FLAG_UUID"

docker --context colima-coolify exec coolify curl \
  --silent --show-error --fail-with-body --request PATCH \
  --header "Authorization: Bearer $TASK_COOLIFY_TOKEN" \
  --header 'Accept: application/json' \
  --header 'Content-Type: application/json' \
  --data '{"git_branch":"main","git_commit_sha":"HEAD"}' \
  "http://127.0.0.1:8080/api/v1/applications/$TASK_APP_UUID"

docker --context colima-coolify image inspect "$TASK_PREDECESSOR_ALIAS" >/dev/null
docker --context colima-coolify image tag \
  "$TASK_PREDECESSOR_ALIAS" "$TASK_PREDECESSOR_TAG"
```

Restore production `VERSION` to `v0.3.0-beta.26`, then queue the same rollback mechanism used by the installed Coolify UI:

```bash
docker --context colima-coolify exec coolify curl \
  --silent --show-error --fail-with-body --request PATCH \
  --header "Authorization: Bearer $TASK_COOLIFY_TOKEN" \
  --header 'Accept: application/json' \
  --header 'Content-Type: application/json' \
  --data '{"key":"VERSION","value":"v0.3.0-beta.26","is_preview":false,"is_buildtime":true,"is_runtime":false}' \
  "http://127.0.0.1:8080/api/v1/applications/$TASK_APP_UUID/envs"

docker --context colima-coolify exec coolify php artisan tinker --execute="\
\$app=App\\Models\\Application::where('uuid','h5pm6j2dmmj8f80ffuzayuks')->firstOrFail(); \
\$deploymentUuid=(new Visus\\Cuid2\\Cuid2)->toString(); \
\$result=queue_application_deployment(\
  application:\$app, deployment_uuid:\$deploymentUuid, \
  commit:'e8bd240664a40b9cc78b974f3c34ad30712e2aa5', \
  rollback:true, force_rebuild:false); \
echo \$deploymentUuid.'|'.\$result['status'].PHP_EOL;"
```

Keep ingress unavailable until the predecessor health reports `v0.3.0-beta.26`, three orbits are present, the database integrity/FK checks pass, and legacy node/role/invite behavior is verified. If projection cannot succeed, keep the application stopped; do not count an unprojected predecessor boot as rollback.

## Recovered red gates and operator errors

These failures are reported as failures; none is presented as green.

| Attempt | Real result | Correction |
|---|---|---|
| First route harness used zsh special variable `path` | exit `1`; Docker command lookup failed | renamed variable; route gate exit `0` |
| Base-file-only migrated DB diagnostic | exit `1`; missing migrated column because WAL was omitted | used SQLite online `.backup`; gates exit `0` |
| `sqlite3 -readonly` against WAL-era copy | exit `14`; unable to open database | used `PRAGMA query_only=ON` on valid backup; exit `0` |
| First temporary Coolify token creation | CLI exit `0` but embedded Laravel exception; no token row | set current team explicitly; token created and verified |
| First token verification predicate | exit `1`; compared varchar team ID to integer | corrected typed predicate; exit `0` |
| First safe application export assertion | exit `1`; API intentionally hid numeric ID | asserted UUID/owned fields; exit `0` |
| First source diff shell wrapper | exit `1`; zsh readonly `status` variable | renamed variable; exact diff gate exit `0` |
| First wrong-commit cancellation shell | outer shell exit `0`, embedded curl exit `52`, HTTP `000`; token file was container-local and deployment was already finished | treated deployment as active incident; redeployed exact branch and clean-restored DB |
| First candidate post-incident backup tried image-local `sqlite3` | exit `127`; binary absent | used ephemeral Alpine SQLite; exit `0` |
| First post-incident scratch boot | container exit `1`; scratch volume root prevented media-dir creation | fixed scratch ownership; candidate health exit `0` |
| Health probe against that failed scratch container | exit `7`; connection refused | reran after ownership fix; exit `0` |
| First branch PATCH payload | curl exit `22`, HTTP `400`; malformed JSON quoting | passed JSON directly to containerized curl; HTTP `200`, exit `0` |
| First live device-invite status assertion | exit `1`; expected `401`, observed registered validation `400` | asserted the actual non-mutating registered response `400`; exit `0` |
| Post-restart log assertion typo | exit `2`; misspelled temporary filename | corrected path; exit `0` |
| First projected-copy backup used read-only volume | exit `1`; SQLite could not open journal state | used stopped scratch volume read-write; exit `0` |
| First clean offline backup used `--network none` while installing SQLite | exit `1`; package unavailable | reran ephemeral tool with package network; exit `0` |
| First restored-live schema assertion had shell-quoting loss | exit `1`; SQL parsed `type=table` | corrected quoting; integrity/schema/count gate exit `0` |

Two `php artisan route:list` diagnostics also returned exit `1` because this Coolify version lacks `--columns` and `--compact`; the supported `--json` form returned exit `0`. These diagnostics did not mutate live state.

## Cleanup and security

- No production credential, API token, invite/link code, recovery secret, database row, or request body is present in this evidence.
- The one-hour task-scoped Coolify token is revoked during handoff cleanup; all token/response temporary files are deleted and token-row absence is verified.
- Disposable containers and volumes used for migration and rollback rehearsal were removed.
- The erroneous main image was removed; the exact live and exact predecessor images remain.
- The task logbook records the source-resolution anomaly, clean restore, final schema, and rollback proof.
