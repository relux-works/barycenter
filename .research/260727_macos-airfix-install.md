# macOS Air onboarding app installation

Date: 2026-07-27

## Installed artifact

- Version: `0.3.0`
- Bundle build: `958.1`
- Path: `/Applications/Pulsar.app`
- NodeApp SHA-256: `4e8c986edf48fb4c3e1d05faf813881d3d5c370423d096c638dc77d3afed51d0`
- Bundle identifier: `works.relux.pulsar`
- Team identifier: `262RZ595FP`
- Signature: Developer ID Application, Relux Works, LLC

The bundle passed strict deep code-signature verification and contains the new
Barycenter/device onboarding, typed Air capability gate, and resumable recovery
copy.

## Recovery and runtime

- Previous app retained at `/Applications/Pulsar.app.backup-20260727-airfix`.
- Existing application-support data was not modified.
- The updated app and bundled go-librespot were launched successfully.
- Coordinator registration, welcome, Spotify authentication, ping/pong, and
  state publishing are present in the post-install log without new errors.
