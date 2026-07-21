# TASK-260722-1zv67l: macos-create-windows-join-readiness-handoff

## Description
Verify the complete automated readiness boundary and reduce final owner work to one Mac Create and Windows Join pass.

## Scope
Re-run coordinator health/route probes, macOS full tests/release/package verification, ordinary Mac launch/UI inspection, and Windows installed-package/version/join-surface health. Publish exact Mac and Windows hashes, the two-screen owner sequence, expected success states, cleanup guidance and focused bug routing. Update TASK-260721-ryk8c0 without marking any manual row passed.

## Acceptance Criteria
All deterministic readiness gates are green and independently accepted. The handoff names exact installed Mac and Windows candidates and asks Ivan Oparin only to create on Mac, save recovery, generate/copy one invitation, join on Windows and report the visible result. No terminal commands or duplicate manual tasks are required, and no manual PASS is claimed before the owner performs it.
