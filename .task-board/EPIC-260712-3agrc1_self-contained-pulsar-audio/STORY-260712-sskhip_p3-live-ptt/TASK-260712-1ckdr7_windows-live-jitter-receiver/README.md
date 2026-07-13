# Implement Windows live jitter and mixer playback

## Description
Receive the selected live profile through a bounded per-session jitter and concealment path integrated with the existing overlay mixer.

## Scope
Validate session and sequence, reorder only within the frozen window, apply FEC or PLC and late-frame drop, decode off render into a bounded PCM ring and start at the negotiated live edge. Drive pre-duck or duck and local volume ceiling, reject second or stale sessions, recover from 2 percent loss and bounded jitter, and drain or click-free cancel on end, timeout, leave, DND, disable or coordinator loss. Store no live audio and expose only buffer, loss and latency telemetry.

## Acceptance Criteria
Windows jitter and PCM memory are hard-bounded, malformed or stale frames cannot play, 2 percent loss remains intelligible by the agreed method and main program recovers without ring stall, overlap, clipping or stranded duck. Decoder, network and waits stay off WASAPI render and terminal cleanup removes all session state.
