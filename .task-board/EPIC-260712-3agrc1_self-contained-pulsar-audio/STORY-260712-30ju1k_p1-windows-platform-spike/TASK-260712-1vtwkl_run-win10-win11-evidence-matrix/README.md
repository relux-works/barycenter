# Produce the Windows 10 and 11 evidence matrix

## Description
Execute the signed probe on real Windows 10 and 11 hardware and publish a pass-fail evidence matrix with next actions.

## Scope
Install the reproducibly signed probe on at least one real supported Windows 10 machine and one real Windows 11 machine without relying on unpackaged or developer-mode behavior. Record OS build, architecture, audio devices, package identity and signature. From cold permission state exercise prompt timing, default and selected capture, brokered picker and continued file access, hotkey and conflict, hidden-window recording, repeated cycles, quit, suspend, session lock, device removal and permission revoke. Reinstall where needed and publish screenshots, structured logs and exact next action for every failure.

## Acceptance Criteria
The evidence matrix gives an honest pass or fail for every P1.0 bullet on both OS families using the same signed MSIX posture intended downstream. No unsupported path is counted as pass. Cleanup leaves no process, capture session, hotkey, temp handle or inaccessible picker result. Downstream Windows work may begin only from explicitly passed assumptions; any failed sandbox requirement triggers a documented architecture or product decision.
