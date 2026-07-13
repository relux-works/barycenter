# Prove the signed Windows voice-processing path

## Description
Select or reject an official AppContainer-compatible Windows AEC, noise suppression and AGC path on real supported hardware.

## Scope
Extend the signed Phase 1 probe to candidate documented Windows capture or communications APIs and exact OS builds. Exercise built-in speakers, wired and USB headsets, Bluetooth profile changes, default-device switches, sample-rate drift, echo-reference routing and delay, far-end-only, near-end-only and double-talk, plus lock, suspend, UAC, RDP, permission revoke and device loss. Measure callback ownership, latency, CPU and bounded memory and review redistribution, signing, SBOM and support implications. Never assume runFullTrust or development-only registration.

## Acceptance Criteria
A dated signed-package hardware report selects one exact supported path or blocks Windows capture quality. It states minimum OS, API and driver prerequisites, reference-tap and thread model, measurable quality and resource results, accepted, degraded and unsupported routes and no-go cases. No private API, sandbox weakening, first-run code download or false parity claim remains.
