# Add redaction-safe Phase 3 observability and evidence views

## Description
Provide stable operator evidence without exposing content keys or turning optional subsystem state into misleading health.

## Scope
Expose bounded counters and archived evidence for mouth-to-ear latency, jitter and drops, stale-session rejection, capture-stop and callback safety, DSP route and degradation, public crypto epoch or revoke progress without secret or plaintext health claims, automation execution and revoke outcomes, feature flags, build hashes and incident state. Keep readiness per optional subsystem rather than making base health falsely red or green; document queries, retention and redaction. Never emit audio, ciphertext blobs, filenames, captions, transcripts, keys, tokens, local paths or device fingerprints beyond approved pseudonyms.

## Acceptance Criteria
Reviewers can reproduce every required metric from stable documented surfaces tied to a build and environment. Missing or stale evidence is visible, optional-disabled features remain honest and secret scans plus hostile labels prove logs, metrics, health, crashes and exported artifacts contain no prohibited content.
