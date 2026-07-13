# Implementation guard — TASK-260712-2y74io

Before editing, read the complete task card and lifecycle diagram, docs/spec-self-contained-audio.md sections 3.13, 18, and P1.0/P1.7 in section 19, .task-board/.resources/TASK-260712-dib11l/windows-capture-bridge-r16.md, .task-board/.resources/TASK-260712-dib11l/root-review-r4-acceptance.md, and .task-board/.resources/TASK-260712-3v1k7q/p1-root-review-amendments.md. Preserve the accepted bridge baseline.

Work only within this task scope. Preserve AppContainer boundaries and the dirty worktree. Implement deterministic idempotent shutdown for quit, suspend/session lock, and permission revoke wherever observable; explicitly document real platform limitations. Cleanup must cover capture, hotkey registration, temporary artifacts, and ordered non-secret evidence logging. Repeated start/stop must not leave hidden capture or a hung process. Do not weaken manifest capabilities, introduce mocks into production paths, fabricate Windows hardware evidence, create commits, or push.

Add focused lifecycle tests using real production seams, then run all relevant Go/native build and test checks available on this host. Distinguish host-verifiable evidence from the later signed-MSIX Windows 10/11 hardware gate.

When finished, attach an outcome resource containing exact changed files, behavior mapped to every AC/checklist item, exact commands and unabridged pass/fail summary, residual platform gaps, and git diff/status scope. Leave the task in to-review; completion requires an independent reviewer and root line-by-line review.