REVIEWER VERDICT 2026-07-22 (RUN-260721-5b05eb, claude-opus-4-8): ACCEPTED.

Independently re-verified live state (read-only, GET-only, no mutation):
- healthz HTTP 200 -> {status:ok, version:git-3565c1e1ca0511168026ec2ba72440d23fb1317f, orbits:3, nodes_connected:0} — matches evidence exactly.
- Route probes: GET /v1/onboarding/orbits=400, /v1/device-invites=400, /v1/device-invites/consume=400, negative control=404. Source confirms registerOnboarding returns nil when cfg.SelfServiceOnboarding is false (routes would 404), so live 400s prove the flag is ON and routes registered.
- Running container 3c032b292354 image h5pm6j2dmmj8f80ffuzayuks:3565c1e1... with DUET_SELF_SERVICE_ONBOARDING=1 in env — flag durable in the actual running container.
- Startup log: version git-3565c1e1..., telegram_enabled=true, self_service_onboarding=true, listening 0.0.0.0:8080; full log scan shows zero error/panic/fatal/migration-failure lines. Legacy Telegram preserved.
- Backup dir mode 0700; spot-checked SHA-256 duet-prechange.db=54db4c6c... and pinned-predecessor-e8bd240-image.tar=f85c29cf... both match evidence.
- Rollback alias barycenter-rollback:TASK-260722-3fsxj5-e8bd240 present with image id sha256:4c23a2f199... matching evidence. Executable rollback procedure documented (not flag-off; DB restore + projection + predecessor image via owned Coolify path).
- Commits 3565c1e1 ("feat: make identity rollback reproducible") and predecessor e8bd240 exist in repo.

AC met: verified pre-change backup + rollback exist; coordinator healthy at expected version; orbit counts preserved (3->3); GET probes distinguish registered routes from 404 without mutation (identical pre/post probe backup hashes); no migration/startup errors; flag durable across restart. Coolify runtime correctly identified as NOT relux-remote-infra-owned (matches orchestrator preflight). Coolify main-branch mis-deploy incident was honestly reported and cleanly recovered (writes frozen, 12 legacy tables proven byte-unchanged, pre-change DB restored, exact image restarted, erroneous image removed). No product code changed; release validation gates (gofmt/vet/test/-race/build) green at the exact commit. Solution fits architecture. Tests green. Verdict: done.
