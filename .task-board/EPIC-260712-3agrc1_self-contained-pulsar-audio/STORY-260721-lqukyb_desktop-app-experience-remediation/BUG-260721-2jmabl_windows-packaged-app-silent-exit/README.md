# BUG-260721-2jmabl: windows-packaged-app-silent-exit

## Description
Installed production-schema MSIX activates through its AUMID on the Win10 hardware host but creates an AppContainer process that exits after about one second. The desktop shortcut therefore appears to do nothing and the sandbox pulsar.log remains empty. Diagnose and repair the startup path so normal shell launch is reliable and failures are observable.

## Scope
In scope: Win10 packaged activation, unpaired startup shell, initialization error propagation, durable startup diagnostics, user-visible fatal error fallback, regression tests, rebuilt package installation and autonomous remote verification on mbpro-win. Out of scope: manual microphone/audio acceptance and other real-hardware checks retained in the consolidated owner task.

## Acceptance Criteria
Clicking the installed shortcut or activating the AUMID launches a visible production window without a console and keeps the process alive. Startup failures can no longer terminate silently: they produce a durable diagnostic and a user-visible error. Deterministic tests cover the failing startup seam. The repaired exact package is installed on mbpro-win and remote evidence records package identity, process/window lifetime and hashes; no manual audio PASS is claimed.
