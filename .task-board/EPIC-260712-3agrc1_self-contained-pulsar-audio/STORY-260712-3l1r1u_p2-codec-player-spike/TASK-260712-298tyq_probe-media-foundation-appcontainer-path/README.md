# Probe Windows Media Foundation inside the signed AppContainer package

## Description
Prove or reject a native Windows streaming adapter using the exact Phase 1 signed package posture and shared hard gates.

## Scope
Build a minimal range-backed Media Foundation or approved WinRT adapter with explicit COM apartment and thread ownership, cancellation and decoder-error handling. Test exact fixture containers for MP3, AAC and Opus rather than assuming OS support; schedule from coordinator time; pause, seek, resume and drain ended; keep network, disk and decode work out of WASAPI render; exercise x64 and every intended Store architecture on real supported Windows 10 and 11 package builds. Measure all shared gates and record manifest or capability needs.

## Acceptance Criteria
The signed packaged prototype passes every required format and performance gate or is rejected with the exact unsupported format, OS build, API, timing, memory, architecture or sandbox evidence. It never depends on developer mode, runFullTrust, undocumented APIs or render-thread work and is reproducible from a pinned package and machine matrix.
