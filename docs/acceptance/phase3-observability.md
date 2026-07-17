# Phase 3 observability, health and evidence export

`GET /v1/moderation/phase3-observability` is the sanitized operator snapshot for
Phase 3 runtime review. It requires HTTPS and a live moderation credential with
the `list` capability, rejects every query parameter, and returns
`Cache-Control: no-store`. The machine contract is
[`acceptance/phase3/observability-contract-v1.json`](../../acceptance/phase3/observability-contract-v1.json).

The endpoint contains only fixed aggregates. It never emits actor, orbit, node,
session, cue, schedule, transmission or media identifiers; audio, ciphertext,
filenames, captions, transcripts, keys, bearer tokens, credential hashes, local
paths and arbitrary error strings are prohibited. The environment is represented
by a domain-separated SHA-256 reference, not by a raw URL or hostname.

## Runtime and evidence semantics

- Live PTT exports bounded lifecycle, relay, duplicate/stale/invalid and drop
  counters plus the mandatory zero-retention gauges. Mouth-to-ear latency and
  jitter remain `client_evidence_required`; the coordinator cannot infer audible
  playout from server timing.
- Capture quality is aggregated across node snapshots into fixed lifecycle,
  quality and input-health enums. A connected capable node without a heartbeat
  blocks runtime readiness. Capture stop/callback evidence still requires the
  real client process.
- E2EE is explicitly `deferred_unavailable`, with the feature flag false and no
  claimed epoch or revocation samples. Public crypto metrics never contain keys.
- Soundboard and automation use canonical database aggregates over the rolling
  24-hour window. Automation enabled without soundboard, or all scopes emergency
  disabled, is visible and blocks or warns as specified by the contract.
- `/healthz` reports coarse runtime readiness only. Disabled optional subsystems
  do not degrade the established product; enabled subsystems with missing or
  unsafe telemetry do. A green health response never proves a manual gate,
  independent review, rollout drill, beta soak or promotion decision.

`promotion_evidence_ready` is always false in this runtime view. The signed
Phase 3 campaign manifest owns that decision and must bind each snapshot to its
bytes, SHA-256, build, environment hash, flag permutation, collector and time.

## Collect and archive

Provision a short-lived list-only operator credential using the coordinator
command documented in `docs/moderation-operations-runbook.md`. On a trusted
operator workstation with shell tracing disabled:

```sh
umask 077
export MODERATION_LIST_TOKEN='<one-time-loaded-secret>'
run_id="$(date -u +%Y%m%dT%H%M%SZ)"
artifact=".temp/acceptance/phase3/observability/${run_id}/sanitized-metrics.json"
mkdir -p "$(dirname "${artifact}")"
curl --fail --silent --show-error \
  -H "Authorization: Bearer ${MODERATION_LIST_TOKEN}" \
  https://<coordinator>/v1/moderation/phase3-observability \
  > "${artifact}"
unset MODERATION_LIST_TOKEN
shasum -a 256 "${artifact}"
wc -c "${artifact}"
```

Do not add tenant selectors or labels to the URL or archive name. Store only the
sanitized response and its manifest binding. Revoke the temporary operator
credential after collection.

Validate the repository contract before and after collection:

```sh
python3 scripts/acceptance/validate_phase3_observability.py
python3 scripts/validate_phase3_gate_matrix.py
```

## Alert and review commands

Review the fixed `readiness.alerts` values and hold the affected capability when
any enabled subsystem is `blocked`. Treat non-zero retained or persisted Live
PTT audio as critical. Treat missing capture telemetry from a connected capable
node as critical. Do not turn `client_evidence_required`, `not_run` or
`deferred_unavailable` into zero-valued passing measurements.

Manual app/hardware testing, acoustic and route fixtures, accessibility,
independent reviews, recovery drills and beta incident evidence remain in
`EPIC-260714-th54l3`; this engineering task does not claim them.
