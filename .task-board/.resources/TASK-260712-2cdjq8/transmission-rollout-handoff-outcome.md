# Phase 1 transmission rollout handoff outcome

- Task: `TASK-260712-2cdjq8`
- Story: `STORY-260712-25lysg`
- Exact documentation/code head: `cd234c913634db2fef5bbfcd866e8298e45f23cb`
- Pull request: `#28`
- Hosted exact-code CI: `29334550550`
- Manual evidence boundary: `EPIC-260714-th54l3`

The canonical deliverable is
[`docs/analysis/p1-transmission-rollout-handoff.md`](../../../docs/analysis/p1-transmission-rollout-handoff.md).
It is the stable entry point for the frozen caller and receipt contract,
legacy whole-downgrade rules, coordinator-first mixed-version rollout,
drain-before-rollback procedure, downstream ownership and the linked contract,
diagram and deterministic-evidence set.

An automated document guard verifies the frozen operational decisions, all
required evidence targets and the runbook/protocol entry points. No real-app,
packaged-install, audible or physical-hardware outcome is claimed.

Local coordinator vet/unit/race, Windows vet/unit/race and amd64/arm64
cross-builds, Swift release build, task-board validation and diff checks
passed. Hosted exact-code run `29334550550` passed coordinator, node-core,
pulsar-win and the signed packaged-probe job.
