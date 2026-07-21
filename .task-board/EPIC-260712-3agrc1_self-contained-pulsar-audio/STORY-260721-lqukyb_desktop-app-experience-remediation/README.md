# STORY-260721-lqukyb: desktop-app-experience-remediation

## Description
Raise the shipped Windows and macOS desktop shells to a professional system-native quality bar while preserving existing product flows. Correct the Windows test-launch instrumentation so it never exposes a console window, make the signed Windows probe DPI-aware and visually legible without presenting it as the product UI, polish the production Windows shell for high-DPI system theming and accessibility, polish the macOS SwiftUI shell for Retina-native hierarchy, scenes, localization and accessibility, and finish with deterministic cross-platform UI quality gates.

## Scope
In scope: Windows GUI-subsystem and hidden observer launch behavior; probe PerMonitorV2 font/layout/theme; production Windows 10/11 shell theming, DPI, keyboard, contrast and resize behavior; macOS 14+ SwiftUI navigation, toolbar, settings, layout, localization, VoiceOver semantics and previews; automated source/layout/model/package checks. Out of scope: claiming physical DPI, Narrator, VoiceOver, audible, Store or hardware acceptance; those are represented by one consolidated owner task in EPIC-260714-th54l3.

## Acceptance Criteria
No observer or packaged app launch exposes a terminal window. The diagnostic probe is crisp and usable at 100/125/150/200 percent scaling and clearly identifies itself as verification tooling. The production Windows and macOS apps use system typography, spacing, navigation, control states, focus behavior, localized EN/RU copy and accessible non-color status semantics; Windows includes safe Win10 fallbacks and macOS renders natively on Retina. Deterministic tests, builds and package checks cover the changed seams. No manual result is claimed.
