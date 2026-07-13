# Prove Store-safe hold input and lost-release fail-safe

## Description
Select or reject a true global key-down/up route per platform before any live capture relies on it.

## Scope
Extend the signed Windows AppContainer and macOS sandbox probes to test press, repeat, release and lost release across supported foreground apps, hidden windows, keyboard layouts, remoting, lock, suspend, quit, UAC secure desktop and accessibility conflicts. Compare only documented APIs and record capability, permission and thread ownership. Freeze debounce, one-session-at-a-time, maximum hold watchdog, focus and lock cancellation, visible or audible capture indicators, emergency local Stop, crash recovery and when the UI must use Phase 1 toggle instead. Never request broad Accessibility or relax AppContainer silently.

## Acceptance Criteria
A dated real-hardware matrix selects a reliable documented hold path per supported environment or declares toggle-only. One hundred probe cycles have no lost release or stuck microphone, unsupported secure or remote desktops cannot record invisibly, and every kill trigger, watchdog and fallback is precise enough for platform implementation without broader permissions.
