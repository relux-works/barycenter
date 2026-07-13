# Windows microphone capture engine

## Description
Implement the AppContainer-safe Windows capture engine selected by P1.0 as a bounded service independent of UI and network transport.

## Scope
Use the bridge proven by the Windows spike to request microphone permission only after explicit Record; capture mono PCM from default or selected input; expose device state and a local level meter without exporting samples; enforce the 180-second and 50-MiB limits; write active capture only to app-private temporary storage; sequence cues outside committed microphone samples; duck the local main program; and stop cleanly on cancel, quit, session lock, suspend, device loss or permission revoke. Hand only successfully finalized media to the durable-draft service. Do not claim AEC or noise suppression.

## Acceptance Criteria
On signed Windows 10 and 11 packages, explicit Record captures default and selected inputs while visible or hidden; hard limits auto-stop with an explicit reason; cue audio is not embedded; every cancel and lifecycle edge closes devices and partial files; a normal stop yields exactly one finalized draft; permission denial and device loss are typed failures; no microphone samples leave the machine before explicit upload.
