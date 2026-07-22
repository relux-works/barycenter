## Status
backlog

## Review
required

## Task Class
metadata

## Blocked By
- TASK-260721-2346wf

## Blocks
- (none)

## Checklist
- [ ] Install and launch exact Windows and macOS candidates as ordinary GUI applications.
- [ ] Exercise deny/allow, built-in-microphone capture, playback, interrupt and recovery on both platforms.
- [ ] Spot-check Windows scaling/keyboard and macOS Retina/keyboard/VoiceOver presentation.
- [ ] Exercise one real routed transport plus enabled stream/live capability without duplicate playback.
- [ ] Smoke moderation, delete, recovery, restart and final cleanup.
- [ ] Complete the handoff-defined passive soak and return one timestamped verdict packet.
- [ ] SOLE CURRENT SEQUENCE (items 1-6 are superseded and stay unchecked): Mac Create once → save recovery → generate/copy one invitation → Windows Join securely once → report the visible result; no Terminal or broader manual/hardware work.

## Notes
Owner: Ivan Oparin. Do not start until TASK-260721-2346wf Desktop UI automated acceptance and owner handoff publishes exact candidate hashes and the autonomous engineering goal is otherwise complete.
Autonomous handoff candidate: source a7258db, local manifest SHA-256 17735d6f42371e75824689bcdc926676bb1b29dd2f63cf4dd7e897e126a6970b, hosted desktop jobs in CI 29828660852 passed, signed diagnostic probe SHA-256 42081733678469a97d065ef0c0950c7f481b3e63ccfc5b528dce96a50fac8994. Do not start production rows until a signed Windows production candidate and notarized macOS DMG from this accepted source are supplied. The attached final-owner-verification.md is the only manual checklist; no terminal use or legacy task revival is requested.
2026-07-21 Windows preparation: installed exact current test-signed production MSIX ReluxWorksLLC.PulsarBarycenter_0.1.1.0_x64__q036g2bzd7ngc on DESKTOP-3PBO632/admin. Package SHA-256 eaa01ad6de70bf020a9ff4f145045003a93a475ae8711e62e30b532531f79d4a; GUI EXE SHA-256 accd11d545ff89aa0ce106b1599771c588be9e90ba32a5f0530868bae5f43d28. Package status Ok, valid Developer signature, packagedClassicApp/appContainer, microphone capability, Start AUMID and Desktop shortcut verified. Installer did not launch the app: no visible-window, microphone, audio, hardware or manual PASS is claimed. Windows functional rows are ready; final task still waits for macOS notarized candidate and makes no Store/EV claim.
2026-07-21 Windows launch-repair handoff: installed exact accepted package ReluxWorksLLC.PulsarBarycenter_0.1.11.0_x64__q036g2bzd7ngc from commit 62302e0 on DESKTOP-3PBO632. Package SHA-256 b8374791fa95c4b17eb1cae9195c19e344293263678946a02c958599800aafa2; GUI EXE SHA-256 839b00a84dd271121b8c4987a33b97b238e3ea9d458e19f39c09b0540265f0bb. AUMID soak stayed alive 720/720 samples for 188.523 seconds with visible Pulsar HWND in 719/720 after the first pre-HWND sample; UI Automation reported visible, responding and not hung at 192 DPI. This closes autonomous launch engineering only; microphone, audio, subjective UI, hardware and manual PASS remain unclaimed, and the consolidated task still waits for the macOS candidate.
2026-07-22 readiness handoff supersedes the old broad owner resource with one two-screen sequence only. Exact installed Mac: Pulsar 0.3.0 (946), source fb807e1, NodeApp a862bfd5, local signed/non-notarized. Exact installed Windows: 0.1.20.0, source 76f09a4, EXE 0a77f53f, Developer signed. Ivan Oparin is asked only to Create on Mac, save recovery, generate/copy one invitation, Join on Windows, and report the visible result. No terminal, audio, hardware, accessibility, or duplicate manual task is requested. All six existing manual rows remain unchecked and no manual PASS is claimed. Deterministic evidence: TASK-260722-1zv67l; UIA semantics follow-up: BUG-260722-224lo9.

## Precondition Resources
(none)

## Outcome Resources
- [TASK-260721-ryk8c0_windows-install-receipt-f6c9b47.json](file://TASK-260721-ryk8c0/TASK-260721-ryk8c0_windows-install-receipt-f6c9b47.json) — Exact Windows 10 local-test-signed MSIX installation receipt; preparation evidence only, no manual PASS claim
- [final-owner-verification.md](file://TASK-260721-ryk8c0/final-owner-verification.md) — Single two-screen no-terminal Mac Create/recovery/invite to Windows Join owner pass with exact current candidates; manual rows remain unchecked
- [TASK-260721-ryk8c0_windows-install-receipt-62302e0.json](file://TASK-260721-ryk8c0/TASK-260721-ryk8c0_windows-install-receipt-62302e0.json) — Exact installed Windows 10 package, component hashes, 188-second soak and Desktop shortcut launch evidence

## Created
2026-07-21T10:58:43Z

## Last Update
2026-07-22T00:05:33Z

## Assigned To
Ivan Oparin
