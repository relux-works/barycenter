# BUG-260721-27irt6: windows-ui-flicker-navigation-crash

## Description
Stop-the-line remediation of production Windows client flicker, unstable multi-button navigation, crash behavior, high-DPI scaling defects, and visibly unfinished information architecture. Investigate the shipped Win10 package and Win32 implementation, apply best-effort logic and UI fixes, and validate autonomously on the designated mbpro-win host.

## Scope
In scope: Win32 message loop and command dispatch; repaint/theme/font/layout/control lifecycle; crash diagnostics and guardrails; coherent EN/RU shell hierarchy; high-DPI and resize behavior; keyboard/focus/accessibility basics; deterministic unit/source/build/package tests; autonomous Win10 install and navigation soak. Out of scope: final human visual/audio acceptance, Win11 hardware acceptance, and unrelated backend capability work.

## Acceptance Criteria
Rapid repeated navigation and representative action sequences do not crash or hang the packaged app. Idle refresh and section transitions do not visibly flash the entire UI. The shell remains legible at the host DPI and common 100-200 percent scales, uses a restrained system-native hierarchy with consistent navigation/cards/actions/status, avoids clipped or overlapping primary content at the supported minimum size, and preserves existing product workflows. Go tests, race tests, vet, Windows build/package checks, autonomous Win10 interaction evidence, and independent code review pass. Manual acceptance is not claimed.
