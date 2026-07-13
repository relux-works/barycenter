# Cross-platform UI accessibility, DPI and acceptance evidence

## Description
Produce the verification coverage and acceptance evidence for the Phase 1 UI and local capture path.

## Scope
Add platform-appropriate tests, probes or manual evidence steps for shell state, local-only self-test, temporary-media cleanup, denied mic, absent device, hotkey conflict fallback, keyboard navigation, screen-reader labels, Windows DPI 125, 150 and 200 and the A1 capture path. Capture the sanitized screenshots, logs and review notes needed for decomposition follow-through and later certification preparation.

## Acceptance Criteria
There is reproducible evidence for the A1 UI path plus the negative cases called out in the story acceptance criteria, and equivalent macOS parity checks are named and executable. The verification package identifies any remaining cross-story prerequisites for A2, A6, A7 and A8 and avoids secrets, raw audio or local paths in published artefacts.
