# Barycenter backups & restore

Why this exists: the production DB (`/var/lib/duet/duet.db` on the Coolify
persistent volume) holds **every orbit, member, pairing token-hash, queue and
session snapshot**. Before this, the Docker/Coolify path had no backup at all —
a lost or corrupted volume meant total, unrecoverable loss and every user having
to re-`/create` and re-`/pair`. This wires up continuous backups so we can
rebuild the DB onto a fresh volume.

## How it works

[Litestream](https://litestream.io) runs as a **sidecar** next to the
coordinator (`docker-compose.yml`). Both containers share the `duet-data`
volume, so Litestream sees the same `duet.db` the coordinator writes. Litestream
streams the SQLite WAL frames to S3-compatible object storage continuously and
takes periodic full snapshots (config: `deploy/litestream.yml` — snapshot every
6h, 7 days retained). A restore reconstructs the DB to any point within the
retention window.

**What it protects against:** a lost/corrupted Coolify volume, an accidental
`duet.db` deletion, or host loss — because the replica lives off-host in object
storage.

**What it does NOT protect against:** a logically bad write that Litestream
faithfully replicates (garbage in → garbage in the replica). For that, restore
to a point in time *before* the bad write, using the retention window.

## One-time setup

1. **Create a bucket** at any S3-compatible provider (AWS S3, Cloudflare R2,
   Backblaze B2, Hetzner Object Storage, MinIO, …). Create an access key scoped
   to that bucket.

2. **Set these env vars** in the Coolify app (or in a local `.env` next to
   `docker-compose.yml`). Values are yours to provide — see
   `deploy/coolify.env.example`. Nothing is invented here:

   | Var | Meaning |
   |-----|---------|
   | `LITESTREAM_BUCKET` | bucket name |
   | `LITESTREAM_PATH` | object path/prefix (default `duet.db`) |
   | `LITESTREAM_ENDPOINT` | S3-compatible endpoint URL; **empty for real AWS S3** |
   | `LITESTREAM_REGION` | region (AWS needs one; many others accept `auto`) |
   | `LITESTREAM_ACCESS_KEY_ID` | access key id |
   | `LITESTREAM_SECRET_ACCESS_KEY` | secret access key |

3. **Deploy.** The `litestream` service starts alongside `coordinator`.

4. **Verify** it is actually replicating (do this once, and after any infra
   change):

   ```sh
   # sidecar logs should show "replicating to ... name=s3"
   docker compose logs litestream | grep -i replicat

   # list what has landed in object storage
   docker compose exec litestream litestream snapshots /var/lib/duet/duet.db
   ```

   If you see no snapshots after ~10 min, the credentials/bucket/endpoint are
   wrong — Litestream logs the reason. **A silent no-op is the failure mode to
   watch for**: if `LITESTREAM_BUCKET` is empty the sidecar runs but backs up
   nothing.

## Restore (disaster recovery)

The volume is gone/corrupt and you have a fresh, empty `duet-data` volume.

1. **Stop the coordinator** so nothing writes while you restore:

   ```sh
   docker compose stop coordinator
   ```

2. **Restore the DB** from the replica into the shared volume (run from the
   litestream sidecar, which already has the config + credentials). `-if-db-not-exists`
   makes this a no-op if a DB is somehow already present, so you never clobber
   good data:

   ```sh
   docker compose run --rm litestream \
     litestream restore -if-db-not-exists -config /etc/litestream.yml \
     /var/lib/duet/duet.db
   ```

   To restore to a specific earlier point (e.g. before a bad write), add
   `-timestamp 2026-07-08T20:00:00Z`.

3. **Start the coordinator** and confirm it comes up:

   ```sh
   docker compose start coordinator
   docker compose logs -f coordinator     # look for "duet-coordinator starting"
   curl -fsS http://localhost:8080/healthz # orbits count should be non-zero
   ```

   On boot the coordinator demotes any PLAYING session to PAUSED; users resume
   with `/resume`. Pairings and orbits are intact.

## Manual on-demand snapshot (no external creds)

Litestream is the primary path. For a quick belt-and-suspenders copy before a
risky migration or deploy — and requiring no S3 — use SQLite's online backup
API against the live DB (safe, consistent, no downtime):

```sh
docker compose exec coordinator \
  sh -c 'apk add --no-cache sqlite >/dev/null 2>&1; \
         sqlite3 /var/lib/duet/duet.db ".backup /var/lib/duet/duet.pre-migration.db"'
```

That single-file snapshot lands on the same volume. Copy it off-host
(`docker compose cp`) if it must survive volume loss — for durable, automatic
off-host backups, that is exactly what Litestream already does.

## Notes

- **WAL mode is required.** Litestream enables WAL on the DB itself; because WAL
  is a persistent property of the SQLite file, the coordinator inherits it. Do
  not configure the coordinator to force rollback-journal mode or replication
  stops. (Adopting an explicit `?_journal_mode=WAL&_busy_timeout=5000` DSN in
  the coordinator — architecture review 1.7 — is complementary and recommended.)
- **Restore is manual by design.** The sidecar only replicates; a human runs the
  restore during a disaster so we never auto-overwrite a live volume.
- Test the restore path on a throwaway stack at least once so it is not the
  first time during a real incident.
