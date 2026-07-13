# Add phase-one ingest acceptance and regression coverage

## Description
Close the story with focused integration coverage that proves supported formats, failure states, retry semantics, ACLs and Telegram compatibility on the common ingest path.

## Scope
Add coordinator and worker tests and adversarial fixtures for every accepted format, corrupt and polyglot data, truncated and oversized files, declared versus actual mismatch, ffprobe and ffmpeg timeout or crash, network-protocol attempts, output-size cap, idempotency and concurrent resume, quota, tenant dedupe isolation, target ACL, delete and expiry races, cleanup restart and Telegram compatibility. Map story acceptance criteria and security invariants to automated evidence.

## Acceptance Criteria
Every accepted input family and failure class runs end to end through the common service. No adversarial input becomes ready, reaches network through ffmpeg or leaks tenant existence. Interrupted, concurrent, deleted and expired cases leave consistent rows and bytes. Legacy Telegram acceptance ordering and compatibility WAV playback remain green, and all uncovered hardware or operational evidence is explicitly named.
