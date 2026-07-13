# Implement Windows live capture, encode and send

## Description
Turn an explicit validated hold into bounded ephemeral frames without persisting audio or allowing network state to own the microphone.

## Scope
Use the Phase 1 capture engine and validated hold controller to start only from current local action, show persistent visual and audible state, capture frames, run the selected encoder off callbacks and feed a bounded send queue with frozen drop or cancel behavior. Apply debounce, generation, watchdog and maximum duration; stop on release, local Stop, lock, suspend, permission revoke, device loss, quit, disconnect or backpressure terminal state. Never write live audio to disk, log samples or let a coordinator message start or resume capture; fall back to toggle clip before capture when hold capability is absent.

## Acceptance Criteria
Signed Windows 10 and 11 builds emit valid bounded frames only during one visible local hold. One hundred cycles, lost release, key repeat, lock, disconnect and queue saturation leave no microphone or encoder running, no old generation resumes, main capture controls remain usable and realtime callbacks contain no network, disk, allocation or blocking work.
