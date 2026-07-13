# P1.0 Windows Store platform spike

## Description
Prove the packaged Windows capture and input surface before domain implementation.

## Scope
Build the smallest signed MSIX proof under current appContainer/packagedClassicApp settings. Exercise microphone permission and capture, default and selected input, toggle RegisterHotKey, brokered file picker, hidden-window recording, session lock, permission revoke and clean shutdown on real Windows 10 and 11. Select a legal capture bridge if the current Go stack is insufficient.

## Acceptance Criteria
A reproducible probe and evidence matrix covers every P1.0 bullet in spec section 19.2 on real hardware. The chosen API and manifest declarations are documented. No runFullTrust or sandbox weakening is introduced without a separate approved decision. Failures have concrete next actions and do not get reported as passes.
