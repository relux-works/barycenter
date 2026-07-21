# Desktop app experience remediation

## Objective

Ship Windows and macOS desktop shells that feel like ordinary, professional
system applications while keeping all existing Pulsar product flows and honest
capability states.

## Windows launch and diagnostic probe

- The packaged executable and production executable use the Windows GUI
  subsystem and never create a console window during an ordinary launch.
- Autonomous console-session instrumentation starts hidden and must not be
  mistaken for product UI.
- The signed hardware-verification probe clearly identifies itself as
  diagnostic tooling. It uses PerMonitorV2 awareness, system UI fonts,
  DPI-scaled geometry and deterministic relayout at 96, 120, 144 and 192 DPI.
- Probe presentation changes may not weaken permission, capture, lifecycle,
  evidence, package-identity or cleanup contracts.

## Production Windows shell

- Keep the existing Go/Win32 product architecture and command surface. Do not
  add a web shell or a runtime-heavy UI dependency.
- Use native Windows typography, spacing, navigation, focus and semantic-state
  conventions with safe Windows 10 fallbacks and progressive Windows 11 DWM
  enhancements.
- Follow system light/dark/high-contrast settings where supported. Status and
  destructive states must never rely on color alone.
- Preserve keyboard/dialog navigation, screen-reader names, minimum-window
  behavior and PerMonitorV2 relayout at 100, 125, 150 and 200 percent scaling.

## Production macOS shell

- Target macOS 14+ with native SwiftUI scenes, NavigationSplitView, toolbars,
  system typography/materials and Retina rendering.
- Prefer SwiftUI over new AppKit bridges. Existing AppKit integration may
  remain where menu-bar or application lifecycle ownership requires it.
- Keep view state owned or injected correctly, split complex surfaces into
  stable subviews and avoid layout work that fights SwiftUI.
- Preserve EN/RU behavior, keyboard commands, VoiceOver labels, non-color
  states, light/dark and increased-contrast behavior, and all product actions.
- Add self-contained previews or deterministic presentation fixtures that do
  not use live services, disk or network access.

## Verification boundary

Autonomous acceptance includes unit, model, source, layout, race, vet,
cross-build, release-build and package checks plus exact hashes. It does not
claim physical DPI/Retina quality, Narrator, VoiceOver, audible quality, Store
acceptance or real-hardware success. Those observations are requested once in
`TASK-260721-ryk8c0` after `TASK-260721-2346wf` publishes the exact candidates.
