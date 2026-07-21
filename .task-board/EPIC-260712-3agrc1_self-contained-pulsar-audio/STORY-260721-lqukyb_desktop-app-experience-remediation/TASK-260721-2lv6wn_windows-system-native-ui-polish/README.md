# TASK-260721-2lv6wn: windows-system-native-ui-polish

## Description
Polish the production Windows desktop shell to a professional system-native experience across Windows 10 and 11 without replacing established product flows.

## Scope
Audit and improve navigation hierarchy, window/chrome integration, system typography, spacing, surfaces, semantic state styling, dark/high-contrast behavior, keyboard focus, resize and PerMonitorV2 behavior. Use native Win32/DWM/theme APIs with Win10 fallbacks; do not add a fragile web or runtime-heavy UI stack.

## Acceptance Criteria
The production shell has a coherent navigation and content hierarchy, crisp system typography, intentional spacing and semantic status surfaces. It follows OS theme/high contrast where available, preserves keyboard/dialog navigation and non-color states, relayouts at 96/120/144/192 DPI and across supported minimum sizes, and retains all existing commands and product flows. Deterministic Go/source/cross-build checks pass.
