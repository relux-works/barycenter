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
- [x] For the two-home Air E2E, open the installed Pulsar in an interactive Windows desktop session and complete Create a new Barycenter (never Connect another device); leave Pulsar open and report ready so the Mac Air invite and Windows join verification can continue.
- [x] For the resumed two-home Air E2E, close and relaunch Pulsar once on the interactive Windows desktop after onboarding; leave it open and report ready so nodes_connected=2 can be verified.
- [x] For the two-home Air E2E, on Mac create a test Air and issue one member invitation; keep the one-time code private and report code ready without posting it in chat.
- [x] On Windows open Airs, paste the private one-time member code into Join Air, review the invitation and confirm join; never use Connect another device. Leave both apps open and report joined.
- [x] After the duplicate-instance cleanup, launch exactly one Pulsar instance from the interactive Windows desktop, leave it open, and report ready before issuing a fresh Air invite.
- [x] On Mac, select the existing hhome-test Air and issue a fresh member invitation (do not create another Air); save or overwrite /Users/iv/Downloads/pulsar-air-invite-code.txt without posting the code in chat, then report fresh code ready.
- [ ] On Windows, use the fresh Desktop invite file in Airs > Join Air > Review > Confirm for the existing hhome-test; leave both apps open and report joined so membership can be independently verified before the reuse test.
- [x] On Windows Airs, refresh the projection, locate the existing Pending confirmation / Ожидает подтверждения hhome-test card, and click Join and activate / Вступить и включить; do not enter another invite code. Leave both apps open and report confirmed.
- [ ] Shared playback E2E: on Mac Airs press Command-R (or the circular-arrow toolbar button), make hhome-test active, then from Home record a short voice clip; in Outgoing drafts choose Current Air and After current, acknowledge rights, Send. On Windows keep Allow all and audible volume, then report whether the clip was heard.

## Notes
Owner: Ivan Oparin. Do not start until TASK-260721-2346wf Desktop UI automated acceptance and owner handoff publishes exact candidate hashes and the autonomous engineering goal is otherwise complete.
Autonomous handoff candidate: source a7258db, local manifest SHA-256 17735d6f42371e75824689bcdc926676bb1b29dd2f63cf4dd7e897e126a6970b, hosted desktop jobs in CI 29828660852 passed, signed diagnostic probe SHA-256 42081733678469a97d065ef0c0950c7f481b3e63ccfc5b528dce96a50fac8994. Do not start production rows until a signed Windows production candidate and notarized macOS DMG from this accepted source are supplied. The attached final-owner-verification.md is the only manual checklist; no terminal use or legacy task revival is requested.
2026-07-21 Windows preparation: installed exact current test-signed production MSIX ReluxWorksLLC.PulsarBarycenter_0.1.1.0_x64__q036g2bzd7ngc on DESKTOP-3PBO632/admin. Package SHA-256 eaa01ad6de70bf020a9ff4f145045003a93a475ae8711e62e30b532531f79d4a; GUI EXE SHA-256 accd11d545ff89aa0ce106b1599771c588be9e90ba32a5f0530868bae5f43d28. Package status Ok, valid Developer signature, packagedClassicApp/appContainer, microphone capability, Start AUMID and Desktop shortcut verified. Installer did not launch the app: no visible-window, microphone, audio, hardware or manual PASS is claimed. Windows functional rows are ready; final task still waits for macOS notarized candidate and makes no Store/EV claim.
2026-07-21 Windows launch-repair handoff: installed exact accepted package ReluxWorksLLC.PulsarBarycenter_0.1.11.0_x64__q036g2bzd7ngc from commit 62302e0 on DESKTOP-3PBO632. Package SHA-256 b8374791fa95c4b17eb1cae9195c19e344293263678946a02c958599800aafa2; GUI EXE SHA-256 839b00a84dd271121b8c4987a33b97b238e3ea9d458e19f39c09b0540265f0bb. AUMID soak stayed alive 720/720 samples for 188.523 seconds with visible Pulsar HWND in 719/720 after the first pre-HWND sample; UI Automation reported visible, responding and not hung at 192 DPI. This closes autonomous launch engineering only; microphone, audio, subjective UI, hardware and manual PASS remain unclaimed, and the consolidated task still waits for the macOS candidate.
2026-07-26 owner instruction: connect to the designated Windows host via SSH alias win and install the latest Windows build. Treat this as Windows candidate preparation only: verify source/version/hash/signature, preserve manual acceptance boundaries, and record the installed package receipt without claiming functional PASS rows.
2026-07-26 Windows candidate preparation completed on DESKTOP-3PBO632/admin. Upgraded the installed ReluxWorksLLC.PulsarBarycenter package from 0.1.20.0 to latest GitHub release v0.3.0-beta.32 / MSIX version 0.3.32.0 at source commit 0445f3f. Frozen release MSIX SHA-256 8f4a733e99cb12ad002db567317271ac731de72218a553994e7394c9b075ea7c matched GitHub but was unsigned; a separate copy was test-signed with the already-present and already-trusted publisher certificate without modifying trust stores. Test-signed MSIX SHA-256 b487b498280cc4e79ff2deb65d8d0ee5ac0330a76637c3078fa603348d902d97. Installed PackageFullName ReluxWorksLLC.PulsarBarycenter_0.3.32.0_x64__q036g2bzd7ngc reports Status=Ok. A launch attempt from the non-interactive OpenSSH session did not expose a surviving GUI process and produced no recent Application Error or Windows Error Reporting event; ordinary Start-menu/manual launch remains unclaimed. No microphone, audio, UI quality, transport, lifecycle, Store, EV, or manual acceptance PASS is claimed. Receipt: TASK-260721-ryk8c0_windows-install-receipt-v0.3.0-beta.32.json.
2026-07-27 focused owner boundary from TASK-260727-1msjz6: production Air authority is live and accepted, but ssh win is non-interactive and the Windows candidate is not onboarded (no DPAPI credentials or Barycenter identity). The only required owner action is to launch the installed Pulsar from the physical/RDP Windows desktop, choose Create a new Barycenter (not device pairing), leave the app open, and reply ready; autonomous E2E resumes afterward.
2026-07-28 Ivan reported готово: the Windows interactive onboarding step is complete and Pulsar was left open. This checks only the focused two-home Air prerequisite, not the broader final owner-verification rows.
2026-07-28 Ivan reported готово after the required Windows Pulsar relaunch. This checks only the focused E2E workaround step; downstream Air create/join rows remain unverified.
2026-07-28 Ivan reported код есть: Mac test Air creation and one member invitation were completed. The one-time secret was deliberately not posted in chat. This does not yet assert Windows consumption or membership PASS.
2026-07-28 Ivan reported вошел after Windows Airs > Join Air > Review > Confirm using the privately transferred one-time member code. This records the owner action only; membership and persistence PASS await independent verification.
2026-07-28 Ivan reported готово after the duplicate-instance cleanup and launched exactly one Windows Pulsar. This records only the owner action; stable-session PASS awaits independent verification.
2026-07-28 stable precondition verified before fresh invite: one Windows Pulsar process with unchanged PID across 30s, two established 443 connections, coordinator nodes_connected=2. The prior invite must not be reused because it may be expired or consumed by the failed attempt.
2026-07-28 Ivan reported fresh code ready for the existing hhome-test Air. Orchestrator overwrote C:\Users\admin\Desktop\pulsar-air-invite-code.txt by direct scp, then verified remote size 43 bytes and SHA-256 equality without reading or logging the secret. Next owner-only step: Windows Airs > Join Air > paste from file > Review > Confirm, then leave both apps open and report joined.
2026-07-28 correction after server-side read-only diagnosis: item 12 was unchecked. The transferred 23:13 file contained an older invite; production air_invites showed no new hhome-test issuance after approximately 22:42 UTC, and the latest open rows had expired at approximately 22:57 UTC. The generic invite_unavailable text covers expired, used, withdrawn, or unknown. Required owner action: Mac Refresh Airs, select existing hhome-test, explicitly issue a new member invite, wait for the newly issued invite result, save/overwrite the exact Downloads path, then report issued so server creation time is verified before transfer.
2026-07-28 verified fresh invitation successfully: coordinator created a new open hhome-test member invite at 23:23:33.645 UTC expiring 23:38:33.645 UTC; Mac file mtime was 23:23:41 UTC. Windows remained one stable PID 8336 with two established 443 connections. Exact Desktop file was overwritten and size/SHA-256 equality verified without reading or logging the code.
2026-07-28 shared-playback E2E blocked before transmission by reproducible Mac production capture failure BUG-260728-3mx5hm. Two attempts with built-in speakers plus explicit MacBook Pro Microphone and explicit one-generation degraded consent both returned capture_backendUnavailable. CoreAudio evidence shows CADefaultDeviceAggregate channel-layout validation churn followed by StartIO error 35; valid aggregate arrives only after the app has already failed. No local draft or server transmission was created, so Windows audibility remains untested rather than failed.
2026-07-28 Mac capture blocker BUG-260728-3mx5hm accepted and installed as production-signed Pulsar 0.3.0 (958.2), PID 90081. Automated gates and installed runtime health are green. Mac recording and Mac-to-Windows audibility remain NOT TESTED after the fix until the owner performs the next manual Record/Stop/Send check; do not claim E2E pass yet.
2026-07-29 post-fix owner smoke still did not create a Mac draft. Healthy default MacBook Pro Microphone was rejected indirectly because ordinary clip capture constructed a transient VPIO/default-duplex aggregate that disappeared with OSStatus !dev 560227702. Shared playback remains NOT TESTED. Architectural rework is tracked in BUG-260729-3kecpr.

## Precondition Resources
(none)

## Outcome Resources
- [TASK-260721-ryk8c0_windows-install-receipt-f6c9b47.json](file://TASK-260721-ryk8c0/TASK-260721-ryk8c0_windows-install-receipt-f6c9b47.json) — Exact Windows 10 local-test-signed MSIX installation receipt; preparation evidence only, no manual PASS claim
- [final-owner-verification.md](file://TASK-260721-ryk8c0/final-owner-verification.md) — Single no-terminal owner checklist with current exact Windows candidate and deferred macOS handoff
- [TASK-260721-ryk8c0_windows-install-receipt-62302e0.json](file://TASK-260721-ryk8c0/TASK-260721-ryk8c0_windows-install-receipt-62302e0.json) — Exact installed Windows 10 package, component hashes, 188-second soak and Desktop shortcut launch evidence
- [TASK-260721-ryk8c0_windows-install-receipt-v0.3.0-beta.32.json](file://TASK-260721-ryk8c0/TASK-260721-ryk8c0_windows-install-receipt-v0.3.0-beta.32.json) — Windows 10 installation receipt for locally test-signed latest GitHub release v0.3.0-beta.32; package identity, hashes, signature and installed status only

## Created
2026-07-21T10:58:43Z

## Last Update
2026-07-29T13:34:35Z

## Assigned To
Ivan Oparin
