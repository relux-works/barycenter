# P2 Codec and streaming player spike

## Description
Select and prove a licensed Store-compatible bounded-memory streaming decoder path.

## Scope
Evaluate Media Foundation, pure-Go and bundled decoder options plus canonical server variants. Prove licensing, Store/AppContainer compatibility, MP3/AAC/Opus decode, range fetch, incremental buffering, bounded memory on two-hour media, scheduled start, pause, seek and resume on Windows and macOS.

## Acceptance Criteria
A decision record selects one production path from measured evidence, license review and distributable prototypes. The spike meets every section 20.2 proof item on both platforms, includes rejected-option evidence and defines exact interfaces, cache limits and test fixtures for implementation.
