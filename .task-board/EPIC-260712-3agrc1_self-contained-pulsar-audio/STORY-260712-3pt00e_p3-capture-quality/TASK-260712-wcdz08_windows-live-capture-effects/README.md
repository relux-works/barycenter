# Implement the shared Windows capture DSP path

## Description
Add the selected Windows processor once and reuse it for recorded clips, local record-then-play and live PTT.

## Scope
Implement the approved capture graph with render-reference alignment, AEC, noise suppression and bounded AGC before clip or live encoding. Support speaker, headphone and auto policies, device and route transitions, typed degraded or unsupported states, explicit fallback and exact teardown on release, cancel, lock, suspend, permission revoke, device loss, disconnect and quit. Keep capture and render callbacks allocation-free and nonblocking, preserve the distinct receiver output ceiling and never persist live samples or send diagnostic audio.

## Acceptance Criteria
Signed supported Windows builds pass the deterministic DSP fixtures for clips and live PTT. Accepted routes meet the frozen C3 thresholds without destroying double-talk, input AGC respects its ceiling, receiver output ceiling remains last, all degraded states are honest and every terminal path leaves no capture, reference, worker or stale generation running.
