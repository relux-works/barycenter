# Implement macOS interrupt with audible-position resume

## Description
Implement exact interrupt semantics over the prepared branch, preserving the position actually heard rather than a provider or buffered-ahead position.

## Scope
Fade main program out over 250 ms, derive and persist an audible-position anchor from provider position and queued ring frames at the interruption boundary, pause the provider only after buffered-tail handling is deterministic, play the clip branch, restore or seek to the anchor and fade in over 120 ms. If the adapter cannot pause and resume exactly, return the frozen capability error and schedule no fallback. Make cancel, stop, provider events and reconnect generation-safe so no duplicate tail or ghost resume survives.

## Acceptance Criteria
A4 holds on macOS: the Spotify main program resumes within 500 ms of the preserved audible position without duplicate tail, missing samples or a second resume. Unsupported capability is explicit and never silently becomes overlay or after_current. Cancel and reconnect leave one terminal clip state, no armed timer and no stale provider command.
