# Independent E2EE design audit and post-audit implementation

## Description
Pass the independent cryptographic design review, then implement and independently review the client-owned E2EE media design.

## Scope
Start with TASK-260712-aniuyy using the audit-ready packet produced by STORY-260712-1frfmi. All remaining children are post-audit work and remain directly blocked by the audit task. Own schema, runtime, platform key state, protected send and playback, live PTT crypto, recovery, history grants, report evidence, UX, engineering evidence and external implementation review.

## Acceptance Criteria
The independent design review passes before any implementation starts. After the gate, implementation matches the exact reviewed contract; protected clips, tracks, cues and live PTT interoperate across Windows and macOS; recovery and reporting preserve the reviewed boundary; C4-C6 engineering evidence is reproducible; external implementation review closes every critical and high finding.
