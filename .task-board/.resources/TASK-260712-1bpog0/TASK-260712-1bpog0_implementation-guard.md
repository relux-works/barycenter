# Implementation guard — TASK-260712-1bpog0

Before editing, read the complete task card and its identity model, docs/spec-self-contained-audio.md sections 3.13, 6, 11, 12, 18, and 19, .task-board/.resources/TASK-260712-3v1k7q/research.md (accepted Rev15 contract), and .task-board/.resources/TASK-260712-3v1k7q/p1-root-review-amendments.md. Treat the accepted contract and source specification as authoritative.

Work only within this task scope. Preserve the dirty worktree and all existing production state, orbit roles, slot ownership, pair/node token validity, and rollback compatibility. Migrations must be additive. Newly introduced server-side secrets must be hash-only at rest and absent from logs, fixtures, screenshots, and outcome artifacts. Keep the resolver behind self_service_onboarding. Do not create commits or push.

Implement production code and focused tests, including migration/backfill from a representative pre-feature database, hash-only persistence/lookup, capability distinctions, feature-off compatibility, and relevant full Go test suites. Do not claim external evidence that was not executed.

When finished, attach an outcome resource containing exact changed files, behavioral decisions mapped to every AC/checklist item, exact commands and unabridged pass/fail summary, residual risks, and git diff/status scope. Leave the task in to-review; completion requires an independent reviewer and root line-by-line review.