# Freeze the shared capture DSP graph and C3 contract

## Description
Define one capture-quality contract for every microphone workflow before either platform implementation diverges.

## Scope
Cover recorded clips, the five-second local record-then-play self-test and live PTT through one reusable capture processor. Freeze processing and thread order from device capture through clock and format alignment, echo-reference tap, AEC, noise suppression, bounded AGC, encoder or draft output, plus teardown. Distinguish an input AGC ceiling from the existing receiver output-volume ceiling, which remains last in the playback graph. Define speaker, headphone and auto route modes, reference eligibility, device-switch behavior, accepted, degraded and unsupported states, explicit user fallback, mixed-version behavior, privacy and the exact far-end-only, near-end-only, double-talk and route-change C3 matrix.

## Acceptance Criteria
A dated decision maps every P3.2 and C3 requirement to a graph seam, state, negative case and evidence artifact. It names the two distinct ceilings, the render-reference timing and ownership, all microphone workflows, transition and fallback rules, and exact objective plus blinded-listening criteria. No platform may silently bypass processing, claim AEC when unsupported or weaken explicit capture indication.
