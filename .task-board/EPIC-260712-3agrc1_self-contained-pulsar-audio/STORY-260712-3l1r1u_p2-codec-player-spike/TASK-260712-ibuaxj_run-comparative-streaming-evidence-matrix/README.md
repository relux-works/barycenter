# Run the complete comparative codec and player matrix

## Description
Execute identical hard-gate experiments for every viable platform combination and retain failures as first-class evidence.

## Scope
Run one-hour and two-hour RSS and disk tests, repeated seek-to-audio, pause and resume, scheduled start, drain, network resets and stalls, no-range fallback, cache reuse and eviction, corruption and hostile input on Windows to Windows, Windows to macOS and macOS to macOS. Compare platform-specific pairs such as Media Foundation plus native macOS as well as cross-platform pure-Go or bundled choices. Include CPU, package size, architecture, operational and legal matrix results; stop a candidate at a failed hard gate but retain the artifact.

## Acceptance Criteria
The evidence pack maps every proof and threshold to raw comparable artifacts, includes all three platform pairings and cannot hide a failed format behind a combined score. Rejected candidates have reproducible failure evidence. At least one complete Windows plus macOS production combination passes every hard gate before selection can proceed.
