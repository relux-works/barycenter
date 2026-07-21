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
- [x] Capture reproducible AUMID process lifetime, exit behavior and startup artifacts on mbpro-win
- [x] Identify and repair the silent startup-exit path
- [x] Add deterministic regression tests and startup diagnostics
- [x] Build and install the repaired production-schema MSIX on mbpro-win
- [x] Record autonomous process, window, package and hash evidence
- [x] Complete independent code review and close actionable findings
- [x] Implementation matches AC
- [x] Solution fits project architecture
- [x] Tests green
- [x] If review does not accept the work — verdict evidence added and status routed by the explicit verdict branches

## Notes
2026-07-21: Reproduced from repeated user shortcut clicks. AppModel/TWinUI activation succeeds, AppContainer PIDs live about one second, no WER/Application Error is emitted, package status is Ok, and LocalCache/Roaming/Pulsar/pulsar.log exists at zero bytes. Treating this as an application startup exit rather than shortcut registration failure.
2026-07-21 candidate: installed package 0.1.11.0 (MSIX b8374791..., EXE 839b00a8...) on DESKTOP-3PBO632. Exact AUMID run stayed alive/visible for 31.199s in 120/120 samples; UI Automation reported responding=true, visible=true, hung=false at 192 DPI and the process remained alive after observer completion. Root cause was unbounded all-section initial render before the message pump, compounded by hidden top-level activation and broken GUI log fan-out. Manual audio/hardware acceptance remains outside this bug.
spawn agent resolution: Agent selection: claude via explicit_override
spawn queued: [reviewer] reviewer (claude) (run=RUN-260721-536d63, max_parallel=20)
spawn run started: [reviewer] reviewer (claude) (run=RUN-260721-536d63)
agent completed: [reviewer] reviewer (claude) (exit=1)
spawn run completed: claude (run=RUN-260721-536d63, pid=48958, exit=1)
spawn agent resolution: Agent selection: claude via explicit_override
spawn queued: [reviewer] reviewer (claude) (run=RUN-260721-72524f, max_parallel=20)
spawn run started: [reviewer] reviewer (claude) (run=RUN-260721-72524f)
REVIEWER VERDICT: ACCEPTED (RUN-260721-72524f, non-goal-bound). Diagnosis & fix (commit 62302e0) correct and complete. Empty pulsar.log root cause: io.MultiWriter aborted on GUI invalid stderr handle before file sink; replaced with bestEffortWriter (succeeds if any sink accepts full write; CLI still mirrors stderr). ~1s silent AppContainer exit root cause: monolithic initial render() configured all 129 off-screen controls before the Win32 message pump started, so Win10 killed the unresponsive activation; fix creates top-level HWND visible (wsVisible), child controls hidden (style &^= wsVisible), bounded Home-only initial render via renderHome() early-return (verified safe). Observability: configureCrashOutput/debug.SetCrashOutput crash sink; guardUnpairedShellStartup recovers GUI panics into durable log + native showFatalStartupError messageBox; tray WM_DESTROY guarded via shouldPostTrayQuit. AC coverage: window visible/no-console/alive (PID 4188, 120/120 samples, responding, not hung, no terminal); durable diagnostic + user-visible error present; deterministic tests cover startup seams; exact repaired package 0.1.11.0 installed on mbpro-win with package/exe/screenshot SHA-256 recorded; no manual audio PASS claimed. Verification: go test ./... uncached GREEN; GOOS=windows go vet ./... clean; -H windowsgui cross-build succeeds. Fits project architecture (platform-agnostic testable seams, Windows plumbing isolated, darwin stub in sync). No forced fits. Verdict: done.
agent completed: [reviewer] reviewer (claude) (exit=0)
spawn run completed: claude (run=RUN-260721-72524f, pid=49095, exit=0)
2026-07-21 post-review extended soak: exact installed 0.1.11.0 AUMID remained alive in 720/720 samples for 188.523 seconds; Pulsar HWND was visible in 719/720 after the first pre-HWND sample. Independent UI Automation during the soak reported processRunning=true, responding=true, mainWindowVisible=true, mainWindowHung=false at 192 DPI; screenshot SHA-256 49e2836428c0b136cece5f03a54e7da8fc3c608ec0c2fcbcb491c2936b866887. No manual audio/hardware PASS is inferred.
2026-07-21 ordinary-shortcut proof: Windows Desktop Shell default verb invoked Pulsar.lnk; launcher task completed with result 0 while PID 6152 remained alive outside it. A separate UI Automation capture task also completed and reported title=Pulsar, visible=true, responding=true, hung=false, DPI=192, 22 controls; screenshot SHA-256 e398c8c2ea9d38137effc8d298fb47fb5a08be888850dee0f4e59a902a6fe9c1. This closes the exact user shortcut-launch path without terminal interaction.

## Precondition Resources
(none)

## Outcome Resources
- [windows-packaged-launch-evidence.md](file://BUG-260721-2jmabl/windows-packaged-launch-evidence.md) — Reproduction, root cause, exact Win10 package hashes, 188-second soak and ordinary Desktop shortcut evidence
- [BUG-260721-2jmabl_spawn-log_-reviewer--reviewer--claude-_RUN-260721-536d63.log](file://BUG-260721-2jmabl/BUG-260721-2jmabl_spawn-log_-reviewer--reviewer--claude-_RUN-260721-536d63.log) — System spawn log captured by task-board
- [BUG-260721-2jmabl_spawn-log_-reviewer--reviewer--claude-_RUN-260721-72524f.log](file://BUG-260721-2jmabl/BUG-260721-2jmabl_spawn-log_-reviewer--reviewer--claude-_RUN-260721-72524f.log) — System spawn log captured by task-board
- [BUG-260721-2jmabl_review.md](file://BUG-260721-2jmabl/BUG-260721-2jmabl_review.md) — Independent reviewer verdict: ACCEPTED

## Created
2026-07-21T15:47:23Z

## Last Update
2026-07-21T17:00:09Z

## Assigned To
(none)
