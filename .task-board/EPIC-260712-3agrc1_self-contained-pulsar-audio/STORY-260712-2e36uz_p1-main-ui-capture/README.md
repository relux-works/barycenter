# P1 Main UI, local self-test and capture

## Description
Build the RU/EN main UI, local self-test, microphone capture, toggle hotkey and short-file input.

## Scope
Implement RU/EN main window and tray surfaces for Create, Join, Try locally, presence, record, file drop/picker, routing, history and settings. Add explicit microphone permission, input/output selection, level meter, toggle hotkey, cancel/limit behavior, local-only cue and loopback, accessibility and DPI support.

## Acceptance Criteria
The A1 UI path works on clean Windows without network accounts and local self-test uploads nothing. Permission denial, absent mic, hotkey conflict and coordinator outage degrade honestly. Recording is visibly and audibly indicated, temporary media is deleted per policy, keyboard/screen-reader and 125/150/200 percent DPI checks pass, and equivalent macOS surfaces meet the stated parity scope.
