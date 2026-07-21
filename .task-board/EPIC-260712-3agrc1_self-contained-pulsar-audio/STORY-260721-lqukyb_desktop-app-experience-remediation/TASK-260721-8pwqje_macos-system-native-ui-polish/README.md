# TASK-260721-8pwqje: macos-system-native-ui-polish

## Description
Polish the production macOS SwiftUI shell to a professional Retina-native experience aligned with macOS Human Interface Guidelines.

## Scope
Refine NavigationSplitView hierarchy, toolbar and command placement, window sizing, Settings scene behavior, section composition, system materials, typography, spacing, empty/error/recording states, keyboard focus, VoiceOver semantics, EN/RU localization behavior and self-contained previews. Prefer native SwiftUI and retain AppKit bridging only where lifecycle integration requires it.

## Acceptance Criteria
The macOS shell renders with native Retina text and symbols, coherent sidebar/detail hierarchy, system controls/materials, resilient layouts and clear primary/secondary/destructive actions. It supports macOS 14+, light/dark appearance, reduced motion/high contrast semantics, keyboard navigation and VoiceOver labels without color-only meaning. EN/RU model behavior and all existing product actions remain intact; Swift tests and release build pass.
