# Probe native macOS AVFoundation or AudioToolbox streaming

## Description
Prove or reject the native macOS half of a platform-specific production combination using the same range, cache and scheduling contract.

## Scope
Build a sandboxed, codesigned AVFoundation or AudioToolbox adapter over the shared authenticated range cache. Test exact MP3, AAC and Opus container fixtures, incremental prepare, coordinator-time start, pause, random VBR seek, resume, decoder drain and cancellation with disk and network off the render callback. Measure all shared memory, start, seek and skew gates on the supported macOS matrix and record API, OS-version, sandbox, codesign and architecture limitations.

## Acceptance Criteria
A reproducible native macOS prototype passes the same hard gates as every other candidate or produces exact rejection evidence. It can be paired explicitly with a Windows candidate in the ADR, does not hide full-file buffering or render I/O, and names every unsupported format or OS version rather than assuming parity.
