# Air onboarding implementation verification

Date: 2026-07-27

## Implemented contract

- Pulsar installation = device.
- Barycenter = permanent private identity/home.
- Air = optional shared playback room between Barycenters.
- Air UI is gated by the typed coordinator `phase2.air_rooms_enabled` capability.
- A new Barycenter activates immediately; recovery export is resumable and can rotate fresh one-time material after restart.
- Mac can issue a one-time device invitation for Windows from Settings.
- Creating an Air also issues the initial member invitation, with stable retry keys until the workflow succeeds.
- A disabled Air rollout returns `air_rooms_not_enabled` with HTTP 503 instead of a misleading revision conflict.

## Verification

- `swift test` in `node-app`: 360 tests in 58 suites passed.
- Targeted coordinator store and Air HTTP tests passed.
- Windows unit tests passed.
- Windows test binary cross-compiled successfully with `GOOS=windows`.
- `git diff --check` passed.

Full coordinator-package reruns still reproduce unrelated existing moderation-handler failures. The Air-specific coordinator tests and all internal packages pass.
