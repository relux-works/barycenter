# Phase 2 observability and quota evidence

`GET /v1/moderation/phase2-observability` is the single sanitized operator and
acceptance view for Phase 2. It aggregates the existing stream accounting,
playback domain, target, inbox and Air tables; it does not write parallel quota
or metric rows. The endpoint requires HTTPS and a live moderation credential
with the `list` capability and always returns `Cache-Control: no-store`.

The machine contract is
[`acceptance/phase2/observability-contract-v1.json`](../acceptance/phase2/observability-contract-v1.json).

## Readiness behavior

`/healthz` exposes only coarse Phase 2 readiness, feature state and the existing
stream-accounting ready/saturated/reconciliation fields. Disabled streamed
tracks do not make Phase 1 unhealthy. Once streamed tracks are enabled,
accounting reconciliation, the media processor and stream storage are all
mandatory. Once Air is authoritative, any authority divergence or pointer to a
non-active/non-joined runtime makes health degraded.

The current codec/player decision remains no-go, so `streamed_tracks.enabled`
is false. That is a truthful flag-off state, not a passing production playback
claim.

## Query and sanitized export

Run from a trusted operator workstation without shell tracing:

```sh
umask 077
export MODERATION_LIST_TOKEN='<one-time-loaded-secret>'
curl --fail --silent --show-error \
  -H "Authorization: Bearer ${MODERATION_LIST_TOKEN}" \
  https://<coordinator>/v1/moderation/phase2-observability \
  > .temp/acceptance/phase2/<campaign>/17-observability/<run-id>/sanitized-metrics.json
unset MODERATION_LIST_TOKEN
```

Copy the same exact bytes into the campaign's
`20.5/accounting/<run-id>/sanitized-metrics.json`, then record byte length and
SHA-256 in its artifact manifest. Do not use query parameters: the endpoint
rejects them so a raw tenant selector cannot enter a command log or artifact.

Validate the repository contract before and after the collection:

```sh
python3 scripts/acceptance/validate_phase2_observability.py
python3 scripts/validate_phase2_gate_matrix.py
```

The rolling event/timing window is 24 hours. Storage, queue, feature and Air
values are current gauges. Server target timing uses coordinator wall-clock
Unix milliseconds and excludes negative samples. Buffer depth and seek-to-audio require process-monotonic client
samples; until a real client campaign supplies them, the view returns
`client_evidence_required` with zero samples instead of inventing latency.

## Alert recipes

- Page immediately if an enabled runtime is not ready, Air divergence or
  runtime-shape anomalies are non-zero, or duplicate-target anomalies appear.
- Hold promotion when quota saturation has no approved capacity decision, or
  when processing/egress crash-release totals grow without an explained
  restart and reconciliation record.
- Investigate warning trends from the exact canonical accounting snapshot;
  never calculate a second quota total from logs.
- Keep the fixed JSON dimensions. Do not add IDs, filenames, titles, arbitrary
  errors or other unbounded labels to dashboards or exports.

Manual audible, packaged-hardware, production rollout and seven-day beta
evidence remains in `EPIC-260714-th54l3` and is not claimed by this view.
