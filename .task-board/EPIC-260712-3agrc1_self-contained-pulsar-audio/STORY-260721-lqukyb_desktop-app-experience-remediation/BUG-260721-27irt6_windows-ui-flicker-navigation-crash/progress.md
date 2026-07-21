## Status
done

## Review
required

## Task Class
code

## Blocked By
- (none)

## Blocks
- (none)

## Checklist
- [x] Capture current Win10 crash, flicker, DPI and process-lifecycle evidence
- [x] Document root causes in message dispatch, repaint, layout and control lifecycle
- [x] Implement stable rendering and crash-safe command handling
- [x] Deliver coherent system-native Windows shell layout, typography and states
- [x] Add deterministic regression tests and pass Go race vet and Windows build checks
- [x] Build sign install and autonomously soak the package on mbpro-win
- [x] Complete independent review and record outcome resources
- [x] Implementation matches AC
- [x] Solution fits project architecture
- [x] Tests green
- [ ] If review does not accept the work — verdict evidence added and status routed by the explicit verdict branches

## Notes
Engineering gate accepted on mbpro-win: signed MSIX 0.1.20.0 installed; UIAutomation 240/240 (max 93 ms, p95 80 ms), idle frame hashes 1, handles 347 to 346, no crash/AppModel events; direct WM_COMMAND 240/240; 150 ms PrintWindow gallery complete. Manual visual/audio acceptance remains out of scope.
Independent review anchor: exact commit 76f09a4 on tracking/bug-260721-27irt6-windows-ui-stability; review code, tests, evidence claims and manual-boundary honesty.
spawn agent resolution: Agent selection: claude via explicit_override
spawn queued: [reviewer] reviewer (claude) (run=RUN-260721-305602, max_parallel=20)
spawn run started: [reviewer] reviewer (claude) (run=RUN-260721-305602)
agent completed: [reviewer] reviewer (claude) (exit=1)
spawn run completed: claude (run=RUN-260721-305602, pid=1232, exit=1)
spawn agent resolution: Agent selection: claude via explicit_override
spawn queued: [reviewer] reviewer (claude) (run=RUN-260721-c80cef, max_parallel=20)
spawn run started: [reviewer] reviewer (claude) (run=RUN-260721-c80cef)
REVIEW VERDICT: ACCEPTED (run RUN-260721-c80cef, anchor 76f09a4). Prior run RUN-260721-305602 exit=1 was a Fable-5 429 rate limit, not a verdict. Re-ran all gates locally: windows cross-build, go vet, go test ./..., go test -race ./... all PASS; six new regression tests pass. Code review confirms the claimed fixes are real: reentrancy guard (ctx.rendering), section-bounded render with returns, bounds caching (controlBounds/appliedBounds, skip hidden/unchanged), structural-chrome-only repaint (no recursive RDW_ALLCHILDREN), setText/showControl skip-unchanged, wndProc panic recovery keeps AppContainer alive, Common-Controls v6 manifest, DPI/min-size layout retune. Tests were strengthened not weakened. Crash/flicker/hang ACs validated by recorded autonomous Win10 soak (240/240 UIA + 240/240 WM_COMMAND, idle frame-hash=1, 0 crash events) since Win32 logic is build-tag windows and cannot run on the review host. Manual visual/audio acceptance correctly kept out of scope. Fits pure-syscall Win32 architecture. Verdict evidence: BUG-260721-27irt6_review.md.
agent completed: [reviewer] reviewer (claude) (exit=0)
spawn run completed: claude (run=RUN-260721-c80cef, pid=1404, exit=0)
Post-review CI delta: packaged-probe failed only because TestWindowsProductionRenderIsSectionBounded matched LF against a Windows CRLF checkout. Test now normalizes CRLF to LF before the source assertion; production code and installed 0.1.20.0 are unchanged. Local test/race/vet/Windows cross-build remain green; awaiting rerun.
Hosted CI rerun 29863591495 passed 4/4 on delta head b625606: coordinator, node-core, pulsar-win and pulsar-win-packaged-probe. CRLF portability delta is accepted; task remains done.

## Precondition Resources
(none)

## Outcome Resources
- [BUG-260721-27irt6_windows-ui-stability-remediation.md](file://BUG-260721-27irt6/BUG-260721-27irt6_windows-ui-stability-remediation.md) — Root-cause analysis, signed 0.1.20.0 receipt, autonomous Win10 evidence, accepted review and green hosted CI
- [BUG-260721-27irt6_spawn-log_-reviewer--reviewer--claude-_RUN-260721-305602.log](file://BUG-260721-27irt6/BUG-260721-27irt6_spawn-log_-reviewer--reviewer--claude-_RUN-260721-305602.log) — System spawn log captured by task-board
- [BUG-260721-27irt6_spawn-log_-reviewer--reviewer--claude-_RUN-260721-c80cef.log](file://BUG-260721-27irt6/BUG-260721-27irt6_spawn-log_-reviewer--reviewer--claude-_RUN-260721-c80cef.log) — System spawn log captured by task-board
- [BUG-260721-27irt6_review.md](file://BUG-260721-27irt6/BUG-260721-27irt6_review.md) — Independent review verdict (ACCEPTED) with locally re-run gates, code-mechanism verification, and boundary check

## Created
2026-07-21T17:31:02Z

## Last Update
2026-07-21T20:00:16Z

## Assigned To
[reviewer] reviewer (claude)
