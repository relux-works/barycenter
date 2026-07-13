# Root Phase 1 line-by-line integration review

## Description
The root agent personally decides whether Phase 1 work is accepted after inspecting every implementation diff and all independent findings.

## Scope
Review the complete diff from the approved planning baseline file by file, preserve unrelated user changes, trace each change to a task AC, source section and A1-A8 scenario, and explicitly inspect security, protocol, migration, realtime audio, Store metadata and policy seams. Verify implementation-agent reports against code and artifacts, examine all independent reviewer findings and fixes, rerun targeted plus broad regression suites and compare real evidence to exact tolerances. Reject or return any task with unproven behavior; do not repair by silently accepting scope changes.

## Acceptance Criteria
The root review resource lists every reviewed commit or diff, AC and scenario mapping, commands and results, independent findings and dispositions, remaining risks and the exact accepted build hash. No implementation is accepted solely because its agent marked to-review, and no required test, platform proof or external limitation is silently waived.
