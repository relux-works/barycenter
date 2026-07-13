# Select the legal AppContainer capture and picker bridge

## Description
Select the legal AppContainer capture, picker and lifecycle API surface for the Windows Store probe before implementation.

## Scope
Audit the current Go Windows shell, Appx manifest and official Windows APIs for explicit microphone permission, default and selected input enumeration, brokered file access, RegisterHotKey on the tray loop and lifecycle signals inside packagedClassicApp appContainer. Compare pure Go WASAPI with WinRT or Media Foundation bridges, including Store eligibility, ABI or COM ownership, thread and callback rules, redistribution license, binary architecture, signing and minimum OS support. Define an interface boundary and exact manifest changes. Reject runFullTrust, broad filesystem access, undocumented APIs and developer-mode-only behavior unless separately escalated.

## Acceptance Criteria
A dated decision note chooses capture and picker paths, names required declarations, COM or thread ownership and bridge distribution, records rejected options and unresolved hardware proofs, and maps every P1.0 scenario. The choice is legal to redistribute, works in the intended signed AppContainer package and does not assume a development certificate, unpackaged process or sandbox weakening.
