# TASK-260722-3fsxj5: live-self-service-onboarding-rollout

## Description
Safely enable the already implemented self-service onboarding API on the live Barycenter coordinator.

## Scope
Identify the authoritative deployment runtime and current exact image/config. Take and verify a restorable database/config backup, record current health/version/orbit/node counts, validate additive identity schema and rollback posture, enable SELF_SERVICE_ONBOARDING=1 through the owned deployment mechanism, and verify health plus route registration without creating durable test identities. Keep legacy Telegram and existing node connectivity intact. Record exact deployment provenance and rollback command.

## Acceptance Criteria
A verified pre-change backup and rollback procedure exist. The live coordinator remains healthy at the expected version, existing orbit counts are preserved, and GET probes distinguish registered Create/device-invite/consume routes from 404 without mutating data. Logs show no migration or startup errors and the rollout flag is durable across restart. Exact evidence is attached and independently reviewed.
