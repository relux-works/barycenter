# BUG-260728-3mx5hm: mac-recording-internal-capture-quality-error

## Description
On macOS with the built-in speaker route, pressing Record briefly enters processing and then surfaces the internal string capture_captureQualityUnsupported. The capture backend intentionally fails closed when processing quality is degraded and one-generation consent is absent, but the primary recording action gives no actionable route to the existing Allow degraded capture control. This blocks the real Current Air playback journey and makes a supported safety decision look like a broken recorder.

## Scope
Mac capture composition, capture-quality presentation, recording toolbar failure UX, Russian and English copy, focused regression coverage. Preserve the fail-closed quality contract and one-generation consent semantics.

## Acceptance Criteria
When capture quality cannot be accepted, the user receives an actionable localized prompt before or at Record explaining Use headphones versus Allow this limited recording. Explicit consent resumes the same recording journey for one generation; cancel remains fail-closed. No internal enum/debug string is shown. Built-in-speaker, headphone/accepted, cancel, consent-reset, and retry paths are covered; existing capture limits, privacy, AEC/NS claims, Air routing and secret redaction remain unchanged.
