# Document transmission contract, rollout, and cross-story handoff

## Description
Capture the final HTTP and WebSocket contract, receipt matrix, rollout order, and explicit dependencies for UI, Telegram, mixer, and Store-compliance work.

## Scope
Record the final transmission request and receipt model, legacy downgrade behavior, rollout order for additive migrations and mixed-version nodes, and the exact semantics downstream UI and bot stories should render for status, presence, DND, block, and receipts. Link the evidence, diagrams, and contract notes so later stories do not rediscover protocol behavior by reading coordinator code.

## Acceptance Criteria
Downstream stories can implement labels, presence rendering, and certification notes without reopening transmission semantics. The rollout note names the required deploy order and legacy-node window. Documentation links the final contract note, diagrams, and verification task outputs in one place.
