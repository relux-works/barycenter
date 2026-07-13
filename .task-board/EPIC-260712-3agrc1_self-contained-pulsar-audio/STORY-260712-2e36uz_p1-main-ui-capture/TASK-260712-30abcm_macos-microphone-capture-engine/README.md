# macOS microphone capture engine

## Description
Implement a bounded macOS capture engine independent of UI and network transport, with explicit TCC permission and the same phase-one draft contract.

## Scope
Request TCC microphone permission only after explicit Record; capture mono PCM from default or selected input; expose device state and a local level meter without exporting samples; enforce the 180-second and 50-MiB limits; write active capture only to app-private temporary storage; sequence cues outside committed microphone samples; duck the local main program; and stop cleanly on cancel, quit, sleep, session lock where observable, device loss or permission revoke. Hand only successfully finalized media to the durable-draft service. Do not claim AEC or noise suppression.

## Acceptance Criteria
On supported macOS versions, explicit Record captures default and selected inputs; hard limits auto-stop with an explicit reason; cue audio is not embedded; lifecycle and device failures close the engine and partial files; a normal stop yields exactly one finalized draft; denial is typed and non-mic paths remain usable; no samples leave the machine before explicit upload.
