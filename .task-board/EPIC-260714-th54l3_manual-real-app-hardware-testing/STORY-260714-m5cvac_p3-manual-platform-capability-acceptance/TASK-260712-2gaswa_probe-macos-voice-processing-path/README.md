# Prove the macOS voice-processing and route path

## Description
Select or reject a supported macOS AEC, noise suppression and AGC path on real supported hardware.

## Scope
Evaluate exact native voice-processing and Core Audio routes on supported macOS builds for built-in speakers, wired and USB headsets, Bluetooth profile changes, AirPlay or aggregate-device exclusions, default-device switches, sample-rate drift, echo-reference delay, far-end-only, near-end-only and double-talk. Measure render and capture callback ownership, latency, CPU and bounded memory, TCC and sandbox behavior, and signing, entitlement, SBOM and support implications. Define safe response to sleep, permission revoke, device loss and route changes without private APIs.

## Acceptance Criteria
A dated real-device report selects one exact supported path or blocks macOS capture quality. It states minimum OS and hardware assumptions, reference-tap and thread model, measurable quality and resource results, accepted, degraded and unsupported routes and no-go cases. Unsupported paths require an honest headphone or degraded fallback and never hidden background capture.
