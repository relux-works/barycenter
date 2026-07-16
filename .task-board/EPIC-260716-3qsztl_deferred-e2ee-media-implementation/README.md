# Independent E2EE design audit and deferred implementation

## Description
Run the independent cryptographic design audit and only then implement client-owned E2EE media. This separate epic is intentionally outside the sequential EPIC-260712-3agrc1 Self-contained Pulsar Audio engineering agent cycle.

## Scope
Consume the audit-ready threat model, selected cryptographic and protected-container stacks and frozen protocol from STORY-260712-1frfmi. First execute TASK-260712-aniuyy. Only after its acceptance may schema, key state, coordinator routing, recovery, report evidence, protected send and playback, live PTT integration, UX, C4-C6 engineering evidence and external implementation review begin. Protocol-affecting changes reopen the design audit.

## Acceptance Criteria
TASK-260712-aniuyy is independently accepted with no open critical or high design finding before any implementation child enters development. All sixteen post-audit implementation and evidence tasks then follow the exact reviewed hashes, and the external cryptographic implementation review closes critical and high findings. e2ee_media remains off until separate rollout acceptance.
