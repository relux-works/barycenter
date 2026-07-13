# Implement the minimal packaged Windows probe

## Description
Implement the minimal packaged probe that exercises capture, hotkey, picker and hidden-window behavior under the current MSIX posture.

## Scope
Implement the smallest probe surface under pulsar-win or a sibling Windows-only target that requests microphone permission from an explicit Record action, captures from the default and a user-selected input, toggles recording from the tray message loop with RegisterHotKey, opens a standard file picker without broad filesystem capability, keeps recording while the main window is hidden, and emits scenario-level logs.

## Acceptance Criteria
The probe runs inside the current packagedClassicApp appContainer package and can explicitly exercise each P1.0 interaction except the lifecycle stop cases handled by the next task. Logs identify the attempted scenario, selected API path, device identity, permission result and any failure cause. Unsupported behavior is exposed as fail or blocked evidence, never as a pass.
